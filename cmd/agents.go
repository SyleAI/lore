package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var agentsContext string

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Show all agents and their ticket workload",
	Args:  cobra.NoArgs,
	RunE:  runAgents,
}

func init() {
	agentsCmd.Flags().StringVar(&agentsContext, "context", "", "filter to tickets touching this file path")
	rootCmd.AddCommand(agentsCmd)
}

func runAgents(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore agents: %w", err)
	}
	defer s.Close()

	agents, err := s.ListAgents(agentsContext)
	if err != nil {
		return fmt.Errorf("lore agents: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("no agents found")
		return nil
	}

	if agentsContext != "" {
		fmt.Printf("Agents (context: %s):\n", agentsContext)
	} else {
		fmt.Println("Agents:")
	}
	fmt.Printf("  %-20s  %5s  %5s  %5s  %5s  %5s\n", "AGENT", "TOTAL", "OPEN", "WIP", "BLK", "RDY")
	for _, a := range agents {
		fmt.Printf("  %-20s  %5d  %5d  %5d  %5d  %5d\n",
			a.Agent, a.Total, a.Open, a.InProgress, a.Blocked, a.Ready)
	}
	return nil
}
