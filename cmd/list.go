package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/store"
	"github.com/spf13/cobra"
)

var (
	listStatus string
	listAll    bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tickets",
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "open", "filter by status: open, in-progress, blocked, ready, closed")
	listCmd.Flags().BoolVar(&listAll, "all", false, "show tickets of all statuses")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore list: %w", err)
	}
	defer s.Close()

	status := listStatus
	if listAll {
		status = ""
	}

	rows, err := s.ListTickets(store.ListFilter{Status: status})
	if err != nil {
		return fmt.Errorf("lore list: %w", err)
	}

	if len(rows) == 0 {
		fmt.Println("no tickets found")
		return nil
	}

	fmt.Printf("%-14s  %-4s  %-12s  %s\n", "ID", "PRI", "STATUS", "TITLE")
	fmt.Printf("%-14s  %-4s  %-12s  %s\n", "--------------", "----", "------------", "-----")
	for _, r := range rows {
		fmt.Printf("%-14s  %-4d  %-12s  %s\n", r.ID, r.Priority, r.Status, r.Title)
	}
	return nil
}
