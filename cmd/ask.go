package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/ai"
	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/store"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	askType       string
	askText       string
	askAssumption string
	askDirectedTo string
	askBlocking   bool
)

var askCmd = &cobra.Command{
	Use:   "ask <ticket-id>",
	Short: "Record a question on a ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runAsk,
}

func init() {
	askCmd.Flags().StringVar(&askType, "type", "", "question type: clarification, constraint_check, knowledge_gap, validation")
	askCmd.Flags().StringVar(&askText, "text", "", "question text")
	askCmd.Flags().StringVar(&askAssumption, "assumption", "", "assumption the agent is proceeding under (for non-blocking questions)")
	askCmd.Flags().StringVar(&askDirectedTo, "directed-to", "", "agent or role to direct the question to")
	askCmd.Flags().BoolVar(&askBlocking, "blocking", false, "block ticket progress until answered")
	_ = askCmd.MarkFlagRequired("type")
	_ = askCmd.MarkFlagRequired("text")
	rootCmd.AddCommand(askCmd)
}

func runAsk(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	if !ticket.ValidQuestionType(ticket.QuestionType(askType)) {
		return fmt.Errorf("lore ask: invalid type %q (valid: clarification, constraint_check, knowledge_gap, validation)", askType)
	}

	id := args[0]

	qid, err := ticket.NewQuestionID()
	if err != nil {
		return fmt.Errorf("lore ask: %w", err)
	}

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status == ticket.StatusClosed {
			return fmt.Errorf("ticket %s is closed", t.ID)
		}
		t.Questions = append(t.Questions, ticket.Question{
			ID:         qid,
			Type:       ticket.QuestionType(askType),
			Text:       askText,
			Blocking:   askBlocking,
			Assumption: askAssumption,
			DirectedTo: askDirectedTo,
			AskedAt:    time.Now().UTC(),
		})
		if askBlocking {
			t.Status = ticket.StatusBlocked
			t.BlockReason = fmt.Sprintf("blocking question %s: %s", qid, askText)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore ask: %w", err)
	}

	fmt.Printf("question %s recorded on ticket %s\n", qid, t.ID)

	if askType == string(ticket.QuestionTypeKnowledgeGap) {
		if suggestion := findAnsweredSimilar(ctx, gitRoot, askText); suggestion != "" {
			fmt.Printf("hint: similar answered question found — %s\n", suggestion)
		}
	}

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventQuestionAsked, map[string]any{
		"ticket_id":   t.ID,
		"question_id": qid,
		"type":        askType,
		"blocking":    askBlocking,
	}))
	return nil
}

const knowledgeGapSimilarityThreshold = 0.82

// findAnsweredSimilar embeds the question text and compares it against all
// answered questions in the store. Returns a hint string if a match is found,
// or an empty string if AI is unavailable or no match exceeds the threshold.
func findAnsweredSimilar(ctx context.Context, gitRoot, questionText string) string {
	s, err := openStore(gitRoot)
	if err != nil {
		return ""
	}
	defer s.Close()

	candidates, err := s.ListQuestions(store.QuestionFilter{Answered: true})
	if err != nil || len(candidates) == 0 {
		return ""
	}

	embedder := newEmbedder(ctx)

	texts := make([]string, len(candidates)+1)
	texts[0] = questionText
	for i, c := range candidates {
		texts[i+1] = c.Text
	}

	vecs, err := embedder.Embed(texts)
	if err != nil {
		return "" // AI unavailable — skip silently
	}

	queryVec := vecs[0]
	var bestSim float64
	var best *store.QuestionRow
	for i, c := range candidates {
		sim := ai.CosineSimilarity(queryVec, vecs[i+1])
		if sim > bestSim {
			bestSim = sim
			best = c
		}
	}

	if best == nil || bestSim < knowledgeGapSimilarityThreshold {
		return ""
	}
	return fmt.Sprintf("%q (answered: %q)", best.Text, best.Answer)
}
