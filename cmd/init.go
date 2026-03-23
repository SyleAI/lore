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
	"github.com/loreteam/lore/internal/store"
	"github.com/loreteam/lore/internal/templates"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	initForce       bool
	initImprovement bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize lore in the current git repository",
	Long: `init sets up lore storage in the current git repository.

It writes default policy and graph index to git refs (refs/tickets/policy
and refs/tickets/graph), creates the .lore/ configuration directory, and
installs Claude skill files under .claude/skills/.`,
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
	initCmd.Flags().BoolVar(&initImprovement, "improvement", false, "also write improvement.yaml to the working tree root")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(doctorCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// logf prints progress messages unless --quiet is set.
	logf := func(format string, a ...any) {
		if !flagQuiet {
			fmt.Printf(format, a...)
		}
	}

	// init must find a git root itself — PersistentPreRunE may not have found one
	// (or may have stored empty string for repos not yet initialized).
	gitRoot, err := gitcmd.FindGitRoot(".")
	if err != nil {
		return fmt.Errorf("lore init: %w", err)
	}

	// Check if already initialized.
	if !initForce {
		_, refErr := gitcmd.ReadRef(ctx, gitRoot, "refs/tickets/policy")
		if refErr == nil {
			return fmt.Errorf("lore init: already initialized (use --force to reinitialize)")
		}
	}

	loreDir := filepath.Join(gitRoot, ".lore")
	claudeSkillsDir := filepath.Join(gitRoot, ".claude", "skills")

	// Write default policy blob → ref.
	policySHA, err := gitcmd.WriteBlob(ctx, gitRoot, templates.DefaultPolicy)
	if err != nil {
		return fmt.Errorf("lore init: write policy blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, "refs/tickets/policy", policySHA); err != nil {
		return fmt.Errorf("lore init: write policy ref: %w", err)
	}
	logf("  created refs/tickets/policy (%s)\n", policySHA[:8])

	// Write empty graph index blob → ref.
	graphSHA, err := gitcmd.WriteBlob(ctx, gitRoot, templates.DefaultGraph)
	if err != nil {
		return fmt.Errorf("lore init: write graph blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, "refs/tickets/graph", graphSHA); err != nil {
		return fmt.Errorf("lore init: write graph ref: %w", err)
	}
	logf("  created refs/tickets/graph (%s)\n", graphSHA[:8])

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
	gitignoreContent := "graph.db\n*.db-wal\n*.db-shm\nevents.log\nmeasurements/\n"
	gitignorePath := filepath.Join(loreDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return fmt.Errorf("lore init: write .lore/.gitignore: %w", err)
	}
	logf("  created .lore/.gitignore\n")

	// Create .claude/skills/ directory and write skill files.
	if err := os.MkdirAll(claudeSkillsDir, 0755); err != nil {
		return fmt.Errorf("lore init: create .claude/skills dir: %w", err)
	}

	skillFiles := []string{"lore-worker.md", "lore-lead.md", "lore-observer.md"}
	for _, name := range skillFiles {
		content, readErr := templates.SkillFiles.ReadFile("skills/" + name)
		if readErr != nil {
			return fmt.Errorf("lore init: read embedded skill %s: %w", name, readErr)
		}
		dest := filepath.Join(claudeSkillsDir, name)
		if err := os.WriteFile(dest, content, 0644); err != nil {
			return fmt.Errorf("lore init: write skill %s: %w", name, err)
		}
		logf("  created .claude/skills/%s\n", name)
	}

	// Write default prompt templates to .lore/prompts/.
	promptsDir := filepath.Join(loreDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		return fmt.Errorf("lore init: create .lore/prompts dir: %w", err)
	}
	for _, name := range templates.PromptNames() {
		content, readErr := templates.PromptFiles.ReadFile("prompts/" + name + ".md")
		if readErr != nil {
			return fmt.Errorf("lore init: read embedded prompt %s: %w", name, readErr)
		}
		dest := filepath.Join(promptsDir, name+".md")
		if err := os.WriteFile(dest, content, 0644); err != nil {
			return fmt.Errorf("lore init: write prompt %s: %w", name, err)
		}
		logf("  created .lore/prompts/%s.md\n", name)
	}

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

	// Optionally write improvement.yaml.
	if initImprovement {
		improvementPath := filepath.Join(gitRoot, "improvement.yaml")
		if err := os.WriteFile(improvementPath, templates.DefaultImprovement, 0644); err != nil {
			return fmt.Errorf("lore init: write improvement.yaml: %w", err)
		}
		logf("  created improvement.yaml\n")
	}

	// Initialize the SQLite index.
	if s, err := openStore(gitRoot); err != nil {
		logf("  warning: could not initialize graph.db: %v\n", err)
	} else {
		s.Close()
		logf("  created .lore/graph.db\n")
	}

	// Emit event.
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventLoreInitialized, map[string]any{"git_root": gitRoot}))

	logf("\nlore initialized successfully.")
	return nil
}

