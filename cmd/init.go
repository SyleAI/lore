package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/loreteam/lore/internal/config"
	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/templates"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize lore in the current git repository",
	Long: `init sets up lore storage in the current git repository.

It writes default policy to refs/tickets/policy, creates the .lore/
configuration directory, and installs Claude skill files under .claude/skills/.`,
	RunE: runInit,
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check lore installation health",
	Long:  `doctor verifies that all required lore components are present and valid.`,
	RunE:  runDoctor,
}

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "reinitialize even if lore is already set up")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(doctorCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	logf := func(format string, a ...any) {
		if !flagQuiet {
			fmt.Printf(format, a...)
		}
	}

	gitRoot, err := gitcmd.FindGitRoot(".")
	if err != nil {
		return fmt.Errorf("lore init: %w", err)
	}

	if !initForce {
		_, refErr := gitcmd.ReadRef(ctx, gitRoot, "refs/tickets/policy")
		if refErr == nil {
			return fmt.Errorf("lore init: already initialized (use --force to reinitialize)")
		}
	}

	loreDir := filepath.Join(gitRoot, ".lore")

	// Write default policy blob → ref.
	policySHA, err := gitcmd.WriteBlob(ctx, gitRoot, templates.DefaultPolicy)
	if err != nil {
		return fmt.Errorf("lore init: write policy blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, "refs/tickets/policy", policySHA); err != nil {
		return fmt.Errorf("lore init: write policy ref: %w", err)
	}
	logf("  created refs/tickets/policy (%s)\n", policySHA[:8])

	// Create .lore/ directory.
	if err := os.MkdirAll(loreDir, 0755); err != nil {
		return fmt.Errorf("lore init: create .lore dir: %w", err)
	}
	logf("  created .lore/\n")

	// Write .lore/config.yaml.
	configPath := filepath.Join(loreDir, "config.yaml")
	if err := os.WriteFile(configPath, templates.DefaultConfig, 0644); err != nil {
		return fmt.Errorf("lore init: write .lore/config.yaml: %w", err)
	}
	logf("  created .lore/config.yaml\n")

	// Write .lore/.gitignore.
	gitignoreContent := "events.log\n"
	gitignorePath := filepath.Join(loreDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return fmt.Errorf("lore init: write .lore/.gitignore: %w", err)
	}
	logf("  created .lore/.gitignore\n")

	// Add fetch refspec to each remote.
	remotes, remoteErr := listRemotes(ctx, gitRoot)
	if remoteErr != nil {
		logf("  warning: could not list remotes: %v\n", remoteErr)
	} else {
		for _, remote := range remotes {
			if err := ensureTicketRefspec(ctx, gitRoot, remote); err != nil {
				logf("  warning: could not update remote %s: %v\n", remote, err)
			} else {
				logf("  updated remote %s with refs/tickets/* refspec\n", remote)
			}
		}
	}

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventLoreInitialized, map[string]any{"git_root": gitRoot}))

	logf("\nlore initialized successfully.\n")
	return nil
}

func listRemotes(ctx context.Context, gitRoot string) ([]string, error) {
	out, err := execGit(ctx, gitRoot, "remote")
	if err != nil {
		return nil, err
	}
	var remotes []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			remotes = append(remotes, line)
		}
	}
	return remotes, nil
}

func ensureTicketRefspec(ctx context.Context, gitRoot, remote string) error {
	refspec := "+refs/tickets/*:refs/tickets/*"
	configKey := fmt.Sprintf("remote.%s.fetch", remote)

	out, err := execGit(ctx, gitRoot, "config", "--get-all", configKey)
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(line) == refspec {
				return nil
			}
		}
	}

	_, err = execGit(ctx, gitRoot, "config", "--add", configKey, refspec)
	return err
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	gitRoot, err := gitcmd.FindGitRoot(".")
	if err != nil {
		return fmt.Errorf("lore doctor: %w", err)
	}

	loreDir := filepath.Join(gitRoot, ".lore")

	allOK := true
	check := func(label string, ok bool, reason string) {
		if ok {
			fmt.Printf("  ✓ %s\n", label)
		} else {
			fmt.Printf("  ✗ %s — %s\n", label, reason)
			allOK = false
		}
	}

	checkYAMLBlob := func(ref, label string) {
		sha, err := gitcmd.ReadRef(ctx, gitRoot, ref)
		if err != nil {
			check(label, false, err.Error())
			return
		}
		data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
		if err != nil {
			check(label, false, err.Error())
			return
		}
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err != nil {
			check(label, false, err.Error())
			return
		}
		check(label, true, "")
	}

	checkYAMLBlob("refs/tickets/policy", "refs/tickets/policy")

	configData, configErr := os.ReadFile(filepath.Join(loreDir, "config.yaml"))
	if configErr != nil {
		check(".lore/config.yaml", false, configErr.Error())
	} else {
		var cfgObj map[string]any
		if yamlErr := yaml.Unmarshal(configData, &cfgObj); yamlErr != nil {
			check(".lore/config.yaml", false, yamlErr.Error())
		} else {
			check(".lore/config.yaml", true, "")
		}
	}

	cfg, cfgErr := config.Load(loreDir)
	if cfgErr != nil {
		cfg = config.Default()
	}
	if cfg.AI.Reasoning.Provider == "claude-cli" || cfg.AI.Reasoning.Provider == "" {
		_, lookErr := exec.LookPath("claude")
		check("claude CLI in PATH", lookErr == nil,
			"install Claude Code or set ai.reasoning.provider in .lore/config.yaml")
	}
	if cfg.AI.Reasoning.Provider == "anthropic" {
		_, hasKey := os.LookupEnv("ANTHROPIC_API_KEY")
		check("ANTHROPIC_API_KEY set", hasKey,
			"set ANTHROPIC_API_KEY or switch ai.reasoning.provider to claude-cli")
	}

	if !allOK {
		return fmt.Errorf("lore doctor: one or more checks failed")
	}
	return nil
}
