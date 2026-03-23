package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var escalateCmd = &cobra.Command{
	Use:   "escalate <id> <reason>",
	Short: "Mark a ticket as blocked with a reason",
	Args:  cobra.ExactArgs(2),
	RunE:  runEscalate,
}

func init() {
	rootCmd.AddCommand(escalateCmd)
}

func runEscalate(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("lore escalate: %w", err)
	}

	fmt.Printf("ticket %s blocked: %s\n", t.ID, reason)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketEscalated, map[string]any{
		"id":     t.ID,
		"reason": reason,
	}))
	return nil
}
