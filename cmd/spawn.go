package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var spawnCmd = &cobra.Command{
	Use:   "spawn <parent-id> <description>",
	Short: "Create a child ticket under a parent",
	Args:  cobra.ExactArgs(2),
	RunE:  runSpawn,
}

func init() {
	rootCmd.AddCommand(spawnCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	parentID, desc := args[0], args[1]

	if _, err := loadTicket(ctx, gitRoot, parentID); err != nil {
		return fmt.Errorf("lore spawn: parent %w", err)
	}

	id, err := ticket.NewID()
	if err != nil {
		return fmt.Errorf("lore spawn: generate id: %w", err)
	}

	now := time.Now().UTC()
	child := &ticket.Ticket{
		ID:        id,
		Desc:      desc,
		Status:    ticket.StatusOpen,
		Parent:    parentID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := saveTicket(ctx, gitRoot, child); err != nil {
		return fmt.Errorf("lore spawn: %w", err)
	}

	fmt.Printf("spawned %s (parent: %s)\n", child.ID, parentID)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketCreated, map[string]any{
		"id":     child.ID,
		"parent": parentID,
	}))
	return nil
}
