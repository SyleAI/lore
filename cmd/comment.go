package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var commentFrom string

var commentCmd = &cobra.Command{
	Use:   "comment <ticket-id> <message>",
	Short: "Add a human comment to a ticket without changing its status",
	Args:  cobra.ExactArgs(2),
	RunE:  runComment,
}

func init() {
	commentCmd.Flags().StringVar(&commentFrom, "from", "", "commenter identity (defaults to agent_id or hostname)")
	rootCmd.AddCommand(commentCmd)
}

func runComment(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	message := args[1]
	from := agentID(ctx, commentFrom)

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		t.Checkpoints = append(t.Checkpoints, newCheckpoint(fmt.Sprintf("[%s] %s", from, message)))
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore comment: %w", err)
	}

	fmt.Printf("comment added to ticket %s\n", t.ID)
	return nil
}
