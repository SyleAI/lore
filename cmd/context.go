package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/loreteam/lore/internal/ai"
	"github.com/loreteam/lore/internal/store"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context <ticket-id>",
	Short: "Print full agent briefing for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runContext,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore context: %w", err)
	}

	printTicket(t)

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore context: %w", err)
	}
	defer s.Close()

	related, err := s.FindRelatedByFiles(id)
	if err == nil && len(related) > 0 {
		fmt.Printf("\nRelated (shared files):\n")
		for _, r := range related {
			fmt.Printf("  %-14s  [%s]  %s\n", r.ID, r.Status, r.Title)
		}
	}

	if similar := findSimilarTickets(ctx, s, id, ticketQueryText(t)); len(similar) > 0 {
		fmt.Printf("\nSemantically similar:\n")
		for _, r := range similar {
			fmt.Printf("  %-14s  [%s]  %s\n", r.ID, r.Status, r.Title)
		}
	}

	var unanswered []string
	for _, q := range t.Questions {
		if q.AnsweredAt == nil {
			blocking := ""
			if q.Blocking {
				blocking = " [BLOCKING]"
			}
			unanswered = append(unanswered, fmt.Sprintf("  %s%s  %s", q.ID, blocking, q.Text))
		}
	}
	if len(unanswered) > 0 {
		fmt.Printf("\nOpen questions:\n%s\n", strings.Join(unanswered, "\n"))
	}

	return nil
}

// findSimilarTickets returns up to 5 non-closed tickets with embedding similarity >= 0.75
// to queryText, excluding excludeID. Returns nil when AI is unavailable or no match found.
func findSimilarTickets(ctx context.Context, s *store.Store, excludeID, queryText string) []*store.Row {
	rows, err := s.ListTickets(store.ListFilter{})
	if err != nil {
		return nil
	}

	var candidates []*store.Row
	for _, r := range rows {
		if r.ID != excludeID && r.Status != "closed" {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	embedder := newEmbedder(ctx)

	texts := make([]string, 1, len(candidates)+1)
	texts[0] = queryText

	var staleBatch []int // indices into candidates needing embedding
	cachedVecs := make([]ai.Embedding, len(candidates))
	for i, r := range candidates {
		blob, err := s.GetEmbedding(r.ID, r.SHA)
		if err != nil || blob == nil {
			staleBatch = append(staleBatch, i)
			texts = append(texts, r.Title) // title only; description not in store Row
		} else {
			vec, err := ai.DecodeVector(blob)
			if err != nil {
				staleBatch = append(staleBatch, i)
				texts = append(texts, r.Title)
			} else {
				cachedVecs[i] = vec
			}
		}
	}

	vecs, err := embedder.Embed(texts)
	if err != nil {
		return nil // AI unavailable — skip silently
	}

	queryVec := vecs[0]

	allVecs := make([]ai.Embedding, len(candidates))
	staleIdx := 1 // vecs[0] is query; vecs[1..] are stale candidates in order
	for i := range candidates {
		if cachedVecs[i] != nil {
			allVecs[i] = cachedVecs[i]
		} else {
			allVecs[i] = vecs[staleIdx]
			_ = s.SetEmbedding(candidates[i].ID, candidates[i].SHA, ai.EncodeVector(vecs[staleIdx]))
			staleIdx++
		}
	}

	type scored struct {
		row *store.Row
		sim float64
	}
	var results []scored
	for i, r := range candidates {
		if sim := ai.CosineSimilarity(queryVec, allVecs[i]); sim >= 0.75 {
			results = append(results, scored{r, sim})
		}
	}

	// insertion sort — small N
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].sim > results[j-1].sim; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	const maxSimilar = 5
	out := make([]*store.Row, 0, min(maxSimilar, len(results)))
	for i := 0; i < len(results) && i < maxSimilar; i++ {
		out = append(out, results[i].row)
	}
	return out
}
