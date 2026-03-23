package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var unblockFrom string

var unblockCmd = &cobra.Command{
	Use:   "unblock <ticket-id>",
	Short: "Clear an escalation and resume a blocked ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runUnblock,
}

func init() {
	unblockCmd.Flags().StringVar(&unblockFrom, "from", "", "reviewer identity (defaults to agent_id or hostname)")
	rootCmd.AddCommand(unblockCmd)
}

func runUnblock(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, unblockFrom)

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status != ticket.StatusBlocked {
			return fmt.Errorf("ticket %s is not blocked (status: %s)", t.ID, t.Status)
		}
		t.Status = ticket.StatusInProgress
		t.BlockReason = ""
		t.Checkpoints = append(t.Checkpoints, newCheckpoint(fmt.Sprintf("unblocked by %s", from)))
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore unblock: %w", err)
	}

	fmt.Printf("ticket %s unblocked\n", t.ID)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketUnblocked, map[string]any{
		"ticket_id": t.ID,
		"from":      from,
	}))
	return nil
}
