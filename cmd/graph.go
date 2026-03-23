package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/graph"
	"github.com/loreteam/lore/internal/policy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Manage the lore knowledge graph",
}

var graphUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Rebuild the graph index from all ticket refs",
	RunE:  runGraphUpdate,
}

var graphShowCmd = &cobra.Command{
	Use:   "show [ticket-id]",
	Short: "Print the graph index or a single ticket node",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runGraphShow,
}

func init() {
	graphCmd.AddCommand(graphUpdateCmd)
	graphCmd.AddCommand(graphShowCmd)
	rootCmd.AddCommand(graphCmd)
}

func runGraphUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	pol, err := policy.Load(ctx, gitRoot)
	if err != nil {
		return fmt.Errorf("lore graph update: load policy: %w", err)
	}

	idx, err := graph.Build(ctx, gitRoot, pol)
	if err != nil {
		return fmt.Errorf("lore graph update: %w", err)
	}

	if err := graph.Save(ctx, gitRoot, idx); err != nil {
		return fmt.Errorf("lore graph update: save: %w", err)
	}

	fmt.Printf("graph updated: %d tickets, %d files, %d clusters\n",
		len(idx.Tickets), len(idx.Files), len(idx.Clusters))
	return nil
}

func runGraphShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	idx, err := graph.Load(ctx, gitRoot)
	if err != nil {
		return fmt.Errorf("lore graph show: %w", err)
	}

	if len(args) == 1 {
		id := args[0]
		node, ok := idx.Tickets[id]
		if !ok {
			return fmt.Errorf("lore graph show: ticket %s not in graph (run lore graph update)", id)
		}
		fmt.Printf("ticket: %s\n", id)
		fmt.Printf("  score:      %d\n", node.Score)
		fmt.Printf("  files:      %v\n", node.Files)
		fmt.Printf("  blocks:     %v\n", node.Blocks)
		fmt.Printf("  blocked_by: %v\n", node.BlockedBy)
		if node.ClusterID != "" {
			fmt.Printf("  cluster:    %s\n", node.ClusterID)
		}
		return nil
	}

	// Full graph dump.
	data, err := yaml.Marshal(idx)
	if err != nil {
		return fmt.Errorf("lore graph show: %w", err)
	}
	fmt.Print(string(data))
	return nil
}
