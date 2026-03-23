package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loreteam/lore/internal/store"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func makeTicket(id, title string, status ticket.Status, priority int) *ticket.Ticket {
	now := time.Now().UTC()
	return &ticket.Ticket{
		ID:        id,
		Title:     title,
		Status:    status,
		Priority:  priority,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestOpenCreatesSchema(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	s.Close()

	// Re-opening should also succeed (idempotent migrations).
	s2, err := store.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	s2.Close()
}

func TestOpenMissingDirectory(t *testing.T) {
	_, err := store.Open("/nonexistent/path/test.db")
	assert.Error(t, err)
}

func TestUpsertAndList(t *testing.T) {
	s := newTestStore(t)

	t1 := makeTicket("aaa111", "First ticket", ticket.StatusOpen, 3)
	t2 := makeTicket("bbb222", "Second ticket", ticket.StatusInProgress, 5)

	require.NoError(t, s.UpsertTicket(t1, "sha1"))
	require.NoError(t, s.UpsertTicket(t2, "sha2"))

	rows, err := s.ListTickets(store.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, rows, 2)

	// Priority 5 should come first.
	assert.Equal(t, "bbb222", rows[0].ID)
	assert.Equal(t, "aaa111", rows[1].ID)
}

func TestUpsertUpdatesExisting(t *testing.T) {
	s := newTestStore(t)

	tkt := makeTicket("aaa111", "Original title", ticket.StatusOpen, 3)
	require.NoError(t, s.UpsertTicket(tkt, "sha1"))

	tkt.Title = "Updated title"
	tkt.Status = ticket.StatusClosed
	require.NoError(t, s.UpsertTicket(tkt, "sha2"))

	rows, err := s.ListTickets(store.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Updated title", rows[0].Title)
	assert.Equal(t, "closed", rows[0].Status)
	assert.Equal(t, "sha2", rows[0].SHA)
}

func TestListFilter(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.UpsertTicket(makeTicket("a1", "Open ticket", ticket.StatusOpen, 3), "sha1"))
	require.NoError(t, s.UpsertTicket(makeTicket("b1", "Closed ticket", ticket.StatusClosed, 3), "sha2"))

	open, err := s.ListTickets(store.ListFilter{Status: "open"})
	require.NoError(t, err)
	assert.Len(t, open, 1)
	assert.Equal(t, "a1", open[0].ID)

	closed, err := s.ListTickets(store.ListFilter{Status: "closed"})
	require.NoError(t, err)
	assert.Len(t, closed, 1)
	assert.Equal(t, "b1", closed[0].ID)
}

func TestUpsertWithFiles(t *testing.T) {
	s := newTestStore(t)

	tkt := makeTicket("aaa111", "Ticket with files", ticket.StatusOpen, 3)
	tkt.Files = []string{"foo/bar.go", "baz/qux.go"}
	require.NoError(t, s.UpsertTicket(tkt, "sha1"))

	// Update with different files — old files should be replaced.
	tkt.Files = []string{"new/file.go"}
	require.NoError(t, s.UpsertTicket(tkt, "sha2"))

	rows, err := s.ListTickets(store.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestDeleteTicket(t *testing.T) {
	s := newTestStore(t)

	tkt := makeTicket("aaa111", "To be deleted", ticket.StatusOpen, 3)
	require.NoError(t, s.UpsertTicket(tkt, "sha1"))

	rows, err := s.ListTickets(store.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, rows, 1)

	require.NoError(t, s.DeleteTicket("aaa111"))

	rows, err = s.ListTickets(store.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, rows, 0)
}

func TestDeleteNonExistentTicket(t *testing.T) {
	s := newTestStore(t)
	// Deleting a non-existent ticket should not error.
	assert.NoError(t, s.DeleteTicket("doesnotexist"))
}

func TestOpenInvalidPath(t *testing.T) {
	// Directory that doesn't exist.
	_, err := store.Open(filepath.Join(os.TempDir(), "no_such_dir_xyz", "test.db"))
	assert.Error(t, err)
}

func makeQuestion(id string, qt ticket.QuestionType, text string, blocking bool) ticket.Question {
	return ticket.Question{
		ID:      id,
		Type:    qt,
		Text:    text,
		Blocking: blocking,
		AskedAt: time.Now().UTC(),
	}
}

func TestUpsertWithQuestions(t *testing.T) {
	s := newTestStore(t)

	tkt := makeTicket("aaa111", "Ticket with questions", ticket.StatusInProgress, 3)
	tkt.Questions = []ticket.Question{
		makeQuestion("q-aabbcc", ticket.QuestionTypeClarification, "What does X mean?", false),
		makeQuestion("q-112233", ticket.QuestionTypeConstraintCheck, "Can I touch Y?", true),
	}
	require.NoError(t, s.UpsertTicket(tkt, "sha1"))

	rows, err := s.ListQuestions(store.QuestionFilter{})
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.Equal(t, "aaa111", rows[0].TicketID)
}

func TestListQuestions_UnansweredFilter(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	answered := makeQuestion("q-aabbcc", ticket.QuestionTypeClarification, "Answered question", false)
	answered.Answer = "yes"
	answered.AnsweredBy = "agent-x"
	answered.AnsweredAt = &now

	unanswered := makeQuestion("q-112233", ticket.QuestionTypeKnowledgeGap, "Unanswered question", false)

	tkt := makeTicket("aaa111", "Ticket", ticket.StatusInProgress, 3)
	tkt.Questions = []ticket.Question{answered, unanswered}
	require.NoError(t, s.UpsertTicket(tkt, "sha1"))

	rows, err := s.ListQuestions(store.QuestionFilter{Unanswered: true})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "q-112233", rows[0].ID)
	assert.Nil(t, rows[0].AnsweredAt)
}

func TestListQuestions_DirectedToFilter(t *testing.T) {
	s := newTestStore(t)

	q1 := makeQuestion("q-aabbcc", ticket.QuestionTypeClarification, "For lead", false)
	q1.DirectedTo = "lead-agent"
	q2 := makeQuestion("q-112233", ticket.QuestionTypeValidation, "For anyone", false)

	tkt := makeTicket("aaa111", "Ticket", ticket.StatusInProgress, 3)
	tkt.Questions = []ticket.Question{q1, q2}
	require.NoError(t, s.UpsertTicket(tkt, "sha1"))

	rows, err := s.ListQuestions(store.QuestionFilter{DirectedTo: "lead-agent"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "q-aabbcc", rows[0].ID)
}

func TestUpsertTicket_ReplacesQuestions(t *testing.T) {
	s := newTestStore(t)

	tkt := makeTicket("aaa111", "Ticket", ticket.StatusInProgress, 3)
	tkt.Questions = []ticket.Question{
		makeQuestion("q-aabbcc", ticket.QuestionTypeClarification, "First question", false),
	}
	require.NoError(t, s.UpsertTicket(tkt, "sha1"))

	// Second upsert with different questions — old ones replaced.
	tkt.Questions = []ticket.Question{
		makeQuestion("q-112233", ticket.QuestionTypeKnowledgeGap, "New question", false),
	}
	require.NoError(t, s.UpsertTicket(tkt, "sha2"))

	rows, err := s.ListQuestions(store.QuestionFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "q-112233", rows[0].ID)
}
