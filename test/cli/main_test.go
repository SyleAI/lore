// Package cli_test contains black-box integration tests for the lore CLI.
// It builds the binary once in TestMain and runs it as a subprocess in each
// test, so every test sees exactly what a real user would see.
package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// loreBin is the path to the compiled lore binary, set by TestMain.
var loreBin string

func TestMain(m *testing.M) {
	bin, err := buildLore()
	if err != nil {
		// Print to stderr and exit — tests cannot run without the binary.
		os.Stderr.WriteString("failed to build lore: " + err.Error() + "\n")
		os.Exit(1)
	}
	loreBin = bin
	defer os.RemoveAll(filepath.Dir(bin))
	os.Exit(m.Run())
}

// buildLore compiles the lore binary into a temp directory and returns its path.
func buildLore() (string, error) {
	dir, err := os.MkdirTemp("", "lore-test-bin-*")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "lore")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/loreteam/lore")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return bin, nil
}

// newRepo creates a temp directory with an initialised git repo and lore.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, "git setup: %s", out)
	}
	// Run lore init so the repo is ready for all commands.
	run(t, dir, 0, "init")
	return dir
}

// run executes the lore binary in dir with args, asserts the exit code matches
// wantCode, and returns combined stdout+stderr output.
func run(t *testing.T, dir string, wantCode int, args ...string) string {
	t.Helper()
	cmd := exec.Command(loreBin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("lore %v: unexpected error: %v", args, err)
		}
	}
	if code != wantCode {
		t.Fatalf("lore %v: exit %d, want %d\noutput: %s", args, code, wantCode, out)
	}
	return string(out)
}

// newTicket creates a ticket and returns its ID.
func newTicket(t *testing.T, dir, desc string) string {
	t.Helper()
	out := run(t, dir, 0, "new", "--from", desc)
	// Output: "created ticket <id>"
	parts := strings.Fields(strings.TrimSpace(out))
	require.GreaterOrEqual(t, len(parts), 3, "unexpected new output: %q", out)
	return parts[len(parts)-1]
}
