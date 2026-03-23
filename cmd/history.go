package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history <file>",
	Short: "List all tickets that have touched a file",
	Args:  cobra.ExactArgs(1),
	RunE:  runHistory,
}

func init() {
	rootCmd.AddCommand(historyCmd)
}

func runHistory(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	filePath := args[0]

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore history: %w", err)
	}
	defer s.Close()

	rows, err := s.FindTicketsByFile(filePath)
	if err != nil {
		return fmt.Errorf("lore history: %w", err)
	}

	if len(rows) == 0 {
		fmt.Printf("no tickets found for %s\n", filePath)
		return nil
	}

	fmt.Printf("Tickets touching %s (%d):\n", filePath, len(rows))
	fmt.Printf("  %-14s  %-12s  %s\n", "TICKET", "STATUS", "TITLE")
	for _, r := range rows {
		fmt.Printf("  %-14s  %-12s  %s\n", r.ID, r.Status, r.Title)
	}
	return nil
}
