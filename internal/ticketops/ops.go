// Package ticketops provides core ticket CRUD operations over git object storage.
// Both the CLI (cmd/) and web UI (internal/ui/) depend on this package.
package ticketops

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/ticket"
)

// ErrCASExhausted is returned when all CAS retries are consumed by conflicts.
var ErrCASExhausted = errors.New("too many CAS conflicts")

// LoadTicket reads a ticket from git refs by ID, checking open/ then done/.
func LoadTicket(ctx context.Context, gitRoot, id string) (*ticket.Ticket, error) {
	sha, _, err := ReadTicketRef(ctx, gitRoot, id)
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

// ReadTicketRef resolves which ref (open or done) holds the ticket.
// Returns the blob SHA and the ref name.
func ReadTicketRef(ctx context.Context, gitRoot, id string) (sha, ref string, err error) {
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

// CASUpdate atomically reads, mutates via fn, and writes back a ticket using CAS.
// Handles open↔done ref transitions automatically based on the resulting status.
func CASUpdate(ctx context.Context, gitRoot, id string, fn func(*ticket.Ticket) error) (*ticket.Ticket, error) {
	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		oldSHA, oldRef, err := ReadTicketRef(ctx, gitRoot, id)
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

		var newRef string
		if t.Status == ticket.StatusDone {
			newRef = ticket.DoneRef(id)
		} else {
			newRef = ticket.OpenRef(id)
		}

		if newRef == oldRef {
			if err := gitcmd.CAS(ctx, gitRoot, newRef, newSHA, oldSHA); err != nil {
				if errors.Is(err, gitcmd.ErrCASConflict) {
					continue
				}
				return nil, err
			}
		} else {
			if err := gitcmd.WriteRef(ctx, gitRoot, newRef, newSHA); err != nil {
				return nil, fmt.Errorf("write ref: %w", err)
			}
			if err := gitcmd.DeleteRef(ctx, gitRoot, oldRef); err != nil {
				return nil, fmt.Errorf("delete old ref: %w", err)
			}
		}
		return t, nil
	}
	return nil, fmt.Errorf("ticket %s: %w", id, ErrCASExhausted)
}

// AppendThread writes a new thread entry blob and CAS-updates the ticket to include it.
func AppendThread(ctx context.Context, gitRoot string, t *ticket.Ticket, entry *ticket.ThreadEntry) (*ticket.Ticket, error) {
	entryData, err := ticket.MarshalEntry(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal entry: %w", err)
	}
	entrySHA, err := gitcmd.WriteBlob(ctx, gitRoot, entryData)
	if err != nil {
		return nil, fmt.Errorf("write entry blob: %w", err)
	}
	return CASUpdate(ctx, gitRoot, t.ID, func(t *ticket.Ticket) error {
		t.Thread = append(t.Thread, entrySHA)
		return nil
	})
}

// ListTickets returns all tickets under refs/tickets/open/ and optionally refs/tickets/done/.
func ListTickets(ctx context.Context, gitRoot string, includeDone bool) ([]*ticket.Ticket, error) {
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
			continue
		}
		t, err := ticket.Unmarshal(data)
		if err != nil {
			continue
		}
		tickets = append(tickets, t)
	}
	return tickets, nil
}

// LoadThread reads all thread entries for a ticket in order.
func LoadThread(ctx context.Context, gitRoot string, t *ticket.Ticket) ([]*ticket.ThreadEntry, error) {
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

// SaveTicket marshals t, writes it as a git blob, and sets the open ref (no CAS).
// Use for new tickets only; use CASUpdate for mutations.
func SaveTicket(ctx context.Context, gitRoot string, t *ticket.Ticket) error {
	data, err := ticket.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal ticket: %w", err)
	}
	sha, err := gitcmd.WriteBlob(ctx, gitRoot, data)
	if err != nil {
		return fmt.Errorf("write blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, ticket.OpenRef(t.ID), sha); err != nil {
		return fmt.Errorf("write ref: %w", err)
	}
	return nil
}
