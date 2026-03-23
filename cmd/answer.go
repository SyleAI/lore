package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	answerQuestionID string
	answerText       string
	answerFrom       string
)

var answerCmd = &cobra.Command{
	Use:   "answer <ticket-id>",
	Short: "Answer a question on a ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runAnswer,
}

func init() {
	answerCmd.Flags().StringVar(&answerQuestionID, "question-id", "", "ID of the question to answer")
	answerCmd.Flags().StringVar(&answerText, "text", "", "answer text")
	answerCmd.Flags().StringVar(&answerFrom, "from", "", "agent or human answering (defaults to config agent_id or hostname)")
	_ = answerCmd.MarkFlagRequired("question-id")
	_ = answerCmd.MarkFlagRequired("text")
	rootCmd.AddCommand(answerCmd)
}

func runAnswer(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, answerFrom)

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		for i := range t.Questions {
			if t.Questions[i].ID != answerQuestionID {
				continue
			}
			if t.Questions[i].AnsweredAt != nil {
				return fmt.Errorf("question %s is already answered", answerQuestionID)
			}
			now := time.Now().UTC()
			t.Questions[i].Answer = answerText
			t.Questions[i].AnsweredBy = from
			t.Questions[i].AnsweredAt = &now

			if t.Questions[i].Blocking && t.Status == ticket.StatusBlocked {
				if !hasUnansweredBlockingQuestions(t, answerQuestionID) {
					t.Status = ticket.StatusInProgress
					t.BlockReason = ""
				}
			}
			return nil
		}
		return fmt.Errorf("question %s not found on ticket %s", answerQuestionID, id)
	})
	if err != nil {
		return fmt.Errorf("lore answer: %w", err)
	}

	fmt.Printf("answered question %s on ticket %s\n", answerQuestionID, t.ID)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventQuestionAnswered, map[string]any{
		"ticket_id":   t.ID,
		"question_id": answerQuestionID,
		"from":        from,
	}))
	return nil
}

// hasUnansweredBlockingQuestions reports whether t has any blocking questions
// that are still unanswered, excluding the question with skipID.
func hasUnansweredBlockingQuestions(t *ticket.Ticket, skipID string) bool {
	for _, q := range t.Questions {
		if q.ID == skipID {
			continue
		}
		if q.Blocking && q.AnsweredAt == nil {
			return true
		}
	}
	return false
}
