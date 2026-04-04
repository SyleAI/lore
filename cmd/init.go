package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

It creates the .tickets/ directory structure, writes default policy to
.lore/policy.yaml, and sets up the .lore/ configuration directory.`,
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

	ticketsDir := filepath.Join(gitRoot, ".tickets")
	if !initForce {
		if _, err := os.Stat(ticketsDir); err == nil {
			return fmt.Errorf("lore init: already initialized (use --force to reinitialize)")
		}
	}

	loreDir := filepath.Join(gitRoot, ".lore")

	// Create .tickets/ subdirectories.
	for _, sub := range []string{"open", "done", "threads", "questions", "blobs", ".locks"} {
		dir := filepath.Join(ticketsDir, sub)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("lore init: create .tickets/%s: %w", sub, err)
		}
	}
	logf("  created .tickets/\n")

	// Write .gitattributes to suppress diff/log noise for .tickets/.
	gaPath := filepath.Join(gitRoot, ".gitattributes")
	gaLine := ".tickets/** -diff\n"
	if err := appendLineIfMissing(gaPath, gaLine); err != nil {
		logf("  warning: could not update .gitattributes: %v\n", err)
	} else {
		logf("  updated .gitattributes (.tickets/** -diff)\n")
	}

	// Write default policy to .lore/policy.yaml.
	if err := os.MkdirAll(loreDir, 0755); err != nil {
		return fmt.Errorf("lore init: create .lore dir: %w", err)
	}
	policyPath := filepath.Join(loreDir, "policy.yaml")
	if err := os.WriteFile(policyPath, templates.DefaultPolicy, 0644); err != nil {
		return fmt.Errorf("lore init: write .lore/policy.yaml: %w", err)
	}
	logf("  created .lore/policy.yaml\n")

	// Write .lore/config.yaml.
	configPath := filepath.Join(loreDir, "config.yaml")
	if err := os.WriteFile(configPath, templates.DefaultConfig, 0644); err != nil {
		return fmt.Errorf("lore init: write .lore/config.yaml: %w", err)
	}
	logf("  created .lore/config.yaml\n")

	// Write .lore/.gitignore.
	gitignorePath := filepath.Join(loreDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("events.log\n"), 0644); err != nil {
		return fmt.Errorf("lore init: write .lore/.gitignore: %w", err)
	}
	logf("  created .lore/.gitignore\n")

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventLoreInitialized, map[string]any{"git_root": gitRoot}))

	logf("\nlore initialized successfully.\n")
	return nil
}

// appendLineIfMissing appends line to path if it isn't already present.
func appendLineIfMissing(path, line string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(existing)
	for _, l := range splitLines(content) {
		if l == trimNewline(line) {
			return nil // already present
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(line)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

func splitLines(s string) []string {
	var lines []string
	for _, l := range splitNewlines(s) {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func splitNewlines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimNewline(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return s[:len(s)-1]
	}
	return s
}

func runDoctor(cmd *cobra.Command, args []string) error {
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

	// Check .tickets/ directory.
	_, ticketsErr := os.Stat(filepath.Join(gitRoot, ".tickets", "open"))
	check(".tickets/open/ exists", ticketsErr == nil, ".tickets/open/ not found — run lore init")

	// Check .lore/policy.yaml.
	policyData, policyErr := os.ReadFile(filepath.Join(loreDir, "policy.yaml"))
	if policyErr != nil {
		check(".lore/policy.yaml", false, policyErr.Error())
	} else {
		var obj map[string]any
		if yamlErr := yaml.Unmarshal(policyData, &obj); yamlErr != nil {
			check(".lore/policy.yaml", false, yamlErr.Error())
		} else {
			check(".lore/policy.yaml", true, "")
		}
	}

	// Check .lore/config.yaml.
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
