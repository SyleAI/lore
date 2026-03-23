package cmd

import (
	"fmt"
	"sort"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var listClosed bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tickets",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&listClosed, "closed", false, "include done tickets")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	tickets, err := listTickets(ctx, gitRoot, listClosed)
	if err != nil {
		return fmt.Errorf("lore list: %w", err)
	}

	if len(tickets) == 0 {
		fmt.Println("no tickets found")
		return nil
	}

	// Sort by created time, newest first.
	sort.Slice(tickets, func(i, j int) bool {
		return tickets[i].CreatedAt.After(tickets[j].CreatedAt)
	})

	fmt.Printf("%-14s  %-16s  %s\n", "ID", "STATUS", "DESCRIPTION")
	fmt.Printf("%-14s  %-16s  %s\n", "--------------", "----------------", "-----------")
	for _, t := range tickets {
		status := string(t.Status)
		desc := truncate(t.Desc, 60)
		// Skip done tickets unless --closed requested.
		if !listClosed && t.Status == ticket.StatusDone {
			continue
		}
		fmt.Printf("%-14s  %-16s  %s\n", t.ID, status, desc)
	}
	return nil
}
