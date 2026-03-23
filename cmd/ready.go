package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var readyCmd = &cobra.Command{
	Use:   "ready <id>",
	Short: "Signal that work is done and the ticket is ready for review",
	Args:  cobra.ExactArgs(1),
	RunE:  runReady,
}

func init() {
	rootCmd.AddCommand(readyCmd)
}

func runReady(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status != ticket.StatusWorking {
			return fmt.Errorf("ticket %s is %s, not working", t.ID, t.Status)
		}
		t.Status = ticket.StatusReadyForReview
		t.BlockReason = ""
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore ready: %w", err)
	}

	fmt.Printf("ticket %s marked as ready for review\n", t.ID)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketReady, map[string]any{"id": t.ID}))
	return nil
}
