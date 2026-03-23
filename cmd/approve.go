package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var approveFrom string

var approveCmd = &cobra.Command{
	Use:   "approve <ticket-id>",
	Short: "Approve a ready-for-review ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runApprove,
}

func init() {
	approveCmd.Flags().StringVar(&approveFrom, "from", "", "approver identity (defaults to agent_id or hostname)")
	rootCmd.AddCommand(approveCmd)
}

func runApprove(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, approveFrom)

	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore approve: %w", err)
	}
	if t.Status != ticket.StatusReadyForReview {
		return fmt.Errorf("lore approve: ticket %s is %s, not ready-for-review", id, t.Status)
	}

	entryID, err := ticket.NewEntryID()
	if err != nil {
		return fmt.Errorf("lore approve: %w", err)
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindComment,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      "approved",
	}
	if _, err := appendThread(ctx, gitRoot, t, entry); err != nil {
		return fmt.Errorf("lore approve: %w", err)
	}

	fmt.Printf("ticket %s approved by %s\n", id, from)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketApproved, map[string]any{
		"ticket_id": id,
		"from":      from,
	}))
	return nil
}
