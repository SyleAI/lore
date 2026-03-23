package graph

import (
	"context"
	"math"
	"time"

	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/policy"
	"github.com/loreteam/lore/internal/ticket"
)

// Build rebuilds the entire graph index from git refs.
// It recomputes file heat, blocking relationships, and scores for all tickets.
func Build(ctx context.Context, gitRoot string, pol *policy.Policy) (*Index, error) {
	idx := Empty()

	refs, err := gitcmd.ListRefs(ctx, gitRoot, "refs/tickets/t/")
	if err != nil {
		return nil, err
	}

	// Collect all tickets.
	tickets := make([]*ticket.Ticket, 0, len(refs))
	for _, sha := range refs {
		data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
		if err != nil {
			continue
		}
		t, err := ticket.Unmarshal(data)
		if err != nil {
			continue
		}
		tickets = append(tickets, t)
	}

	// Build file → ticket index and count file appearances (for heat).
	fileCount := make(map[string]int)
	for _, t := range tickets {
		for _, f := range t.Files {
			fileCount[f]++
		}
	}

	// Normalise heat: 1.0 = file touched by the most tickets.
	maxCount := 0
	for _, c := range fileCount {
		if c > maxCount {
			maxCount = c
		}
	}

	for f, c := range fileCount {
		heat := 0.0
		if maxCount > 0 {
			heat = float64(c) / float64(maxCount)
		}
		idx.Files[f] = &FileNode{Heat: heat}
	}

	// Build ticket nodes and record which files they touch.
	for _, t := range tickets {
		node := &TicketNode{Files: t.Files}
		idx.Tickets[t.ID] = node

		// Register ticket in file nodes.
		for _, f := range t.Files {
			if fn, ok := idx.Files[f]; ok {
				fn.Tickets = append(fn.Tickets, t.ID)
			}
		}
	}

	// Populate blocking relationships from Parent links.
	// A child ticket is blocked by its parent; the parent unblocks the child.
	for _, t := range tickets {
		if t.Parent == "" {
			continue
		}
		childNode, childOK := idx.Tickets[t.ID]
		parentNode, parentOK := idx.Tickets[t.Parent]
		if !childOK || !parentOK {
			continue
		}
		// Child is blocked by parent.
		if !contains(childNode.BlockedBy, t.Parent) {
			childNode.BlockedBy = append(childNode.BlockedBy, t.Parent)
		}
		// Parent unblocks child (completing parent unblocks child ticket).
		if !contains(parentNode.Blocks, t.ID) {
			parentNode.Blocks = append(parentNode.Blocks, t.ID)
		}
	}

	// Score all tickets.
	ScoreAll(idx, tickets, pol)

	return idx, nil
}

// ScoreAll computes system priority scores (0-100) for all tickets and updates
// both the graph index and the ticket nodes.
func ScoreAll(idx *Index, tickets []*ticket.Ticket, pol *policy.Policy) {
	for _, t := range tickets {
		score := scoreTicket(t, idx, pol)
		if node, ok := idx.Tickets[t.ID]; ok {
			node.Score = score
		}
	}
}

// UpdateTicket recomputes the score for a single ticket and updates the graph node.
func UpdateTicket(idx *Index, t *ticket.Ticket, pol *policy.Policy) int {
	score := scoreTicket(t, idx, pol)
	if node, ok := idx.Tickets[t.ID]; ok {
		node.Score = score
	} else {
		idx.Tickets[t.ID] = &TicketNode{Score: score, Files: t.Files}
	}
	return score
}

// scoreTicket computes the raw score for a single ticket using the formula:
//
//	raw = (maxFileHeat × FileHeatWeight) + (unblocksCount × UnblocksWeight)
//	      + (ageDays × AgeDayWeight) - min(attempts × AttemptsPenalty, MaxAttemptsPenalty)
//
// Result is clamped to [0, 100].
func scoreTicket(t *ticket.Ticket, idx *Index, pol *policy.Policy) int {
	sp := pol.Scoring

	// Max file heat across all files this ticket touches.
	maxHeat := 0.0
	for _, f := range t.Files {
		if fn, ok := idx.Files[f]; ok && fn.Heat > maxHeat {
			maxHeat = fn.Heat
		}
	}

	// Count how many tickets this ticket unblocks (tickets that list this as parent).
	unblocksCount := 0
	if node, ok := idx.Tickets[t.ID]; ok {
		unblocksCount = len(node.Blocks)
	}

	ageDays := time.Since(t.CreatedAt).Hours() / 24

	penalty := math.Min(float64(t.Attempts)*sp.AttemptsPenalty, sp.MaxAttemptsPenalty)

	raw := (maxHeat * sp.FileHeatWeight) +
		(float64(unblocksCount) * sp.UnblocksWeight) +
		(ageDays * sp.AgeDayWeight) -
		penalty

	clamped := math.Max(0, math.Min(100, raw))
	return int(math.Round(clamped))
}

// contains reports whether s is in the slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
