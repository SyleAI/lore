package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeOrchestrator creates a minimal valid orchestrator directory in dir and
// returns its path. If withManifest is true it includes a lore.yaml.
func makeOrchestrator(t *testing.T, name string, withManifest bool) string {
	t.Helper()
	dir := t.TempDir()
	orchDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(orchDir, 0755))

	// Minimal entrypoint — prints a line and exits 0.
	script := "#!/bin/sh\necho 'orchestrator ran'\nexit 0\n"
	scriptPath := filepath.Join(orchDir, "orchestrator.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0755))

	if withManifest {
		manifest := "name: " + name + "\nversion: 0.1.0\ndescription: test orchestrator\nruntime: bash\nentrypoint: orchestrator.sh\n"
		require.NoError(t, os.WriteFile(filepath.Join(orchDir, "lore.yaml"), []byte(manifest), 0644))
	}

	return orchDir
}

// ── lore install ─────────────────────────────────────────────────────────────

func TestInstall_LocalPath(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "my-orch", true)

	out := run(t, repo, 0, "install", src)
	assert.Contains(t, out, "installed")
	assert.Contains(t, out, "my-orch")

	// Directory should exist under .lore/orchestrators/.
	orchDir := filepath.Join(repo, ".lore", "orchestrators", "my-orch")
	_, err := os.Stat(orchDir)
	assert.NoError(t, err)
}

func TestInstall_NoManifest_FallsBackToBasename(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "bare-orch", false)

	out := run(t, repo, 0, "install", src)
	assert.Contains(t, out, "bare-orch")

	orchDir := filepath.Join(repo, ".lore", "orchestrators", "bare-orch")
	_, err := os.Stat(orchDir)
	assert.NoError(t, err)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "dup-orch", true)

	run(t, repo, 0, "install", src)
	out := run(t, repo, 1, "install", src)
	assert.Contains(t, out, "already installed")
}

func TestInstall_BadSource(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 1, "install", "not-a-valid-source")
	assert.Contains(t, out, "unrecognized source")
}

func TestInstall_LocalPathNotExist(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 1, "install", "./does-not-exist")
	assert.NotEmpty(t, out)
}

// ── lore orchestrators ───────────────────────────────────────────────────────

func TestOrchestrators_Empty(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 0, "orchestrators")
	assert.Contains(t, out, "No orchestrators installed")
}

func TestOrchestrators_ListsInstalled(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "list-orch", true)
	run(t, repo, 0, "install", src)

	out := run(t, repo, 0, "orchestrators")
	assert.Contains(t, out, "list-orch")
	assert.Contains(t, out, "0.1.0")
	assert.Contains(t, out, "test orchestrator")
}

func TestOrchestrators_NoManifest(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "no-manifest", false)
	run(t, repo, 0, "install", src)

	out := run(t, repo, 0, "orchestrators")
	assert.Contains(t, out, "no manifest")
}

// ── lore orchestrators remove ────────────────────────────────────────────────

func TestOrchestratorRemove(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "remove-me", true)
	run(t, repo, 0, "install", src)

	out := run(t, repo, 0, "orchestrators", "remove", "remove-me")
	assert.Contains(t, out, "removed")

	// Should be gone.
	orchDir := filepath.Join(repo, ".lore", "orchestrators", "remove-me")
	_, err := os.Stat(orchDir)
	assert.True(t, os.IsNotExist(err))

	list := run(t, repo, 0, "orchestrators")
	assert.Contains(t, list, "No orchestrators installed")
}

func TestOrchestratorRemove_NotInstalled(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 1, "orchestrators", "remove", "ghost")
	assert.Contains(t, out, "is not installed")
}

func TestOrchestratorRemove_PathTraversal(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 1, "orchestrators", "remove", "../../etc")
	assert.Contains(t, out, "invalid name")
}

// ── lore run ─────────────────────────────────────────────────────────────────

func TestRun(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "runme", true)

	// Write a GOAL.md so the run command doesn't warn.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "GOAL.md"), []byte("test goal"), 0644))

	run(t, repo, 0, "install", src)
	out := run(t, repo, 0, "run", "runme")
	assert.Contains(t, out, "orchestrator ran")
}

func TestRun_EnvVarsInjected(t *testing.T) {
	repo := newRepo(t)

	// Orchestrator that prints env vars.
	orchDir := t.TempDir()
	script := "#!/bin/sh\necho \"ROOT=$LORE_REPO_ROOT\"\necho \"DIR=$LORE_ORCHESTRATOR_DIR\"\necho \"NAME=$LORE_ORCHESTRATOR_NAME\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "orchestrator.sh"), []byte(script), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "lore.yaml"),
		[]byte("name: env-test\nruntime: bash\nentrypoint: orchestrator.sh\n"), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "GOAL.md"), []byte("goal"), 0644))
	run(t, repo, 0, "install", orchDir)

	out := run(t, repo, 0, "run", "env-test")
	assert.Contains(t, out, "ROOT="+repo)
	assert.True(t, strings.Contains(out, "DIR=") && strings.Contains(out, "orchestrators/env-test"))
	assert.Contains(t, out, "NAME=env-test")
}

func TestRun_NotInstalled(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 1, "run", "ghost")
	assert.Contains(t, out, "not found")
}

func TestRun_PathTraversal(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 1, "run", "../../etc/passwd")
	assert.Contains(t, out, "invalid orchestrator name")
}

func TestRun_MissingGoalWarns(t *testing.T) {
	repo := newRepo(t)
	src := makeOrchestrator(t, "warn-orch", true)
	run(t, repo, 0, "install", src)

	// No GOAL.md — should still run but print a warning.
	out := run(t, repo, 0, "run", "warn-orch")
	assert.Contains(t, out, "warning")
	assert.Contains(t, out, "GOAL.md")
}

func TestRun_ExitCodePropagated(t *testing.T) {
	repo := newRepo(t)
	orchDir := t.TempDir()
	script := "#!/bin/sh\nexit 42\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "orchestrator.sh"), []byte(script), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "lore.yaml"),
		[]byte("name: fail-orch\nruntime: bash\nentrypoint: orchestrator.sh\n"), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "GOAL.md"), []byte("goal"), 0644))
	run(t, repo, 0, "install", orchDir)

	// run() asserts exit code — 42 != 0, so pass wantCode=42.
	run(t, repo, 42, "run", "fail-orch")
}
