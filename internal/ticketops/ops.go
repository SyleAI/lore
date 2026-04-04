// Package ticketops provides core ticket CRUD operations over .tickets/ file storage.
// Both the CLI (cmd/) and web UI (internal/ui/) depend on this package.
package ticketops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/loreteam/lore/internal/ticket"
)

// TicketsDir returns the absolute path to the .tickets/ directory.
func TicketsDir(gitRoot string) string {
	return filepath.Join(gitRoot, ".tickets")
}

func openPath(gitRoot, id string) string {
	return filepath.Join(gitRoot, ".tickets", "open", id+".yaml")
}

func donePath(gitRoot, id string) string {
	return filepath.Join(gitRoot, ".tickets", "done", id+".yaml")
}

func lockPath(gitRoot, id string) string {
	return filepath.Join(gitRoot, ".tickets", ".locks", id)
}

func threadDir(gitRoot, id string) string {
	return filepath.Join(gitRoot, ".tickets", "threads", id)
}

func questionPath(gitRoot, qid string) string {
	return filepath.Join(gitRoot, ".tickets", "questions", qid+".json")
}

func blobPath(gitRoot, sha string) string {
	return filepath.Join(gitRoot, ".tickets", "blobs", sha)
}

// ticketFilePath returns the path to a ticket file (open or done).
func ticketFilePath(gitRoot, id string) (string, error) {
	op := openPath(gitRoot, id)
	if _, err := os.Stat(op); err == nil {
		return op, nil
	}
	dp := donePath(gitRoot, id)
	if _, err := os.Stat(dp); err == nil {
		return dp, nil
	}
	return "", fmt.Errorf("ticket %s not found", id)
}

// atomicWrite writes data to path atomically using a temp file + rename.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// withLock acquires an exclusive flock on the ticket's lock file and runs fn.
func withLock(gitRoot, id string, fn func() error) error {
	lp := lockPath(gitRoot, id)
	if err := os.MkdirAll(filepath.Dir(lp), 0755); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}
	f, err := os.OpenFile(lp, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint
	return fn()
}

// LoadTicket reads a ticket from .tickets/ by ID, checking open/ then done/.
func LoadTicket(_ context.Context, gitRoot, id string) (*ticket.Ticket, error) {
	path, err := ticketFilePath(gitRoot, id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ticket %s: %w", id, err)
	}
	t, err := ticket.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parse ticket %s: %w", id, err)
	}
	return t, nil
}

// SaveTicket marshals t and writes it to .tickets/open/<id>.yaml (no lock; use for new tickets only).
func SaveTicket(_ context.Context, gitRoot string, t *ticket.Ticket) error {
	data, err := ticket.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal ticket: %w", err)
	}
	return atomicWrite(openPath(gitRoot, t.ID), data, 0644)
}

// CASUpdate locks the ticket, reads it, applies fn, and writes back — handling open↔done transitions.
func CASUpdate(_ context.Context, gitRoot, id string, fn func(*ticket.Ticket) error) (*ticket.Ticket, error) {
	var result *ticket.Ticket
	err := withLock(gitRoot, id, func() error {
		oldPath, err := ticketFilePath(gitRoot, id)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(oldPath)
		if err != nil {
			return fmt.Errorf("read ticket %s: %w", id, err)
		}
		t, err := ticket.Unmarshal(data)
		if err != nil {
			return fmt.Errorf("parse ticket %s: %w", id, err)
		}
		if err := fn(t); err != nil {
			return err
		}
		t.UpdatedAt = time.Now().UTC()
		newData, err := ticket.Marshal(t)
		if err != nil {
			return fmt.Errorf("marshal ticket: %w", err)
		}

		var newPath string
		if t.Status == ticket.StatusDone {
			newPath = donePath(gitRoot, id)
		} else {
			newPath = openPath(gitRoot, id)
		}

		if err := atomicWrite(newPath, newData, 0644); err != nil {
			return fmt.Errorf("write ticket: %w", err)
		}
		// Remove old file if the ticket moved between open/ and done/.
		if newPath != oldPath {
			if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove old ticket file: %w", err)
			}
		}
		result = t
		return nil
	})
	return result, err
}

