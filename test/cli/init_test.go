package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}

	out := run(t, dir, 0, "init")
	assert.Contains(t, out, "initialized")

	// .tickets/ subdirectories must exist.
	for _, sub := range []string{"open", "done", "threads", "questions", "blobs"} {
		_, err := os.Stat(filepath.Join(dir, ".tickets", sub))
		assert.NoError(t, err, ".tickets/%s should exist", sub)
	}

	// .lore/config.yaml and policy.yaml must exist.
	for _, f := range []string{"config.yaml", "policy.yaml"} {
		_, err := os.Stat(filepath.Join(dir, ".lore", f))
		assert.NoError(t, err, ".lore/%s should exist", f)
	}

	// .gitattributes must contain the -diff line for .tickets/.
	gaData, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err, ".gitattributes should exist")
	assert.Contains(t, string(gaData), ".tickets/** -diff")
}

func TestInit_AlreadyInitialized(t *testing.T) {
	dir := newRepo(t)
	out := run(t, dir, 1, "init")
	assert.Contains(t, out, "already initialized")
}

func TestInit_Force(t *testing.T) {
	dir := newRepo(t)
	out := run(t, dir, 0, "init", "--force")
	assert.Contains(t, out, "initialized")
}

func TestInit_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	out := run(t, dir, 1, "init")
	assert.Contains(t, out, "git")
}

func TestDoctor_NotInitialised(t *testing.T) {
	// A bare git repo with no lore init should fail doctor.
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	out := run(t, dir, 1, "doctor")
	assert.Contains(t, out, "✗")
}
