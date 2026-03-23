package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var blockCmd = &cobra.Command{
	Use:   "block <id> <reason>",
	Short: "Mark a ticket as blocked with a reason",
	Args:  cobra.ExactArgs(2),
	RunE:  runBlock,
}

func init() {
	rootCmd.AddCommand(blockCmd)
}

func runBlock(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id, reason := args[0], args[1]

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusBlocked
		t.BlockReason = reason
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore block: %w", err)
	}

	fmt.Printf("ticket %s blocked: %s\n", t.ID, reason)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketBlocked, map[string]any{
		"ticket_id": t.ID,
		"reason":    reason,
	}))
	return nil
}
