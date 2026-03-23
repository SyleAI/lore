package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loreteam/lore/internal/store"
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
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".lore"), 0755))
	return dir
}

func makeTestTicket(id, title string) *ticket.Ticket {
	now := time.Now().UTC()
	return &ticket.Ticket{
		ID:        id,
		Title:     title,
		Status:    ticket.StatusOpen,
		Priority:  3,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestSaveAndLoadTicket(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	tkt := makeTestTicket("aabbcc112233", "Test ticket")
	require.NoError(t, saveTicket(ctx, dir, tkt))

	loaded, err := loadTicket(ctx, dir, tkt.ID)
	require.NoError(t, err)
	assert.Equal(t, tkt.ID, loaded.ID)
	assert.Equal(t, tkt.Title, loaded.Title)
	assert.Equal(t, tkt.Status, loaded.Status)
	assert.Equal(t, tkt.Priority, loaded.Priority)
}

func TestSaveTicket_AllFields(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{
		ID:          "aabbcc112233",
		Title:       "Full ticket",
		Description: "Has all fields",
		Status:      ticket.StatusInProgress,
		Priority:    5,
		Files:       []string{"foo/bar.go", "baz/qux.go"},
		Agent:       "agent-007",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, saveTicket(ctx, dir, tkt))

	loaded, err := loadTicket(ctx, dir, tkt.ID)
	require.NoError(t, err)
	assert.Equal(t, tkt.Title, loaded.Title)
	assert.Equal(t, tkt.Description, loaded.Description)
	assert.Equal(t, tkt.Status, loaded.Status)
	assert.Equal(t, tkt.Priority, loaded.Priority)
	assert.Equal(t, tkt.Files, loaded.Files)
	assert.Equal(t, tkt.Agent, loaded.Agent)
	assert.True(t, tkt.CreatedAt.Equal(loaded.CreatedAt))
}

func TestLoadTicket_NotFound(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	_, err := loadTicket(ctx, dir, "doesnotexist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSaveTicket_UpdatesStore(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	tkt := makeTestTicket("aabbcc112233", "Original title")
	require.NoError(t, saveTicket(ctx, dir, tkt))

	s, err := openStore(dir)
	require.NoError(t, err)
	defer s.Close()

	rows, err := s.ListTickets(store.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Original title", rows[0].Title)
}

func TestSaveTicket_Update(t *testing.T) {
	dir := newCmdTestRepo(t)
	ctx := context.Background()

	tkt := makeTestTicket("aabbcc112233", "Original title")
	require.NoError(t, saveTicket(ctx, dir, tkt))

	tkt.Title = "Updated title"
	tkt.Status = ticket.StatusClosed
	tkt.UpdatedAt = time.Now().UTC()
	require.NoError(t, saveTicket(ctx, dir, tkt))

	loaded, err := loadTicket(ctx, dir, tkt.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated title", loaded.Title)
	assert.Equal(t, ticket.StatusClosed, loaded.Status)

	// Store should also reflect the update.
	s, err := openStore(dir)
	require.NoError(t, err)
	defer s.Close()
	rows, err := s.ListTickets(store.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Updated title", rows[0].Title)
	assert.Equal(t, "closed", rows[0].Status)
}

func TestOpenStore_MissingLoreDir(t *testing.T) {
	dir := newCmdTestRepo(t)
	// Remove .lore/ to simulate uninitialised repo.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".lore")))
	_, err := openStore(dir)
	assert.Error(t, err)
}
