package gitcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// FindGitRoot walks up from dir until it finds a directory containing .git.
// Returns the absolute path of that directory, or an error.
func FindGitRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("gitcmd: resolve dir: %w", err)
	}

	current := abs
	for {
		info, err := os.Stat(filepath.Join(current, ".git"))
		if err == nil && info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			// reached filesystem root
			return "", fmt.Errorf("gitcmd: not a git repository (or any of the parent directories): %s", abs)
		}
		current = parent
	}
}

// run executes git with the given args, with working directory set to gitRoot.
// Returns stdout bytes on success, or an error that includes stderr on failure.
func run(ctx context.Context, gitRoot string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %v: %w\nstderr: %s", args, err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// runWithStdin executes git with the given args and the provided stdin data.
func runWithStdin(ctx context.Context, gitRoot string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	cmd.Stdin = bytes.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %v: %w\nstderr: %s", args, err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// runRaw executes git and returns both stdout and stderr, plus the error.
// Used when callers need to inspect stderr for specific error conditions.
func runRaw(ctx context.Context, gitRoot string, stdin []byte, args ...string) (stdout []byte, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), runErr
}
