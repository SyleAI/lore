package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	prioritizeValue  int
	prioritizeUrgent bool
	prioritizeUnpin  bool
)

var prioritizeCmd = &cobra.Command{
	Use:   "prioritize <ticket-id>",
	Short: "Pin a human priority on a ticket (101-200)",
	Long: `prioritize sets a human-controlled priority on a ticket.

Human priorities (101-200) always outrank system scores (0-100).
Use --unpin to hand control back to the system scorer.

  lore prioritize <id> --priority 150   set explicit human priority
  lore prioritize <id> --urgent         shorthand for --priority 200
  lore prioritize <id> --unpin          remove human pin; system will rescore`,
	Args: cobra.ExactArgs(1),
	RunE: runPrioritize,
}

func init() {
	prioritizeCmd.Flags().IntVar(&prioritizeValue, "priority", 0, "human priority value (101-200)")
	prioritizeCmd.Flags().BoolVar(&prioritizeUrgent, "urgent", false, "shorthand for --priority 200")
	prioritizeCmd.Flags().BoolVar(&prioritizeUnpin, "unpin", false, "remove human pin and let the system rescore")
	rootCmd.AddCommand(prioritizeCmd)
}

func runPrioritize(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]

	var newPriority int
	switch {
	case prioritizeUnpin:
		newPriority = 0 // system will rescore on next lore score run
	case prioritizeUrgent:
		newPriority = 200
	case prioritizeValue != 0:
		if prioritizeValue < 101 || prioritizeValue > 200 {
			return fmt.Errorf("lore prioritize: --priority must be 101-200 (use lore score for system scoring)")
		}
		newPriority = prioritizeValue
	default:
		return fmt.Errorf("lore prioritize: provide --priority N (101-200), --urgent, or --unpin")
	}

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		t.Priority = newPriority
		if prioritizeUnpin {
			t.Checkpoints = append(t.Checkpoints, newCheckpoint("priority unpinned — returning to system scorer"))
		} else {
			t.Checkpoints = append(t.Checkpoints, newCheckpoint(fmt.Sprintf("human priority set to %d", newPriority)))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore prioritize: %w", err)
	}

	if prioritizeUnpin {
		fmt.Printf("ticket %s unpinned; run lore score to recompute\n", t.ID)
	} else {
		fmt.Printf("ticket %s priority set to %d\n", t.ID, t.Priority)
	}

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketPrioritized, map[string]any{
		"ticket_id": t.ID,
		"priority":  newPriority,
		"pinned":    !prioritizeUnpin,
	}))
	return nil
}
