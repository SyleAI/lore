package graph_test

import (
	"bytes"
	"context"
	"math"
	"os/exec"
	"testing"
	"time"

	"github.com/loreteam/lore/internal/graph"
	"github.com/loreteam/lore/internal/policy"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test User"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

func defaultPol() *policy.Policy { return policy.Default() }

func TestEmptyIndex(t *testing.T) {
	idx := graph.Empty()
	assert.NotNil(t, idx.Tickets)
	assert.NotNil(t, idx.Files)
	assert.NotNil(t, idx.Clusters)
	assert.Equal(t, 1, idx.Version)
}

func TestLoadFallsBackToEmpty(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)

	idx, err := graph.Load(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, idx.Tickets)
}

func TestSaveAndLoad(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)

	idx := graph.Empty()
	idx.Tickets["abc"] = &graph.TicketNode{Score: 42, Files: []string{"foo.go"}}
	idx.Files["foo.go"] = &graph.FileNode{Heat: 0.75, Tickets: []string{"abc"}}

	require.NoError(t, graph.Save(ctx, dir, idx))

	got, err := graph.Load(ctx, dir)
	require.NoError(t, err)
	require.Contains(t, got.Tickets, "abc")
	assert.Equal(t, 42, got.Tickets["abc"].Score)
	assert.Equal(t, []string{"foo.go"}, got.Tickets["abc"].Files)
	assert.InDelta(t, 0.75, got.Files["foo.go"].Heat, 0.001)
}

func TestUpdateTicketScore(t *testing.T) {
	pol := defaultPol()
	idx := graph.Empty()
	idx.Files["hot.go"] = &graph.FileNode{Heat: 1.0}

	t1 := &ticket.Ticket{
		ID:        "t1",
		Title:     "test",
		Files:     []string{"hot.go"},
		CreatedAt: time.Now().Add(-10 * 24 * time.Hour), // 10 days old
		UpdatedAt: time.Now(),
		Status:    ticket.StatusOpen,
	}

	score := graph.UpdateTicket(idx, t1, pol)

	// Expected: fileHeat(1.0×40) + age(10×0.2) - attempts(0) = 42
	assert.Equal(t, 42, score)
	require.Contains(t, idx.Tickets, "t1")
	assert.Equal(t, 42, idx.Tickets["t1"].Score)
}

func TestScoreClampedToHundred(t *testing.T) {
	pol := defaultPol()
	idx := graph.Empty()
	idx.Files["f.go"] = &graph.FileNode{Heat: 1.0}

	// Ticket with many unblocked children and old age should not exceed 100.
	node := &graph.TicketNode{Blocks: make([]string, 20)}
	idx.Tickets["parent"] = node

	t1 := &ticket.Ticket{
		ID:        "parent",
		Files:     []string{"f.go"},
		CreatedAt: time.Now().Add(-365 * 24 * time.Hour),
		UpdatedAt: time.Now(),
		Status:    ticket.StatusOpen,
	}
	score := graph.UpdateTicket(idx, t1, pol)
	assert.LessOrEqual(t, score, 100)
	assert.GreaterOrEqual(t, score, 0)
}

func TestAttemptsPenalty(t *testing.T) {
	pol := defaultPol()
	idx := graph.Empty()

	// Give tickets enough age so penalty doesn't clamp the score to zero.
	// 200 days × AgeDayWeight(0.2) = 40, penalty cap is 20 → scoreB = 20.
	oldEnough := time.Now().Add(-200 * 24 * time.Hour)

	noAttempts := &ticket.Ticket{
		ID: "a", Attempts: 0,
		CreatedAt: oldEnough, UpdatedAt: time.Now(), Status: ticket.StatusOpen,
	}
	manyAttempts := &ticket.Ticket{
		ID: "b", Attempts: 10,
		CreatedAt: oldEnough, UpdatedAt: time.Now(), Status: ticket.StatusOpen,
	}

	scoreA := graph.UpdateTicket(idx, noAttempts, pol)
	scoreB := graph.UpdateTicket(idx, manyAttempts, pol)

	// More attempts → lower score (penalty capped at MaxAttemptsPenalty=20).
	assert.Greater(t, scoreA, scoreB)
	expectedPenalty := int(math.Min(10*pol.Scoring.AttemptsPenalty, pol.Scoring.MaxAttemptsPenalty))
	assert.Equal(t, scoreA-expectedPenalty, scoreB)
}

func TestBlockingRelationships(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)
	pol := defaultPol()

	parent := &ticket.Ticket{
		ID: "parent", Title: "Parent", Status: ticket.StatusOpen,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	child := &ticket.Ticket{
		ID: "child", Title: "Child", Status: ticket.StatusOpen,
		Parent:    "parent",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	writeTicket(t, dir, parent)
	writeTicket(t, dir, child)

	idx, err := graph.Build(ctx, dir, pol)
	require.NoError(t, err)

	require.Contains(t, idx.Tickets, "parent")
	require.Contains(t, idx.Tickets, "child")
	assert.Contains(t, idx.Tickets["parent"].Blocks, "child")
	assert.Contains(t, idx.Tickets["child"].BlockedBy, "parent")
}

func writeTicket(t *testing.T, gitRoot string, tk *ticket.Ticket) {
	t.Helper()

	data, err := ticket.Marshal(tk)
	require.NoError(t, err)

	cmd := exec.Command("git", "-C", gitRoot, "hash-object", "-w", "--stdin")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	require.NoError(t, err)
	sha := string(out[:len(out)-1])

	out2, err := exec.Command("git", "-C", gitRoot, "update-ref", ticket.Ref(tk.ID), sha).CombinedOutput()
	require.NoError(t, err, string(out2))
}
