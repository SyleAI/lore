package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var checkpointCmd = &cobra.Command{
	Use:   "checkpoint <id> <message>",
	Short: "Append a timestamped progress note to a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runCheckpoint,
}

func init() {
	rootCmd.AddCommand(checkpointCmd)
}

func runCheckpoint(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id, msg := args[0], args[1]

	if _, err := casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		t.Checkpoints = append(t.Checkpoints, ticket.Checkpoint{
			Timestamp: time.Now().UTC(),
			Message:   msg,
		})
		return nil
	}); err != nil {
		return fmt.Errorf("lore checkpoint: %w", err)
	}

	fmt.Printf("checkpoint recorded on %s\n", id)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketCheckpointed, map[string]any{"id": id, "msg": msg}))
	return nil
}
