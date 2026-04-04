package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/loreteam/lore/internal/ticketops"
	"github.com/spf13/cobra"
)

var (
	answerQuestionID string
	answerFrom       string
)

var answerCmd = &cobra.Command{
	Use:   "answer <ticket-id> <answer-text>",
	Short: "Answer a question on a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runAnswer,
}

func init() {
	answerCmd.Flags().StringVar(&answerQuestionID, "question-id", "", "ID of the question to answer")
	answerCmd.Flags().StringVar(&answerFrom, "from", "", "agent or human answering (defaults to config agent_id or hostname)")
	_ = answerCmd.MarkFlagRequired("question-id")
	rootCmd.AddCommand(answerCmd)
}

func runAnswer(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	answerText := args[1]
	from := agentID(ctx, answerFrom)

	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore answer: %w", err)
	}

	// Verify the question exists and belongs to this ticket.
	qRecord, err := ticketops.LoadQuestion(gitRoot, answerQuestionID)
	if err != nil {
		return fmt.Errorf("lore answer: question %s not found", answerQuestionID)
	}
	ticketID, _ := qRecord["ticket_id"].(string)
	if ticketID != id {
		return fmt.Errorf("lore answer: question %s belongs to ticket %s, not %s", answerQuestionID, ticketID, id)
	}
	if answered, _ := qRecord["answered"].(bool); answered {
		return fmt.Errorf("lore answer: question %s is already answered", answerQuestionID)
	}

	// Append answer entry to thread.
	entryID, err := ticket.NewEntryID()
	if err != nil {
		return fmt.Errorf("lore answer: %w", err)
	}
	entry := &ticket.ThreadEntry{
		ID:         entryID,
		Kind:       ticket.EntryKindAnswer,
		Author:     from,
		Timestamp:  time.Now().UTC(),
		Text:       answerText,
		QuestionID: answerQuestionID,
	}
	t, err = appendThread(ctx, gitRoot, t, entry)
	if err != nil {
		return fmt.Errorf("lore answer: %w", err)
	}

	// Mark question as answered.
	qRecord["answered"] = true
	if err := ticketops.SaveQuestion(gitRoot, qRecord); err != nil {
		return fmt.Errorf("lore answer: update question: %w", err)
	}

	// If ticket is blocked, unblock it now that the question is answered.
	if t.Status == ticket.StatusBlocked {
		_, _ = casUpdate(ctx, gitRoot, t.ID, func(t *ticket.Ticket) error {
			t.Status = ticket.StatusWorking
			t.BlockReason = ""
			return nil
		})
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
