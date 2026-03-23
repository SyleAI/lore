package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/ticket"
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
	sha, _, err := readTicketRef(ctx, gitRoot, id)
	if err != nil {
		return nil, err
	}
	data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
	if err != nil {
		return nil, fmt.Errorf("read ticket %s: %w", id, err)
	}
	t, err := ticket.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parse ticket %s: %w", id, err)
	}
	return t, nil
}

// readTicketRef resolves which ref (open or done) holds the ticket and returns the SHA and ref name.
func readTicketRef(ctx context.Context, gitRoot, id string) (sha, ref string, err error) {
	openRef := ticket.OpenRef(id)
	if sha, err = gitcmd.ReadRef(ctx, gitRoot, openRef); err == nil {
		return sha, openRef, nil
	}
	doneRef := ticket.DoneRef(id)
	if sha, err = gitcmd.ReadRef(ctx, gitRoot, doneRef); err == nil {
		return sha, doneRef, nil
	}
	return "", "", fmt.Errorf("ticket %s not found", id)
}

// errCASExhausted is returned by casUpdate when all retries are consumed by CAS conflicts.
var errCASExhausted = errors.New("too many CAS conflicts")

// casUpdate atomically reads, mutates via fn, and writes back a ticket using CAS.
// Handles open↔done ref transitions automatically based on the resulting status.
func casUpdate(ctx context.Context, gitRoot, id string, fn func(*ticket.Ticket) error) (*ticket.Ticket, error) {
	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		oldSHA, oldRef, err := readTicketRef(ctx, gitRoot, id)
		if err != nil {
			return nil, err
		}
		data, err := gitcmd.ReadBlob(ctx, gitRoot, oldSHA)
		if err != nil {
			return nil, fmt.Errorf("read ticket %s: %w", id, err)
		}
		t, err := ticket.Unmarshal(data)
		if err != nil {
			return nil, fmt.Errorf("parse ticket %s: %w", id, err)
		}
		if err := fn(t); err != nil {
			return nil, err
		}
		t.UpdatedAt = time.Now().UTC()
		newData, err := ticket.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("marshal ticket: %w", err)
		}
		newSHA, err := gitcmd.WriteBlob(ctx, gitRoot, newData)
		if err != nil {
			return nil, fmt.Errorf("write blob: %w", err)
		}

		// Determine the target ref based on new status.
		var newRef string
		if t.Status == ticket.StatusDone {
			newRef = ticket.DoneRef(id)
		} else {
			newRef = ticket.OpenRef(id)
		}

		if newRef == oldRef {
			// Same ref namespace — standard CAS.
			if err := gitcmd.CAS(ctx, gitRoot, newRef, newSHA, oldSHA); err != nil {
				if errors.Is(err, gitcmd.ErrCASConflict) {
					continue
				}
				return nil, err
			}
		} else {
			// Ref namespace changed (e.g. open → done). Write new ref, then delete old.
			if err := gitcmd.WriteRef(ctx, gitRoot, newRef, newSHA); err != nil {
				return nil, fmt.Errorf("write ref: %w", err)
			}
			if err := gitcmd.DeleteRef(ctx, gitRoot, oldRef); err != nil {
				return nil, fmt.Errorf("delete old ref: %w", err)
			}
		}
		return t, nil
	}
	return nil, fmt.Errorf("ticket %s: %w", id, errCASExhausted)
}

// appendThread creates a new thread entry blob, then CAS-updates the ticket to include it.
func appendThread(ctx context.Context, gitRoot string, t *ticket.Ticket, entry *ticket.ThreadEntry) (*ticket.Ticket, error) {
	entryData, err := ticket.MarshalEntry(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal entry: %w", err)
	}
	entrySHA, err := gitcmd.WriteBlob(ctx, gitRoot, entryData)
	if err != nil {
		return nil, fmt.Errorf("write entry blob: %w", err)
	}

	return casUpdate(ctx, gitRoot, t.ID, func(t *ticket.Ticket) error {
		t.Thread = append(t.Thread, entrySHA)
		return nil
	})
}

// listTickets returns all tickets under refs/tickets/open/ and optionally refs/tickets/done/.
func listTickets(ctx context.Context, gitRoot string, includeDone bool) ([]*ticket.Ticket, error) {
	refs, err := gitcmd.ListRefs(ctx, gitRoot, "refs/tickets/open/")
	if err != nil {
		return nil, fmt.Errorf("list open tickets: %w", err)
	}
	if includeDone {
		doneRefs, err := gitcmd.ListRefs(ctx, gitRoot, "refs/tickets/done/")
		if err != nil {
			return nil, fmt.Errorf("list done tickets: %w", err)
		}
		for k, v := range doneRefs {
			refs[k] = v
		}
	}

	tickets := make([]*ticket.Ticket, 0, len(refs))
	for _, sha := range refs {
		data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
		if err != nil {
			continue // skip unreadable refs
		}
		t, err := ticket.Unmarshal(data)
		if err != nil {
			continue
		}
		tickets = append(tickets, t)
	}
	return tickets, nil
}

// loadThread reads all thread entries for a ticket in order.
func loadThread(ctx context.Context, gitRoot string, t *ticket.Ticket) ([]*ticket.ThreadEntry, error) {
	entries := make([]*ticket.ThreadEntry, 0, len(t.Thread))
	for _, sha := range t.Thread {
		data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
		if err != nil {
			return nil, fmt.Errorf("read entry %s: %w", sha, err)
		}
		e, err := ticket.UnmarshalEntry(data)
		if err != nil {
			return nil, fmt.Errorf("parse entry %s: %w", sha, err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// saveTicket marshals t, writes it as a git blob, and sets the ref (no CAS).
// Use for new tickets only; use casUpdate for mutations.
func saveTicket(ctx context.Context, gitRoot string, t *ticket.Ticket) error {
	data, err := ticket.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal ticket: %w", err)
	}
	sha, err := gitcmd.WriteBlob(ctx, gitRoot, data)
	if err != nil {
		return fmt.Errorf("write blob: %w", err)
	}
	ref := ticket.OpenRef(t.ID)
	if err := gitcmd.WriteRef(ctx, gitRoot, ref, sha); err != nil {
		return fmt.Errorf("write ref: %w", err)
	}
	return nil
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
			default:
				fmt.Printf("[%s] %s (%s): %s\n", ts, e.Author, e.Kind, strings.TrimSpace(e.Text))
			}
		}
	}
}
