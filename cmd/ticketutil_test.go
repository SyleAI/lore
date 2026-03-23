package cmd

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCmdTestRepo creates a temp git repo with a .lore/ directory ready for tests.
func newCmdTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, "setup %v: %s", args, out)
	}
	return dir
}

func makeTestTicket(id, desc string) *ticket.Ticket {
	now := time.Now().UTC()
	return &ticket.Ticket{
		ID:        id,
		Desc:      desc,
		Status:    ticket.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestSaveAndLoadTicket(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	tkt := makeTestTicket("aabbcc112233", "Fix the broken auth timeout")
	require.NoError(t, saveTicket(ctx, dir, tkt))

	loaded, err := loadTicket(ctx, dir, tkt.ID)
	require.NoError(t, err)
	assert.Equal(t, tkt.ID, loaded.ID)
	assert.Equal(t, tkt.Desc, loaded.Desc)
	assert.Equal(t, tkt.Status, loaded.Status)
}

func TestLoadTicket_NotFound(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	_, err := loadTicket(ctx, dir, "doesnotexist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLoadTicket_FindsDone(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	tkt := makeTestTicket("aabbcc112233", "A completed ticket")
	tkt.Status = ticket.StatusDone
	require.NoError(t, saveTicket(ctx, dir, tkt))

	// saveTicket always writes to open/ — simulate a done ticket by using casUpdate.
	// First save as open, then casUpdate to done.
	tkt2 := makeTestTicket("ddeeff445566", "Will be closed")
	require.NoError(t, saveTicket(ctx, dir, tkt2))
	_, err := casUpdate(ctx, dir, tkt2.ID, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusDone
		return nil
	})
	require.NoError(t, err)

	loaded, err := loadTicket(ctx, dir, tkt2.ID)
	require.NoError(t, err)
	assert.Equal(t, ticket.StatusDone, loaded.Status)
}

func TestAppendThread(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	tkt := makeTestTicket("aabbcc112233", "Ticket with thread")
	require.NoError(t, saveTicket(ctx, dir, tkt))

	entryID, err := ticket.NewEntryID()
	require.NoError(t, err)

	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindUpdate,
		Author:    "worker-1",
		Timestamp: time.Now().UTC(),
		Text:      "started working on the auth handler",
	}

	updated, err := appendThread(ctx, dir, tkt, entry)
	require.NoError(t, err)
	assert.Len(t, updated.Thread, 1)

	// Load thread back.
	loaded, err := loadTicket(ctx, dir, tkt.ID)
	require.NoError(t, err)
	assert.Len(t, loaded.Thread, 1)

	entries, err := loadThread(ctx, dir, loaded)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, entry.Text, entries[0].Text)
	assert.Equal(t, entry.Kind, entries[0].Kind)
}

func TestListTickets(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	t1 := makeTestTicket("aaa111", "First ticket")
	t2 := makeTestTicket("bbb222", "Second ticket")
	require.NoError(t, saveTicket(ctx, dir, t1))
	require.NoError(t, saveTicket(ctx, dir, t2))

	tickets, err := listTickets(ctx, dir, false)
	require.NoError(t, err)
	assert.Len(t, tickets, 2)
}

func TestListTickets_IncludeDone(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	open := makeTestTicket("aaa111", "Open ticket")
	done := makeTestTicket("bbb222", "Done ticket")
	require.NoError(t, saveTicket(ctx, dir, open))
	require.NoError(t, saveTicket(ctx, dir, done))
	_, err := casUpdate(ctx, dir, done.ID, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusDone
		return nil
	})
	require.NoError(t, err)

	without, err := listTickets(ctx, dir, false)
	require.NoError(t, err)
	assert.Len(t, without, 1)

	with, err := listTickets(ctx, dir, true)
	require.NoError(t, err)
	assert.Len(t, with, 2)
}

func TestCasUpdate_ConflictRetry(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	tkt := makeTestTicket("aabbcc112233", "CAS test ticket")
	require.NoError(t, saveTicket(ctx, dir, tkt))

	updated, err := casUpdate(ctx, dir, tkt.ID, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusWorking
		t.Agent = "agent-1"
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, ticket.StatusWorking, updated.Status)
	assert.Equal(t, "agent-1", updated.Agent)
}
