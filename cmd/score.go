package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/graph"
	"github.com/loreteam/lore/internal/policy"
	"github.com/loreteam/lore/internal/store"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var scoreCmd = &cobra.Command{
	Use:   "score",
	Short: "Recompute system priority scores for all open tickets",
	Long: `score rebuilds the graph index and updates system priority (0-100) for
tickets that have not been pinned by a human (priority < 101).

Human-pinned tickets (priority 101-200) are never overwritten.`,
	RunE: runScore,
}

func init() {
	rootCmd.AddCommand(scoreCmd)
}

func runScore(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	pol, err := policy.Load(ctx, gitRoot)
	if err != nil {
		return fmt.Errorf("lore score: load policy: %w", err)
	}

	idx, err := graph.Build(ctx, gitRoot, pol)
	if err != nil {
		return fmt.Errorf("lore score: build graph: %w", err)
	}

	// Use the SQLite store for priorities — avoids re-reading ticket blobs from git.
	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore score: open store: %w", err)
	}
	defer s.Close()

	rows, err := s.ListTickets(store.ListFilter{})
	if err != nil {
		return fmt.Errorf("lore score: list tickets: %w", err)
	}

	updated, skipped := 0, 0
	for _, r := range rows {
		if r.Priority > 100 {
			continue // human-pinned
		}
		node, ok := idx.Tickets[r.ID]
		if !ok || r.Priority == node.Score {
			continue
		}
		score := node.Score
		_, err := casUpdate(ctx, gitRoot, r.ID, func(t *ticket.Ticket) error {
			t.Priority = score
			return nil
		})
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not update %s: %v\n", r.ID, err)
			skipped++
			continue
		}
		updated++
	}

	if err := graph.Save(ctx, gitRoot, idx); err != nil {
		return fmt.Errorf("lore score: save graph: %w", err)
	}

	fmt.Printf("scored %d tickets (%d skipped)\n", updated, skipped)

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketScored, map[string]any{
		"updated": updated,
		"skipped": skipped,
	}))
	return nil
}
