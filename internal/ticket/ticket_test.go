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

func TestRef(t *testing.T) {
	assert.Equal(t, "refs/tickets/t/abc123", ticket.Ref("abc123"))
}

func TestValidStatus(t *testing.T) {
	valid := []ticket.Status{
		ticket.StatusOpen,
		ticket.StatusInProgress,
		ticket.StatusBlocked,
		ticket.StatusReady,
		ticket.StatusClosed,
	}
	for _, s := range valid {
		assert.True(t, ticket.ValidStatus(s), "expected %q to be valid", s)
	}
	assert.False(t, ticket.ValidStatus("unknown"))
	assert.False(t, ticket.ValidStatus(""))
}

func TestMarshalUnmarshal(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	orig := &ticket.Ticket{
		ID:          "aabbccddee11",
		Title:       "Fix the thing",
		Description: "It is broken",
		Status:      ticket.StatusOpen,
		Priority:    4,
		Files:       []string{"foo/bar.go", "baz/qux.go"},
		Agent:       "agent-1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	data, err := ticket.Marshal(orig)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	got, err := ticket.Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, orig.ID, got.ID)
	assert.Equal(t, orig.Title, got.Title)
	assert.Equal(t, orig.Description, got.Description)
	assert.Equal(t, orig.Status, got.Status)
	assert.Equal(t, orig.Priority, got.Priority)
	assert.Equal(t, orig.Files, got.Files)
	assert.Equal(t, orig.Agent, got.Agent)
	assert.True(t, orig.CreatedAt.Equal(got.CreatedAt))
	assert.True(t, orig.UpdatedAt.Equal(got.UpdatedAt))
}

func TestUnmarshalInvalidYAML(t *testing.T) {
	// yaml.v3 is lenient about scalars, but tab-indented content is a YAML error.
	_, err := ticket.Unmarshal([]byte("title: ok\n\tbroken_indent: yes"))
	assert.Error(t, err)
}

func TestUnmarshalPartial(t *testing.T) {
	// Missing fields should unmarshal without error (permissive).
	data := []byte("title: only title\npriority: 2\n")
	tkt, err := ticket.Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, "only title", tkt.Title)
	assert.Equal(t, 2, tkt.Priority)
	assert.Equal(t, ticket.Status(""), tkt.Status)
}
