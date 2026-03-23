package gitcmd

import (
	"context"
	"fmt"
	"strings"
)

// ReadBlob returns the content of the blob object identified by sha.
func ReadBlob(ctx context.Context, gitRoot, sha string) ([]byte, error) {
	out, err := run(ctx, gitRoot, "cat-file", "blob", sha)
	if err != nil {
		return nil, fmt.Errorf("gitcmd: ReadBlob %s: %w", sha, err)
	}
	return out, nil
}

// WriteBlob writes data as a new blob object in the git object store and returns its SHA.
func WriteBlob(ctx context.Context, gitRoot string, data []byte) (string, error) {
	out, err := runWithStdin(ctx, gitRoot, data, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("gitcmd: WriteBlob: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("gitcmd: WriteBlob: git hash-object returned empty SHA")
	}
	return sha, nil
}
