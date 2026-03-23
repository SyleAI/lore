package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/store"
	"github.com/spf13/cobra"
)

var (
	questionsUnanswered bool
	questionsMine       bool
)

var questionsCmd = &cobra.Command{
	Use:   "questions",
	Short: "List questions across all tickets",
	RunE:  runQuestions,
}

func init() {
	questionsCmd.Flags().BoolVar(&questionsUnanswered, "unanswered", false, "show only unanswered questions")
	questionsCmd.Flags().BoolVar(&questionsMine, "mine", false, "show only questions directed at you")
	rootCmd.AddCommand(questionsCmd)
}

func runQuestions(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore questions: %w", err)
	}
	defer s.Close()

	f := store.QuestionFilter{Unanswered: questionsUnanswered}
	if questionsMine {
		f.DirectedTo = agentID(ctx, "")
	}

	rows, err := s.ListQuestions(f)
	if err != nil {
		return fmt.Errorf("lore questions: %w", err)
	}

	if len(rows) == 0 {
		fmt.Println("no questions found")
		return nil
	}

	fmt.Printf("%-14s  %-8s  %-16s  %-8s  %s\n", "TICKET", "Q-ID", "TYPE", "BLOCKING", "TEXT")
	fmt.Printf("%-14s  %-8s  %-16s  %-8s  %s\n", "--------------", "--------", "----------------", "--------", "----")
	for _, r := range rows {
		blocking := "no"
		if r.Blocking {
			blocking = "yes"
		}
		text := r.Text
		if len(text) > 50 {
			text = text[:47] + "..."
		}
		answered := ""
		if r.AnsweredAt != nil {
			answered = " [answered]"
		}
		fmt.Printf("%-14s  %-8s  %-16s  %-8s  %s%s\n",
			r.TicketID, r.ID, r.Type, blocking, text, answered)
	}
	return nil
}
