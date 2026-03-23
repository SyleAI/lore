package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	tagQuestionID string
	tagAgent      string
)

var tagCmd = &cobra.Command{
	Use:   "tag <ticket-id>",
	Short: "Route an open question to a specific agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runTag,
}

func init() {
	tagCmd.Flags().StringVar(&tagQuestionID, "question-id", "", "question ID to route (required)")
	tagCmd.Flags().StringVar(&tagAgent, "agent", "", "agent to route the question to (required)")
	_ = tagCmd.MarkFlagRequired("question-id")
	_ = tagCmd.MarkFlagRequired("agent")
	rootCmd.AddCommand(tagCmd)
}

func runTag(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	qRef := ticket.QuestionRef(tagQuestionID)
	qSHA, err := gitcmd.ReadRef(ctx, gitRoot, qRef)
	if err != nil {
		return fmt.Errorf("lore tag: question %s not found", tagQuestionID)
	}
	qData, err := gitcmd.ReadBlob(ctx, gitRoot, qSHA)
	if err != nil {
		return fmt.Errorf("lore tag: read question: %w", err)
	}

	var qRecord map[string]any
	if err := json.Unmarshal(qData, &qRecord); err != nil {
		return fmt.Errorf("lore tag: parse question: %w", err)
	}
	if answered, _ := qRecord["answered"].(bool); answered {
		return fmt.Errorf("lore tag: question %s is already answered", tagQuestionID)
	}

	qRecord["directed_to"] = tagAgent
	newQData, err := json.Marshal(qRecord)
	if err != nil {
		return fmt.Errorf("lore tag: marshal question: %w", err)
	}
	newQSHA, err := gitcmd.WriteBlob(ctx, gitRoot, newQData)
	if err != nil {
		return fmt.Errorf("lore tag: write question blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, qRef, newQSHA); err != nil {
		return fmt.Errorf("lore tag: update question ref: %w", err)
	}

	ticketID, _ := qRecord["ticket_id"].(string)
	fmt.Printf("question %s on ticket %s routed to %s\n", tagQuestionID, ticketID, tagAgent)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventQuestionTagged, map[string]any{
		"ticket_id":   ticketID,
		"question_id": tagQuestionID,
		"agent":       tagAgent,
	}))
	return nil
}
