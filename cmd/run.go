package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <name> [args...]",
	Short: "Run an installed orchestrator",
	Long: `run executes an installed orchestrator against the current repository.

The orchestrator receives:
  LORE_REPO_ROOT         absolute path to the git repository root
  LORE_ORCHESTRATOR_DIR  absolute path to .lore/orchestrators/<name>/
  LORE_ORCHESTRATOR_NAME orchestrator name

Working directory is set to LORE_REPO_ROOT.
Any extra arguments are passed through to the orchestrator.`,
	RunE:               runOrchestrator,
	DisableFlagParsing: true,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runOrchestrator(cmd *cobra.Command, args []string) error {
	// DisableFlagParsing means all args are raw — handle --help/-h manually.
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return cmd.Help()
	}

	name := args[0]
	passthroughArgs := args[1:]

	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	orchsBase := filepath.Join(gitRoot, ".lore", "orchestrators")
	orchDir := filepath.Join(orchsBase, name)

	// Guard against path traversal (e.g. lore run ../../etc).
	if rel, err := filepath.Rel(orchsBase, orchDir); err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("lore run: invalid orchestrator name %q", name)
	}

	if _, err := os.Stat(orchDir); os.IsNotExist(err) {
		return fmt.Errorf("lore run: orchestrator %q not found (install with: lore install <source>)", name)
	}

	manifest, err := readManifest(orchDir)
	if err != nil {
		// Try to detect entrypoint by convention.
		manifest = &Manifest{Name: name}
	}
	if manifest.Name == "" {
		manifest.Name = name
	}

	entrypoint, rt, err := resolveEntrypoint(orchDir, manifest)
	if err != nil {
		return fmt.Errorf("lore run: %w", err)
	}

	// Warn if goal file is missing.
	goal := manifest.Goal
	if goal == "" {
		goal = "GOAL.md"
	}
	goalPath := filepath.Join(gitRoot, goal)
	if _, err := os.Stat(goalPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: goal file %q not found — create it before running\n", goal)
	}

	// Build command. When no runtime, execute the entrypoint directly.
	var c *exec.Cmd
	if rt == "" {
		c = exec.CommandContext(cmd.Context(), entrypoint, passthroughArgs...)
	} else {
		c = exec.CommandContext(cmd.Context(), rt, append([]string{entrypoint}, passthroughArgs...)...)
	}

	c.Dir = gitRoot
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin

	// Inject lore env vars, replacing any inherited LORE_* values so nested
	// orchestrator calls see the correct repo root rather than an outer one.
	loreKeys := []string{"LORE_REPO_ROOT=", "LORE_ORCHESTRATOR_DIR=", "LORE_ORCHESTRATOR_NAME="}
	env := make([]string, 0, len(os.Environ())+3)
	for _, kv := range os.Environ() {
		masked := false
		for _, key := range loreKeys {
			if strings.HasPrefix(kv, key) {
				masked = true
				break
			}
		}
		if !masked {
			env = append(env, kv)
		}
	}
	c.Env = append(env,
		"LORE_REPO_ROOT="+gitRoot,
		"LORE_ORCHESTRATOR_DIR="+orchDir,
		"LORE_ORCHESTRATOR_NAME="+manifest.Name,
	)

	fmt.Fprintf(os.Stderr, "running %s\n", name)
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("lore run: %w", err)
	}
	return nil
}

// resolveEntrypoint returns the entrypoint path and runtime for the orchestrator.
// If the manifest specifies them, those are used; otherwise detection by convention.
func resolveEntrypoint(orchDir string, manifest *Manifest) (entrypoint, rt string, err error) {
	if manifest.Entrypoint != "" {
		ep := filepath.Join(orchDir, manifest.Entrypoint)
		if _, err := os.Stat(ep); err != nil {
			return "", "", fmt.Errorf("entrypoint %q not found in orchestrator directory", manifest.Entrypoint)
		}
		rt = manifest.Runtime
		if rt == "" {
			rt = detectRuntime(manifest.Entrypoint)
		}
		return ep, rt, nil
	}

	// Convention-based detection.
	candidates := []struct {
		file    string
		runtime string
	}{
		{"orchestrator.py", "python3"},
		{"orchestrator.sh", "bash"},
		{"run.sh", "bash"},
		{"orchestrator", ""},
	}
	for _, c := range candidates {
		p := filepath.Join(orchDir, c.file)
		if _, err := os.Stat(p); err == nil {
			return p, c.runtime, nil
		}
	}
	return "", "", fmt.Errorf("no entrypoint found; add lore.yaml with entrypoint field")
}

// detectRuntime guesses the runtime from a filename extension.
func detectRuntime(filename string) string {
	switch filepath.Ext(filename) {
	case ".py":
		return "python3"
	case ".sh":
		return "bash"
	case ".js":
		if runtime.GOOS == "windows" {
			return "node.exe"
		}
		return "node"
	default:
		return ""
	}
}
