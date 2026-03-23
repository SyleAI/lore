package policy_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/loreteam/lore/internal/policy"
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

func TestDefault(t *testing.T) {
	p := policy.Default()
	assert.Equal(t, "single-prompt", p.Consolidation.Strategy)
	assert.Equal(t, 0.80, p.Consolidation.SpatialThreshold)
	assert.Equal(t, float64(40), p.Scoring.FileHeatWeight)
	assert.Equal(t, float64(15), p.Scoring.UnblocksWeight)
	assert.Equal(t, float64(5), p.Scoring.AttemptsPenalty)
	assert.Equal(t, float64(20), p.Scoring.MaxAttemptsPenalty)
}

func TestLoadFallsBackToDefault(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)

	// No refs/tickets/policy written — should return defaults.
	p, err := policy.Load(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, policy.Default().Consolidation.Strategy, p.Consolidation.Strategy)
}

func TestSaveAndLoad(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)

	p := policy.Default()
	p.Agents.MaxConcurrent = 99
	p.Consolidation.Strategy = "pairwise"
	p.Scoring.FileHeatWeight = 55

	require.NoError(t, policy.Save(ctx, dir, p))

	got, err := policy.Load(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, 99, got.Agents.MaxConcurrent)
	assert.Equal(t, "pairwise", got.Consolidation.Strategy)
	assert.Equal(t, float64(55), got.Scoring.FileHeatWeight)
}

func TestSavePreservesUnsetFields(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)

	p := policy.Default()
	require.NoError(t, policy.Save(ctx, dir, p))

	got, err := policy.Load(ctx, dir)
	require.NoError(t, err)

	// All default fields survive a round-trip.
	assert.Equal(t, p.Risk.Weights.DiffSize, got.Risk.Weights.DiffSize)
	assert.Equal(t, p.Escalation.DefaultPath, got.Escalation.DefaultPath)
	assert.Equal(t, p.Questions.HumanNotifyVia, got.Questions.HumanNotifyVia)
}
