package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/store"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review [ticket-id]",
	Short: "Show everything awaiting human decision",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReview,
}

func init() {
	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	// If a specific ticket ID was given, show full detail.
	if len(args) == 1 {
		t, err := loadTicket(ctx, gitRoot, args[0])
		if err != nil {
			return fmt.Errorf("lore review: %w", err)
		}
		printTicket(t)
		return nil
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore review: %w", err)
	}
	defer s.Close()

	// Escalated tickets (blocked).
	blocked, err := s.ListTickets(store.ListFilter{Status: string(ticket.StatusBlocked)})
	if err != nil {
		return fmt.Errorf("lore review: %w", err)
	}

	// Ready tickets awaiting approval.
	ready, err := s.ListTickets(store.ListFilter{Status: string(ticket.StatusReady)})
	if err != nil {
		return fmt.Errorf("lore review: %w", err)
	}

	// Blocking unanswered questions.
	blockingQ, err := s.ListQuestions(store.QuestionFilter{Unanswered: true, BlockingOnly: true})
	if err != nil {
		return fmt.Errorf("lore review: %w", err)
	}

	if len(blocked)+len(ready)+len(blockingQ) == 0 {
		fmt.Println("nothing requires human attention")
		return nil
	}

	printTicketSection("Ready for approval", ready)
	printTicketSection("Escalated / blocked", blocked)

	if len(blockingQ) > 0 {
		fmt.Printf("Blocking unanswered questions (%d):\n", len(blockingQ))
		fmt.Printf("  %-14s  %-8s  %s\n", "TICKET", "Q-ID", "QUESTION")
		for _, q := range blockingQ {
			text := q.Text
			if len(text) > 60 {
				text = text[:57] + "..."
			}
			fmt.Printf("  %-14s  %-8s  %s\n", q.TicketID, q.ID, text)
		}
	}

	return nil
}

func printTicketSection(label string, rows []*store.Row) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("%s (%d):\n", label, len(rows))
	fmt.Printf("  %-14s  %s\n", "TICKET", "TITLE")
	for _, r := range rows {
		fmt.Printf("  %-14s  %s\n", r.ID, r.Title)
	}
	fmt.Println()
}

