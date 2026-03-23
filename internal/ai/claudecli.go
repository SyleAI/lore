package ai

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// claudeCLICompleter shells out to the `claude` CLI (Claude Code).
// It requires no API key — it inherits the user's existing Claude Code auth.
type claudeCLICompleter struct{}

func (claudeCLICompleter) Complete(prompt string) (string, error) {
	cmd := exec.Command("claude", "-p", prompt)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("ai: claude-cli: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("ai: claude-cli: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
