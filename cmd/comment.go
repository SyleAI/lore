package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var commentFrom string

var commentCmd = &cobra.Command{
	Use:   "comment <ticket-id> <message>",
	Short: "Add a comment to a ticket thread",
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

	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore comment: %w", err)
	}

	entryID, _ := ticket.NewEntryID()
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindComment,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      message,
	}
	if _, err := appendThread(ctx, gitRoot, t, entry); err != nil {
		return fmt.Errorf("lore comment: %w", err)
	}

	fmt.Printf("comment added to ticket %s\n", id)
	return nil
}
