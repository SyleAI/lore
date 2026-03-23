package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var reopenCmd = &cobra.Command{
	Use:   "reopen <ticket-id>",
	Short: "Move a closed ticket back to open",
	Args:  cobra.ExactArgs(1),
	RunE:  runReopen,
}

func init() {
	rootCmd.AddCommand(reopenCmd)
}

func runReopen(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status != ticket.StatusClosed {
			return fmt.Errorf("ticket %s is not closed (status: %s)", t.ID, t.Status)
		}
		t.Status = ticket.StatusOpen
		t.BlockReason = ""
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore reopen: %w", err)
	}

	fmt.Printf("ticket %s reopened\n", t.ID)
	return nil
}
