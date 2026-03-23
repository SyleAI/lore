package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/store"
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

// openStore opens the SQLite index for the given git root.
func openStore(gitRoot string) (*store.Store, error) {
	s, err := store.Open(filepath.Join(gitRoot, ".lore", "graph.db"))
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return s, nil
}

// loadTicket reads and parses a ticket from git refs by ID.
func loadTicket(ctx context.Context, gitRoot, id string) (*ticket.Ticket, error) {
	sha, err := gitcmd.ReadRef(ctx, gitRoot, ticket.Ref(id))
	if err != nil {
		return nil, fmt.Errorf("ticket %s not found: %w", id, err)
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

// errCASExhausted is returned by casUpdate when all retries are consumed by CAS conflicts.
var errCASExhausted = errors.New("too many CAS conflicts")

// casUpdate atomically reads, mutates via fn, and writes back a ticket using CAS.
// fn returning an error is non-retryable (precondition failure).
// CAS conflicts are retried up to 3 times; if all retries fail, errCASExhausted is returned.
func casUpdate(ctx context.Context, gitRoot, id string, fn func(*ticket.Ticket) error) (*ticket.Ticket, error) {
	const maxRetries = 3
	ref := ticket.Ref(id)
	for i := 0; i < maxRetries; i++ {
		oldSHA, err := gitcmd.ReadRef(ctx, gitRoot, ref)
		if err != nil {
			return nil, fmt.Errorf("ticket %s not found: %w", id, err)
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
			return nil, err // non-retryable: precondition failed
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
		if err := gitcmd.CAS(ctx, gitRoot, ref, newSHA, oldSHA); err != nil {
			if errors.Is(err, gitcmd.ErrCASConflict) {
				continue // retry
			}
			return nil, err
		}
		if s, serr := openStore(gitRoot); serr == nil { // best-effort
			_ = s.UpsertTicket(t, newSHA)
			s.Close()
		}
		return t, nil
	}
	return nil, fmt.Errorf("ticket %s: %w", id, errCASExhausted)
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

// printTicket prints ticket details to stdout.
func printTicket(t *ticket.Ticket) {
	fmt.Printf("ID:       %s\n", t.ID)
	fmt.Printf("Title:    %s\n", t.Title)
	fmt.Printf("Status:   %s\n", t.Status)
	fmt.Printf("Priority: %d\n", t.Priority)
	if t.Agent != "" {
		fmt.Printf("Agent:    %s\n", t.Agent)
	}
	if t.Parent != "" {
		fmt.Printf("Parent:   %s\n", t.Parent)
	}
	if t.BlockReason != "" {
		fmt.Printf("Blocked:  %s\n", t.BlockReason)
	}
	if len(t.Files) > 0 {
		fmt.Printf("Files:    %s\n", strings.Join(t.Files, ", "))
	}
	if t.Attempts > 0 {
		fmt.Printf("Attempts: %d\n", t.Attempts)
	}
	if len(t.Metadata) > 0 {
		fmt.Printf("Metadata: %s\n", metadataString(t.Metadata))
	}
	fmt.Printf("Created:  %s\n", t.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:  %s\n", t.UpdatedAt.Format("2006-01-02 15:04:05"))
	if t.Description != "" {
		fmt.Printf("\n%s\n", t.Description)
	}
	if len(t.Checkpoints) > 0 {
		fmt.Printf("\nCheckpoints:\n")
		for _, cp := range t.Checkpoints {
			fmt.Printf("  [%s] %s\n", cp.Timestamp.Format("2006-01-02 15:04:05"), cp.Message)
		}
	}
}

// newCheckpoint returns a Checkpoint stamped at the current UTC time.
func newCheckpoint(msg string) ticket.Checkpoint {
	return ticket.Checkpoint{Timestamp: time.Now().UTC(), Message: msg}
}

// ageDays returns the number of whole days elapsed since t.
func ageDays(t time.Time) int {
	return int(time.Since(t).Hours() / 24)
}

// truncate shortens s to max runes, appending "..." if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ticketQueryText returns the canonical semantic query string for a ticket.
func ticketQueryText(t *ticket.Ticket) string {
	return t.Title + "\n" + t.Description
}

// metadataString formats a metadata map as sorted key=value pairs.
func metadataString(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = fmt.Sprintf("%s=%v", k, m[k])
	}
	return strings.Join(pairs, " ")
}

// saveTicket marshals t, writes it as a git blob, updates the ref, and upserts the store index.
func saveTicket(ctx context.Context, gitRoot string, t *ticket.Ticket) error {
	data, err := ticket.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal ticket: %w", err)
	}
	sha, err := gitcmd.WriteBlob(ctx, gitRoot, data)
	if err != nil {
		return fmt.Errorf("write blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, ticket.Ref(t.ID), sha); err != nil {
		return fmt.Errorf("write ref: %w", err)
	}
	s, err := openStore(gitRoot)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.UpsertTicket(t, sha); err != nil {
		return fmt.Errorf("index ticket: %w", err)
	}
	return nil
}
