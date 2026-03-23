package cmd

import (
	"fmt"
	"strings"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	updateTitle       string
	updateDescription string
	updateStatus      string
	updatePriority    int
	updateFiles       []string
	updateAddFiles    []string
	updateAgent       string
)

var updateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update ticket fields non-interactively",
	Args:  cobra.ExactArgs(1),
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "new title")
	updateCmd.Flags().StringVar(&updateDescription, "description", "", "new description")
	updateCmd.Flags().StringVar(&updateStatus, "status", "", "new status")
	updateCmd.Flags().IntVar(&updatePriority, "priority", 0, "new priority (1–5)")
	updateCmd.Flags().StringSliceVar(&updateFiles, "files", nil, "replace file list (comma-separated)")
	updateCmd.Flags().StringSliceVar(&updateAddFiles, "add-files", nil, "append to file list (comma-separated)")
	updateCmd.Flags().StringVar(&updateAgent, "agent", "", "new agent assignment")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]

	// Validate status flag before the CAS loop.
	if updateStatus != "" && !ticket.ValidStatus(ticket.Status(updateStatus)) {
		return fmt.Errorf("lore update: invalid status %q", updateStatus)
	}

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if updateTitle != "" {
			t.Title = updateTitle
		}
		if updateDescription != "" {
			t.Description = updateDescription
		}
		if updateStatus != "" {
			t.Status = ticket.Status(updateStatus)
		}
		if updatePriority != 0 {
			t.Priority = updatePriority
		}
		if updateFiles != nil {
			t.Files = flattenCSV(updateFiles)
		}
		if updateAddFiles != nil {
			t.Files = append(t.Files, flattenCSV(updateAddFiles)...)
		}
		if updateAgent != "" {
			t.Agent = updateAgent
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore update: %w", err)
	}

	printTicket(t)
	return nil
}

// flattenCSV expands a []string that may contain comma-separated values into a flat slice.
func flattenCSV(in []string) []string {
	var out []string
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