// listRemotes returns the names of all git remotes.
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

// ensureTicketRefspec adds +refs/tickets/*:refs/tickets/* to the remote's fetch config
// if it is not already present.
func ensureTicketRefspec(ctx context.Context, gitRoot, remote string) error {
	refspec := "+refs/tickets/*:refs/tickets/*"
	configKey := fmt.Sprintf("remote.%s.fetch", remote)

	// Check if it already exists.
	out, err := execGit(ctx, gitRoot, "config", "--get-all", configKey)
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(line) == refspec {
				return nil // already present
			}
		}
	}

	// Add the refspec.
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
	claudeSkillsDir := filepath.Join(gitRoot, ".claude", "skills")

	allOK := true
	check := func(label string, ok bool, reason string) {
		if ok {
			fmt.Printf("  \u2713 %s\n", label)
		} else {
			fmt.Printf("  \u2717 %s \u2014 %s\n", label, reason)
			allOK = false
		}
	}

	// checkYAMLBlob verifies a git ref exists, is readable, and contains valid YAML.
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
	checkYAMLBlob("refs/tickets/graph", "refs/tickets/graph")

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

	// Check .claude/skills/lore-worker.md.
	workerPath := filepath.Join(claudeSkillsDir, "lore-worker.md")
	if _, statErr := os.Stat(workerPath); statErr != nil {
		check(".claude/skills/lore-worker.md", false, statErr.Error())
	} else {
		check(".claude/skills/lore-worker.md", true, "")
	}

	// Check AI provider availability.
	cfg, cfgErr := config.Load(loreDir)
	if cfgErr != nil {
		cfg = config.Default()
	}
	if cfg.AI.Reasoning.Provider == "claude-cli" || cfg.AI.Reasoning.Provider == "" {
		_, lookErr := exec.LookPath("claude")
		check("claude CLI in PATH", lookErr == nil,
			"install Claude Code or set ai.reasoning.provider in .lore/config.yaml")
	}

	// Check and rebuild SQLite index.
	s, storeErr := openStore(gitRoot)
	if storeErr != nil {
		check(".lore/graph.db", false, storeErr.Error())
	} else {
		defer s.Close()

		// Rebuild index from git refs.
		refs, refsErr := gitcmd.ListRefs(ctx, gitRoot, "refs/tickets/t/")
		if refsErr != nil {
			check(".lore/graph.db rebuilt from refs", false, refsErr.Error())
		} else {
			rebuilt, skipped := 0, 0
			seenIDs := make(map[string]struct{}, len(refs))
			for ref, sha := range refs {
				data, blobErr := gitcmd.ReadBlob(ctx, gitRoot, sha)
				if blobErr != nil {
					fmt.Fprintf(os.Stderr, "  warning: skipping %s: read blob: %v\n", ref, blobErr)
					skipped++
					continue
				}
				t, parseErr := ticket.Unmarshal(data)
				if parseErr != nil {
					fmt.Fprintf(os.Stderr, "  warning: skipping %s: parse ticket: %v\n", ref, parseErr)
					skipped++
					continue
				}
				if upsertErr := s.UpsertTicket(t, sha); upsertErr != nil {
					fmt.Fprintf(os.Stderr, "  warning: skipping %s: index: %v\n", ref, upsertErr)
					skipped++
					continue
				}
				seenIDs[t.ID] = struct{}{}
				rebuilt++
			}
			// Remove stale index entries for tickets no longer in git.
			allRows, listErr := s.ListTickets(store.ListFilter{})
			if listErr == nil {
				for _, r := range allRows {
					if _, found := seenIDs[r.ID]; !found {
						_ = s.DeleteTicket(r.ID)
					}
				}
			}

			label := fmt.Sprintf(".lore/graph.db rebuilt (%d tickets)", rebuilt)
			if skipped > 0 {
				label += fmt.Sprintf(", %d skipped (see warnings above)", skipped)
			}
			check(label, true, "")
		}
	}

	if !allOK {
		return fmt.Errorf("lore doctor: one or more checks failed")
	}
	return nil
}
