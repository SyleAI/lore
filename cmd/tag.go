package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticketops"
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

	qRecord, err := ticketops.LoadQuestion(gitRoot, tagQuestionID)
	if err != nil {
		return fmt.Errorf("lore tag: question %s not found", tagQuestionID)
	}
	if answered, _ := qRecord["answered"].(bool); answered {
		return fmt.Errorf("lore tag: question %s is already answered", tagQuestionID)
	}

	qRecord["directed_to"] = tagAgent
	if err := ticketops.SaveQuestion(gitRoot, qRecord); err != nil {
		return fmt.Errorf("lore tag: update question: %w", err)
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