// AppendThread writes a new thread entry file and touches the ticket's UpdatedAt.
func AppendThread(_ context.Context, gitRoot string, t *ticket.Ticket, entry *ticket.ThreadEntry) (*ticket.Ticket, error) {
	data, err := ticket.MarshalEntry(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal entry: %w", err)
	}
	dir := threadDir(gitRoot, t.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create thread dir: %w", err)
	}
	// Filename encodes timestamp for chronological ordering.
	filename := fmt.Sprintf("%020d-%s.json", time.Now().UnixNano(), entry.ID)
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0644); err != nil {
		return nil, fmt.Errorf("write entry: %w", err)
	}
	// Touch ticket UpdatedAt.
	return CASUpdate(context.Background(), gitRoot, t.ID, func(*ticket.Ticket) error { return nil })
}

// LoadThread reads all thread entries for a ticket in chronological order.
func LoadThread(_ context.Context, gitRoot string, t *ticket.Ticket) ([]*ticket.ThreadEntry, error) {
	dir := threadDir(gitRoot, t.ID)
	dirEntries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read thread dir: %w", err)
	}
	// ReadDir returns entries sorted by name, which is chronological due to the timestamp prefix.
	var result []*ticket.ThreadEntry
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, de.Name()))
		if err != nil {
			return nil, fmt.Errorf("read entry %s: %w", de.Name(), err)
		}
		te, err := ticket.UnmarshalEntry(data)
		if err != nil {
			return nil, fmt.Errorf("parse entry %s: %w", de.Name(), err)
		}
		result = append(result, te)
	}
	return result, nil
}

// ListTickets returns all tickets under .tickets/open/ and optionally .tickets/done/.
func ListTickets(_ context.Context, gitRoot string, includeDone bool) ([]*ticket.Ticket, error) {
	var paths []string

	openDir := filepath.Join(gitRoot, ".tickets", "open")
	if oe, err := os.ReadDir(openDir); err == nil {
		for _, e := range oe {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				paths = append(paths, filepath.Join(openDir, e.Name()))
			}
		}
	}

	if includeDone {
		doneDir := filepath.Join(gitRoot, ".tickets", "done")
		if de, err := os.ReadDir(doneDir); err == nil {
			for _, e := range de {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
					paths = append(paths, filepath.Join(doneDir, e.Name()))
				}
			}
		}
	}

	tickets := make([]*ticket.Ticket, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
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

// WriteBlob stores data as a content-addressed blob under .tickets/blobs/<sha256>.
// Returns the hex SHA256 of the data.
func WriteBlob(gitRoot string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	p := blobPath(gitRoot, sha)
	if _, err := os.Stat(p); err == nil {
		return sha, nil // already exists (content-addressed, so identical)
	}
	if err := atomicWrite(p, data, 0644); err != nil {
		return "", fmt.Errorf("write blob: %w", err)
	}
	return sha, nil
}

// LoadBlob reads a content-addressed blob from .tickets/blobs/<sha>.
func LoadBlob(gitRoot, sha string) ([]byte, error) {
	data, err := os.ReadFile(blobPath(gitRoot, sha))
	if err != nil {
		return nil, fmt.Errorf("blob %s not found: %w", sha, err)
	}
	return data, nil
}

// LoadQuestion reads a question record from .tickets/questions/<qid>.json.
func LoadQuestion(gitRoot, qid string) (map[string]any, error) {
	data, err := os.ReadFile(questionPath(gitRoot, qid))
	if err != nil {
		return nil, fmt.Errorf("question %s not found: %w", qid, err)
	}
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parse question %s: %w", qid, err)
	}
	return rec, nil
}

// SaveQuestion writes a question record to .tickets/questions/<qid>.json.
// The record must contain a "qid" key with the question ID.
func SaveQuestion(gitRoot string, rec map[string]any) error {
	qid, _ := rec["qid"].(string)
	if qid == "" {
		return fmt.Errorf("question record missing qid")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal question: %w", err)
	}
	return atomicWrite(questionPath(gitRoot, qid), data, 0644)
}
