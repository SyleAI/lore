package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	askFrom  string
	askBlock bool
)

var askCmd = &cobra.Command{
	Use:   "ask <ticket-id> <question>",
	Short: "Post a question on a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runAsk,
}

func init() {
	askCmd.Flags().StringVar(&askFrom, "from", "", "agent asking (defaults to config agent_id or hostname)")
	askCmd.Flags().BoolVar(&askBlock, "block", false, "block the ticket until this question is answered")
	rootCmd.AddCommand(askCmd)
}

func runAsk(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	questionText := args[1]
	from := agentID(ctx, askFrom)

	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore ask: %w", err)
	}
	if t.Status == ticket.StatusDone {
		return fmt.Errorf("lore ask: ticket %s is done", id)
	}

	qid, err := ticket.NewQuestionID()
	if err != nil {
		return fmt.Errorf("lore ask: %w", err)
	}
	entryID, err := ticket.NewEntryID()
	if err != nil {
		return fmt.Errorf("lore ask: %w", err)
	}

	// Append question entry to thread.
	entry := &ticket.ThreadEntry{
		ID:         entryID,
		Kind:       ticket.EntryKindQuestion,
		Author:     from,
		Timestamp:  time.Now().UTC(),
		Text:       questionText,
		QuestionID: qid,
	}
	t, err = appendThread(ctx, gitRoot, t, entry)
	if err != nil {
		return fmt.Errorf("lore ask: %w", err)
	}

	// Optionally block the ticket.
	if askBlock {
		t, err = casUpdate(ctx, gitRoot, t.ID, func(t *ticket.Ticket) error {
			t.Status = ticket.StatusBlocked
			t.BlockReason = fmt.Sprintf("question %s: %s", qid, questionText)
			return nil
		})
		if err != nil {
			return fmt.Errorf("lore ask: block ticket: %w", err)
		}
	}

	// Write question index entry.
	qRecord, err := json.Marshal(map[string]any{
		"qid":       qid,
		"ticket_id": id,
		"text":      questionText,
		"asked_at":  time.Now().UTC(),
		"answered":  false,
		"blocking":  askBlock,
	})
	if err != nil {
		return fmt.Errorf("lore ask: marshal question record: %w", err)
	}
	qSHA, err := gitcmd.WriteBlob(ctx, gitRoot, qRecord)
	if err != nil {
		return fmt.Errorf("lore ask: write question blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, ticket.QuestionRef(qid), qSHA); err != nil {
		return fmt.Errorf("lore ask: write question ref: %w", err)
	}

	fmt.Printf("question %s posted on ticket %s\n", qid, t.ID)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventQuestionAsked, map[string]any{
		"ticket_id":   t.ID,
		"question_id": qid,
		"blocking":    askBlock,
	}))
	return nil
}
