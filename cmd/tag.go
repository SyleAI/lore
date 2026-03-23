package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/event"
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

	id := args[0]
	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		for i, q := range t.Questions {
			if q.ID == tagQuestionID {
				if q.AnsweredAt != nil {
					return fmt.Errorf("question %s is already answered", tagQuestionID)
				}
				t.Questions[i].DirectedTo = tagAgent
				return nil
			}
		}
		return fmt.Errorf("question %s not found on ticket %s", tagQuestionID, t.ID)
	})
	if err != nil {
		return fmt.Errorf("lore tag: %w", err)
	}

	fmt.Printf("question %s on ticket %s routed to %s\n", tagQuestionID, t.ID, tagAgent)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventQuestionTagged, map[string]any{
		"ticket_id":   t.ID,
		"question_id": tagQuestionID,
		"agent":       tagAgent,
	}))
	return nil
}
