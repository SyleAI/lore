package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/loreteam/lore/internal/ticketops"
	"github.com/spf13/cobra"
)

// requireGitRoot returns the git root from context or an error if not in a repo.
func requireGitRoot(cmd *cobra.Command) (string, error) {
	v := gitRootFromContext(cmd.Context())
	if v == "" {
		return "", fmt.Errorf("lore %s: not inside a git repository", cmd.Name())
	}
	return v, nil
}

// loadTicket reads a ticket from git refs by ID, checking open/ then done/.
func loadTicket(ctx context.Context, gitRoot, id string) (*ticket.Ticket, error) {
	return ticketops.LoadTicket(ctx, gitRoot, id)
}

// casUpdate atomically reads, mutates via fn, and writes back a ticket using CAS.
func casUpdate(ctx context.Context, gitRoot, id string, fn func(*ticket.Ticket) error) (*ticket.Ticket, error) {
	return ticketops.CASUpdate(ctx, gitRoot, id, fn)
}

// appendThread writes a new thread entry blob and CAS-updates the ticket to include it.
func appendThread(ctx context.Context, gitRoot string, t *ticket.Ticket, entry *ticket.ThreadEntry) (*ticket.Ticket, error) {
	return ticketops.AppendThread(ctx, gitRoot, t, entry)
}

// listTickets returns all tickets under refs/tickets/open/ and optionally refs/tickets/done/.
func listTickets(ctx context.Context, gitRoot string, includeDone bool) ([]*ticket.Ticket, error) {
	return ticketops.ListTickets(ctx, gitRoot, includeDone)
}

// loadThread reads all thread entries for a ticket in order.
func loadThread(ctx context.Context, gitRoot string, t *ticket.Ticket) ([]*ticket.ThreadEntry, error) {
	return ticketops.LoadThread(ctx, gitRoot, t)
}

// saveTicket marshals t, writes it as a git blob, and sets the open ref (no CAS).
func saveTicket(ctx context.Context, gitRoot string, t *ticket.Ticket) error {
	return ticketops.SaveTicket(ctx, gitRoot, t)
}

// agentID resolves the agent identifier from flag override, config, or hostname fallback.
func agentID(ctx context.Context, flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if cfg := configFromContext(ctx); cfg.AgentID != "" {
		return cfg.AgentID
	}
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// truncate shortens s to max runes, appending "..." if truncated.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}

// printTicket prints ticket details and thread to stdout.
func printTicket(t *ticket.Ticket, entries []*ticket.ThreadEntry) {
	fmt.Printf("ID:      %s\n", t.ID)
	fmt.Printf("Status:  %s\n", t.Status)
	if t.Agent != "" {
		fmt.Printf("Agent:   %s\n", t.Agent)
	}
	if t.Parent != "" {
		fmt.Printf("Parent:  %s\n", t.Parent)
	}
	if t.BlockReason != "" {
		fmt.Printf("Blocked: %s\n", t.BlockReason)
	}
	fmt.Printf("Created: %s\n", t.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", t.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\n%s\n", t.Desc)

	if len(entries) > 0 {
		fmt.Println("\n--- Thread ---")
		for _, e := range entries {
			ts := e.Timestamp.Format("2006-01-02 15:04:05")
			switch e.Kind {
			case ticket.EntryKindImage:
				caption := e.Caption
				if caption == "" {
					caption = e.ImageMIME
				}
				fmt.Printf("[%s] %s [image: %s]\n", ts, e.Author, caption)
			case ticket.EntryKindQuestion:
				fmt.Printf("[%s] %s (question) [qid:%s]: %s\n", ts, e.Author, e.QuestionID, strings.TrimSpace(e.Text))
			case ticket.EntryKindAnswer:
				fmt.Printf("[%s] %s (answer) [qid:%s]: %s\n", ts, e.Author, e.QuestionID, strings.TrimSpace(e.Text))
			default:
				fmt.Printf("[%s] %s (%s): %s\n", ts, e.Author, e.Kind, strings.TrimSpace(e.Text))
			}
		}
	}
}
