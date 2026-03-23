package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Close a ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runClose,
}

func init() {
	rootCmd.AddCommand(closeCmd)
}

func runClose(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	if _, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusClosed
		return nil
	}); err != nil {
		return fmt.Errorf("lore close: %w", err)
	}
	fmt.Printf("ticket %s closed\n", id)
	return nil
}
