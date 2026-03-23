package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/gitcmd"
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
		entries, _ := loadThread(ctx, gitRoot, t)
		printTicket(t, entries)
		return nil
	}

	tickets, err := listTickets(ctx, gitRoot, false)
	if err != nil {
		return fmt.Errorf("lore review: %w", err)
	}

	var readyForReview, blocked []*ticket.Ticket
	for _, t := range tickets {
		switch t.Status {
		case ticket.StatusReadyForReview:
			readyForReview = append(readyForReview, t)
		case ticket.StatusBlocked:
			blocked = append(blocked, t)
		}
	}

	// Scan questions index for unanswered questions.
	questionRefs, _ := gitcmd.ListRefs(ctx, gitRoot, "refs/tickets/questions/")
	type questionEntry struct {
		qid      string
		ticketID string
		text     string
	}
	var openQuestions []questionEntry
	for _, sha := range questionRefs {
		data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
		if err != nil {
			continue
		}
		var q struct {
			QID      string `json:"qid"`
			TicketID string `json:"ticket_id"`
			Text     string `json:"text"`
			Answered bool   `json:"answered"`
		}
		if err := parseJSON(data, &q); err != nil || q.Answered {
			continue
		}
		openQuestions = append(openQuestions, questionEntry{q.QID, q.TicketID, q.Text})
	}

	if len(readyForReview)+len(blocked)+len(openQuestions) == 0 {
		fmt.Println("nothing requires human attention")
		return nil
	}

	printTicketGroup("Ready for review", readyForReview)
	printTicketGroup("Blocked / escalated", blocked)

	if len(openQuestions) > 0 {
		fmt.Printf("Open questions (%d):\n", len(openQuestions))
		fmt.Printf("  %-14s  %-8s  %s\n", "TICKET", "Q-ID", "QUESTION")
		for _, q := range openQuestions {
			fmt.Printf("  %-14s  %-8s  %s\n", q.ticketID, q.qid, truncate(q.text, 60))
		}
		fmt.Println()
	}

	return nil
}

func printTicketGroup(label string, tickets []*ticket.Ticket) {
	if len(tickets) == 0 {
		return
	}
	fmt.Printf("%s (%d):\n", label, len(tickets))
	fmt.Printf("  %-14s  %s\n", "TICKET", "DESCRIPTION")
	for _, t := range tickets {
		fmt.Printf("  %-14s  %s\n", t.ID, truncate(t.Desc, 60))
	}
	fmt.Println()
}
