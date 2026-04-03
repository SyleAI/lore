package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Manifest is the lore.yaml file that every orchestrator ships.
type Manifest struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Runtime     string `yaml:"runtime"`
	Entrypoint  string `yaml:"entrypoint"`
	Goal        string `yaml:"goal"` // path relative to LORE_REPO_ROOT, defaults to GOAL.md
}

var orchestratorsCmd = &cobra.Command{
	Use:   "orchestrators",
	Short: "List installed orchestrators",
	Long:  `orchestrators lists all orchestrators installed in .lore/orchestrators/.`,
	RunE:  runOrchestrators,
}

var orchestratorsRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed orchestrator",
	Args:  cobra.ExactArgs(1),
	RunE:  runOrchestratorRemove,
}

func init() {
	orchestratorsCmd.AddCommand(orchestratorsRemoveCmd)
	rootCmd.AddCommand(orchestratorsCmd)
}

func runOrchestrators(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	orchsDir := filepath.Join(gitRoot, ".lore", "orchestrators")
	entries, err := os.ReadDir(orchsDir)
	if os.IsNotExist(err) {
		fmt.Println("No orchestrators installed. Use: lore install <source>")
		return nil
	}
	if err != nil {
		return fmt.Errorf("lore orchestrators: %w", err)
	}
	if len(entries) == 0 {
		fmt.Println("No orchestrators installed. Use: lore install <source>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	rows := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if rows == 0 {
			fmt.Fprintln(w, "NAME\tVERSION\tDESCRIPTION")
		}
		orchDir := filepath.Join(orchsDir, entry.Name())
		manifest, err := readManifest(orchDir)
		if err != nil {
			fmt.Fprintf(w, "%s\t\t(no manifest)\n", entry.Name())
		} else {
			name := manifest.Name
			if name == "" {
				name = entry.Name()
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", name, manifest.Version, manifest.Description)
		}
		rows++
	}

	if rows == 0 {
		fmt.Println("No orchestrators installed. Use: lore install <source>")
		return nil
	}
	return w.Flush()
}

func runOrchestratorRemove(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	name := args[0]
	orchsBase := filepath.Join(gitRoot, ".lore", "orchestrators")
	orchDir := filepath.Join(orchsBase, name)

	// Guard against path traversal (e.g. lore orchestrators remove ../../).
	if rel, err := filepath.Rel(orchsBase, orchDir); err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("lore orchestrators remove: invalid name %q", name)
	}

	if _, err := os.Stat(orchDir); os.IsNotExist(err) {
		return fmt.Errorf("lore orchestrators remove: %q is not installed", name)
	}

	if err := os.RemoveAll(orchDir); err != nil {
		return fmt.Errorf("lore orchestrators remove: %w", err)
	}

	fmt.Printf("removed %s\n", name)
	return nil
}

// readManifest reads and parses lore.yaml from an orchestrator directory.
func readManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "lore.yaml"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse lore.yaml: %w", err)
	}
	return &m, nil
}
