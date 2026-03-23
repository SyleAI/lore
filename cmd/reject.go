package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	rejectReason string
	rejectFrom   string
)

var rejectCmd = &cobra.Command{
	Use:   "reject <ticket-id>",
	Short: "Reject a ready-for-review ticket, returning it for rework",
	Args:  cobra.ExactArgs(1),
	RunE:  runReject,
}

func init() {
	rejectCmd.Flags().StringVar(&rejectReason, "reason", "", "reason for rejection (required)")
	rejectCmd.Flags().StringVar(&rejectFrom, "from", "", "reviewer identity (defaults to agent_id or hostname)")
	_ = rejectCmd.MarkFlagRequired("reason")
	rootCmd.AddCommand(rejectCmd)
}

func runReject(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, rejectFrom)

	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore reject: %w", err)
	}
	if t.Status != ticket.StatusReadyForReview {
		return fmt.Errorf("lore reject: ticket %s is %s, not ready-for-review", id, t.Status)
	}

	// Append rejection comment to thread.
	entryID, err := ticket.NewEntryID()
	if err != nil {
		return fmt.Errorf("lore reject: %w", err)
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindComment,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      fmt.Sprintf("rejected: %s", rejectReason),
	}
	if _, err := appendThread(ctx, gitRoot, t, entry); err != nil {
		return fmt.Errorf("lore reject: %w", err)
	}

	// Return ticket to working status.
	if _, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusWorking
		t.BlockReason = ""
		return nil
	}); err != nil {
		return fmt.Errorf("lore reject: %w", err)
	}

	fmt.Printf("ticket %s rejected: %s\n", id, rejectReason)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketRejected, map[string]any{
		"ticket_id": id,
		"from":      from,
		"reason":    rejectReason,
	}))
	return nil
}
