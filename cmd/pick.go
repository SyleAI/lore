package cmd

import (
	"errors"
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/store"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var pickAgent string

var pickCmd = &cobra.Command{
	Use:   "pick",
	Short: "Atomically claim the highest-priority open ticket",
	RunE:  runPick,
}

func init() {
	pickCmd.Flags().StringVar(&pickAgent, "agent", "", "agent ID (defaults to config agent_id or hostname)")
	rootCmd.AddCommand(pickCmd)
}

var errTicketUnavailable = errors.New("ticket unavailable")

func runPick(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore pick: %w", err)
	}
	rows, err := s.ListTickets(store.ListFilter{Status: string(ticket.StatusOpen)})
	s.Close()
	if err != nil {
		return fmt.Errorf("lore pick: %w", err)
	}
	if len(rows) == 0 {
		fmt.Println("no open tickets available")
		return nil
	}

	agent := agentID(ctx, pickAgent)

	for _, row := range rows {
		t, casErr := casUpdate(ctx, gitRoot, row.ID, func(t *ticket.Ticket) error {
			if t.Status != ticket.StatusOpen {
				return fmt.Errorf("%w: %s is %s", errTicketUnavailable, t.ID, t.Status)
			}
			t.Status = ticket.StatusInProgress
			t.Agent = agent
			t.Attempts++
			return nil
		})
		if casErr != nil {
			// Skip tickets that are no longer open or have too much contention.
			if errors.Is(casErr, errTicketUnavailable) || errors.Is(casErr, errCASExhausted) {
				continue
			}
			return fmt.Errorf("lore pick: %w", casErr)
		}
		printTicket(t)
		em := event.EmitterFromContext(ctx)
		em.Emit(ctx, event.New(event.EventTicketClaimed, map[string]any{
			"id":    t.ID,
			"agent": agent,
		}))
		return nil
	}

	fmt.Println("no open tickets could be claimed (all taken by other agents)")
	return nil
}
