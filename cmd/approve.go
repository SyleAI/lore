package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var approveFrom string

var approveCmd = &cobra.Command{
	Use:   "approve <ticket-id>",
	Short: "Approve a ready ticket for merge",
	Args:  cobra.ExactArgs(1),
	RunE:  runApprove,
}

func init() {
	approveCmd.Flags().StringVar(&approveFrom, "from", "", "approver identity (defaults to agent_id or hostname)")
	rootCmd.AddCommand(approveCmd)
}

func runApprove(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, approveFrom)

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status != ticket.StatusReady {
			return fmt.Errorf("ticket %s is not ready (status: %s)", t.ID, t.Status)
		}
		t.Checkpoints = append(t.Checkpoints, newCheckpoint(fmt.Sprintf("approved by %s", from)))
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore approve: %w", err)
	}

	fmt.Printf("ticket %s approved by %s\n", t.ID, from)
	return nil
}
