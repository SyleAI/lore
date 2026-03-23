package gitcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrCASConflict = errors.New("cas conflict: ref was modified by another process")
var ErrRefNotFound = errors.New("ref not found")

// ReadRef returns the SHA that the given ref points to.
// Returns ErrRefNotFound if the ref does not exist.
func ReadRef(ctx context.Context, gitRoot, ref string) (string, error) {
	stdout, stderr, err := runRaw(ctx, gitRoot, nil, "rev-parse", "--verify", ref)
	if err != nil {
		s := string(stderr)
		if strings.Contains(s, "unknown revision") ||
			strings.Contains(s, "Needed a single revision") {
			return "", ErrRefNotFound
		}
		return "", fmt.Errorf("gitcmd: ReadRef %s: %w\nstderr: %s", ref, err, s)
	}
	return strings.TrimSpace(string(stdout)), nil
}

// WriteRef creates or updates ref to point to sha.
func WriteRef(ctx context.Context, gitRoot, ref, sha string) error {
	_, err := run(ctx, gitRoot, "update-ref", ref, sha)
	if err != nil {
		return fmt.Errorf("gitcmd: WriteRef %s: %w", ref, err)
	}
	return nil
}

// DeleteRef removes the given ref.
func DeleteRef(ctx context.Context, gitRoot, ref string) error {
	_, err := run(ctx, gitRoot, "update-ref", "-d", ref)
	if err != nil {
		return fmt.Errorf("gitcmd: DeleteRef %s: %w", ref, err)
	}
	return nil
}

// ListRefs returns a map of refname -> sha for all refs under the given prefix.
func ListRefs(ctx context.Context, gitRoot, prefix string) (map[string]string, error) {
	out, err := run(ctx, gitRoot, "for-each-ref", "--format=%(objectname) %(refname)", prefix)
	if err != nil {
		return nil, fmt.Errorf("gitcmd: ListRefs %s: %w", prefix, err)
	}

	result := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		sha := parts[0]
		refname := parts[1]
		result[refname] = sha
	}
	return result, nil
}

// CAS atomically updates ref from oldSHA to newSHA using git update-ref --stdin -z.
// Returns ErrCASConflict if git exits non-zero with "does not match" or "unable to lock" in stderr.
func CAS(ctx context.Context, gitRoot, ref, newSHA, oldSHA string) error {
	// Format: "update <ref>\x00<newSHA>\x00<oldSHA>\x00"
	input := "update " + ref + "\x00" + newSHA + "\x00" + oldSHA + "\x00"

	_, stderr, err := runRaw(ctx, gitRoot, []byte(input), "update-ref", "--stdin", "-z")
	if err != nil {
		stderrStr := string(stderr)
		if strings.Contains(stderrStr, "does not match") ||
			strings.Contains(stderrStr, "unable to lock") ||
			strings.Contains(stderrStr, "is at") {
			return ErrCASConflict
		}
		return fmt.Errorf("gitcmd: CAS %s: %w\nstderr: %s", ref, err, stderrStr)
	}
	return nil
}
