package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// execGit runs git in gitRoot with the given args and returns stdout.
func execGit(ctx context.Context, gitRoot string, args ...string) ([]byte, error) {
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
