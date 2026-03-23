package cmd

import (
	"fmt"
	"math"
	"time"

	"github.com/loreteam/lore/internal/graph"
	"github.com/loreteam/lore/internal/policy"
	"github.com/loreteam/lore/internal/store"
	"github.com/spf13/cobra"
)

var whyCmd = &cobra.Command{
	Use:   "why <ticket-id>",
	Short: "Show priority score breakdown for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runWhy,
}

func init() {
	rootCmd.AddCommand(whyCmd)
}

func runWhy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore why: %w", err)
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore why: %w", err)
	}
	defer s.Close()

	blockingForThis, _ := s.CountQuestions(store.QuestionFilter{
		Unanswered:  true,
		BlockingOnly: true,
		TicketID:    id,
	})

	fmt.Printf("Priority breakdown for %s: %s\n\n", t.ID, t.Title)
	fmt.Printf("  Stored priority:       %d", t.Priority)
	if t.Priority > 100 {
		fmt.Printf("  (human-pinned)\n")
	} else {
		fmt.Printf("  (system-scored)\n")
	}
	fmt.Printf("  Status:                %s\n", t.Status)
	fmt.Printf("  Attempts:              %d\n", t.Attempts)
	fmt.Printf("  Age:                   %dd\n", ageDays(t.CreatedAt))
	fmt.Printf("  Blocking questions:    %d unanswered\n", blockingForThis)
	if t.BlockReason != "" {
		fmt.Printf("  Block reason:          %s\n", t.BlockReason)
	}
	if t.Parent != "" {
		fmt.Printf("  Parent:                %s\n", t.Parent)
	}

	// Show graph score factors if the graph index has an entry for this ticket.
	idx, idxErr := graph.Load(ctx, gitRoot)
	pol, polErr := policy.Load(ctx, gitRoot)
	if idxErr == nil && polErr == nil {
		if node, ok := idx.Tickets[id]; ok {
			sp := pol.Scoring

			maxHeat := 0.0
			for _, f := range t.Files {
				if fn, ok := idx.Files[f]; ok && fn.Heat > maxHeat {
					maxHeat = fn.Heat
				}
			}
			unblocksCount := len(node.Blocks)
			ageDays := time.Since(t.CreatedAt).Hours() / 24
			penalty := math.Min(float64(t.Attempts)*sp.AttemptsPenalty, sp.MaxAttemptsPenalty)

			fileHeatContrib := maxHeat * sp.FileHeatWeight
			unblocksContrib := float64(unblocksCount) * sp.UnblocksWeight
			ageContrib := ageDays * sp.AgeDayWeight

			fmt.Printf("\n  Score factors (graph):\n")
			fmt.Printf("    file heat:    %.1f  (max heat %.2f × weight %.0f)\n", fileHeatContrib, maxHeat, sp.FileHeatWeight)
			fmt.Printf("    unblocks:     %.1f  (%d tickets × weight %.0f)\n", unblocksContrib, unblocksCount, sp.UnblocksWeight)
			fmt.Printf("    age:          %.1f  (%.0fd × weight %.2f)\n", ageContrib, ageDays, sp.AgeDayWeight)
			fmt.Printf("    attempts:    -%.1f  (%d × penalty %.0f, cap %.0f)\n", penalty, t.Attempts, sp.AttemptsPenalty, sp.MaxAttemptsPenalty)

			raw := fileHeatContrib + unblocksContrib + ageContrib - penalty
			computed := int(math.Round(math.Max(0, math.Min(100, raw))))
			fmt.Printf("    computed:     %d\n", computed)

			if node.ClusterID != "" {
				fmt.Printf("\n  Cluster: %s\n", node.ClusterID)
			}
		} else {
			fmt.Printf("\n  (run lore graph update or lore score to populate graph factors)\n")
		}
	}

	return nil
}
