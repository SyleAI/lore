package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var claimAgent string

var claimCmd = &cobra.Command{
	Use:   "claim <id>",
	Short: "Atomically claim a ticket for execution",
	Args:  cobra.ExactArgs(1),
	RunE:  runClaim,
}

func init() {
	claimCmd.Flags().StringVar(&claimAgent, "agent", "", "agent ID (defaults to config agent_id or hostname)")
	rootCmd.AddCommand(claimCmd)
}

func runClaim(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	agent := agentID(ctx, claimAgent)

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status != ticket.StatusOpen {
			return fmt.Errorf("ticket %s is %s, not open", t.ID, t.Status)
		}
		t.Status = ticket.StatusWorking
		t.Agent = agent
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore claim: %w", err)
	}

	// Append claim entry to thread.
	entryID, _ := ticket.NewEntryID()
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindUpdate,
		Author:    agent,
		Timestamp: time.Now().UTC(),
		Text:      fmt.Sprintf("claimed by %s", agent),
	}
	_, _ = appendThread(ctx, gitRoot, t, entry)

	fmt.Printf("ticket %s claimed by %s\n", t.ID, agent)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketClaimed, map[string]any{
		"id":    t.ID,
		"agent": agent,
	}))
	return nil
}
