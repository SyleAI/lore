package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	assignAgent  string
	assignReason string
)

var assignCmd = &cobra.Command{
	Use:   "assign <ticket-id>",
	Short: "Assign a ticket to an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAssign,
}

func init() {
	assignCmd.Flags().StringVar(&assignAgent, "agent", "", "agent ID to assign (required)")
	assignCmd.Flags().StringVar(&assignReason, "reason", "", "reason for this assignment")
	_ = assignCmd.MarkFlagRequired("agent")
	rootCmd.AddCommand(assignCmd)
}

func runAssign(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, "")

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status == ticket.StatusDone {
			return fmt.Errorf("ticket %s is done", t.ID)
		}
		t.Agent = assignAgent
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore assign: %w", err)
	}

	text := fmt.Sprintf("assigned to %s", assignAgent)
	if assignReason != "" {
		text += ": " + assignReason
	}
	entryID, err := ticket.NewEntryID()
	if err != nil {
		return fmt.Errorf("lore assign: %w", err)
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindComment,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      text,
	}
	if _, err := appendThread(ctx, gitRoot, t, entry); err != nil {
		return fmt.Errorf("lore assign: %w", err)
	}

	fmt.Printf("ticket %s assigned to %s\n", t.ID, assignAgent)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketAssigned, map[string]any{
		"ticket_id": t.ID,
		"agent":     assignAgent,
	}))
	return nil
}
