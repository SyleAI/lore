package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	rejectReason string
	rejectFrom   string
)

var rejectCmd = &cobra.Command{
	Use:   "reject <ticket-id>",
	Short: "Reject a ready ticket with a reason, returning it for rework",
	Args:  cobra.ExactArgs(1),
	RunE:  runReject,
}

func init() {
	rejectCmd.Flags().StringVar(&rejectReason, "reason", "", "reason for rejection (required)")
	rejectCmd.Flags().StringVar(&rejectFrom, "from", "", "reviewer identity (defaults to agent_id or hostname)")
	_ = rejectCmd.MarkFlagRequired("reason")
	rootCmd.AddCommand(rejectCmd)
}

func runReject(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, rejectFrom)

	t, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Status == ticket.StatusClosed {
			return fmt.Errorf("ticket %s is closed (status: %s)", t.ID, t.Status)
		}
		msg := fmt.Sprintf("rejected by %s: %s", from, rejectReason)
		t.Status = ticket.StatusBlocked
		t.BlockReason = msg
		t.Checkpoints = append(t.Checkpoints, newCheckpoint(msg))
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore reject: %w", err)
	}

	fmt.Printf("ticket %s rejected: %s\n", t.ID, rejectReason)
	return nil
}
