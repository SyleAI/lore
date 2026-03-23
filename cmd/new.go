package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var newFrom string

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new ticket",
	Long: `Create a new ticket. Without flags, opens an interactive prompt.
Use --from to provide a description directly on the command line.`,
	RunE: runNew,
}

func init() {
	newCmd.Flags().StringVar(&newFrom, "from", "", "ticket description (skips interactive prompt)")
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	var desc string
	if newFrom != "" {
		desc = strings.TrimSpace(newFrom)
	} else {
		desc, err = newWizard()
		if err != nil {
			return fmt.Errorf("lore new: %w", err)
		}
	}
	if desc == "" {
		return fmt.Errorf("lore new: description is required")
	}

	id, err := ticket.NewID()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	t := &ticket.Ticket{
		ID:        id,
		Desc:      desc,
		Status:    ticket.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := saveTicket(ctx, gitRoot, t); err != nil {
		return fmt.Errorf("lore new: %w", err)
	}

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketCreated, map[string]any{
		"id": t.ID,
	}))

	fmt.Printf("created ticket %s\n", t.ID)
	return nil
}

func newWizard() (string, error) {
	r := bufio.NewReader(os.Stdin)
	fmt.Print("Description: ")
	desc, err := r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read description: %w", err)
	}
	return strings.TrimSpace(desc), nil
}
