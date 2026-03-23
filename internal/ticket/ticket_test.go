package ticket_test

import (
	"testing"
	"time"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewID(t *testing.T) {
	id1, err := ticket.NewID()
	require.NoError(t, err)
	assert.Len(t, id1, 12)

	id2, err := ticket.NewID()
	require.NoError(t, err)
	assert.NotEqual(t, id1, id2, "IDs should be unique")
}

func TestNewQuestionID(t *testing.T) {
	qid, err := ticket.NewQuestionID()
	require.NoError(t, err)
	assert.True(t, len(qid) > 2)
	assert.Equal(t, "q-", qid[:2])
}

func TestNewEntryID(t *testing.T) {
	eid, err := ticket.NewEntryID()
	require.NoError(t, err)
	assert.Equal(t, "e-", eid[:2])
}

func TestRefs(t *testing.T) {
	assert.Equal(t, "refs/tickets/open/abc123", ticket.OpenRef("abc123"))
	assert.Equal(t, "refs/tickets/done/abc123", ticket.DoneRef("abc123"))
	assert.Equal(t, "refs/tickets/questions/q-abc", ticket.QuestionRef("q-abc"))
}

func TestValidStatus(t *testing.T) {
	valid := []ticket.Status{
		ticket.StatusOpen,
		ticket.StatusWorking,
		ticket.StatusBlocked,
		ticket.StatusReadyForReview,
		ticket.StatusDone,
	}
	for _, s := range valid {
		assert.True(t, ticket.ValidStatus(s), "expected %q to be valid", s)
	}
	assert.False(t, ticket.ValidStatus("unknown"))
	assert.False(t, ticket.ValidStatus(""))
	assert.False(t, ticket.ValidStatus("in-progress")) // old status, not in Lite
	assert.False(t, ticket.ValidStatus("closed"))      // old status, not in Lite
}

func TestMarshalUnmarshal(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	orig := &ticket.Ticket{
		ID:        "aabbccddee11",
		Desc:      "Fix the broken thing in auth",
		Status:    ticket.StatusOpen,
		Agent:     "agent-1",
		Thread:    []string{"sha1abc", "sha2def"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := ticket.Marshal(orig)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	got, err := ticket.Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, orig.ID, got.ID)
	assert.Equal(t, orig.Desc, got.Desc)
	assert.Equal(t, orig.Status, got.Status)
	assert.Equal(t, orig.Agent, got.Agent)
	assert.Equal(t, orig.Thread, got.Thread)
	assert.True(t, orig.CreatedAt.Equal(got.CreatedAt))
	assert.True(t, orig.UpdatedAt.Equal(got.UpdatedAt))
}

func TestMarshalUnmarshalThreadEntry(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	entry := &ticket.ThreadEntry{
		ID:        "e-abc123",
		Kind:      ticket.EntryKindUpdate,
		Author:    "worker-1",
		Timestamp: now,
		Text:      "fixed the null pointer in auth handler",
	}

	data, err := ticket.MarshalEntry(entry)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	got, err := ticket.UnmarshalEntry(data)
	require.NoError(t, err)

	assert.Equal(t, entry.ID, got.ID)
	assert.Equal(t, entry.Kind, got.Kind)
	assert.Equal(t, entry.Author, got.Author)
	assert.Equal(t, entry.Text, got.Text)
	assert.True(t, entry.Timestamp.Equal(got.Timestamp))
}

func TestMarshalUnmarshalImageEntry(t *testing.T) {
	entry := &ticket.ThreadEntry{
		ID:        "e-img001",
		Kind:      ticket.EntryKindImage,
		Author:    "human",
		Timestamp: time.Now().UTC(),
		ImageSHA:  "deadbeef1234",
		ImageMIME: "image/png",
		Caption:   "screenshot of error modal",
	}

	data, err := ticket.MarshalEntry(entry)
	require.NoError(t, err)

	got, err := ticket.UnmarshalEntry(data)
	require.NoError(t, err)

	assert.Equal(t, entry.ImageSHA, got.ImageSHA)
	assert.Equal(t, entry.ImageMIME, got.ImageMIME)
	assert.Equal(t, entry.Caption, got.Caption)
}

func TestUnmarshalInvalidYAML(t *testing.T) {
	_, err := ticket.Unmarshal([]byte("id: ok\n\tbroken_indent: yes"))
	assert.Error(t, err)
}

func TestUnmarshalInvalidEntryJSON(t *testing.T) {
	_, err := ticket.UnmarshalEntry([]byte("not json"))
	assert.Error(t, err)
}
