package gitcmd_test

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRepo creates a temp directory with a fresh git repo, configures user identity, and returns the path.
func newTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test User"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "setup: %v\n%s", args, out)
	}

	return dir
}

func TestWriteReadBlob(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()

	data := []byte("hello, lore blob content\nwith newline")
	sha, err := gitcmd.WriteBlob(ctx, dir, data)
	require.NoError(t, err)
	require.NotEmpty(t, sha)

	got, err := gitcmd.ReadBlob(ctx, dir, sha)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestWriteReadRef(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()

	data := []byte("ref content for TestWriteReadRef")
	sha, err := gitcmd.WriteBlob(ctx, dir, data)
	require.NoError(t, err)

	ref := "refs/test/myref"
	err = gitcmd.WriteRef(ctx, dir, ref, sha)
	require.NoError(t, err)

	gotSHA, err := gitcmd.ReadRef(ctx, dir, ref)
	require.NoError(t, err)
	assert.Equal(t, sha, gotSHA)
}

func TestListRefs(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()

	prefix := "refs/test/list"
	expected := map[string]string{}

	for i := 0; i < 3; i++ {
		data := []byte(fmt.Sprintf("content for ref %d", i))
		sha, err := gitcmd.WriteBlob(ctx, dir, data)
		require.NoError(t, err)

		ref := fmt.Sprintf("%s/ref%d", prefix, i)
		err = gitcmd.WriteRef(ctx, dir, ref, sha)
		require.NoError(t, err)
		expected[ref] = sha
	}

	refs, err := gitcmd.ListRefs(ctx, dir, prefix)
	require.NoError(t, err)
	assert.Equal(t, expected, refs)
}

func TestDeleteRef(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()

	data := []byte("data to be deleted")
	sha, err := gitcmd.WriteBlob(ctx, dir, data)
	require.NoError(t, err)

	ref := "refs/test/todelete"
	err = gitcmd.WriteRef(ctx, dir, ref, sha)
	require.NoError(t, err)

	// Verify it exists
	_, err = gitcmd.ReadRef(ctx, dir, ref)
	require.NoError(t, err)

	// Delete it
	err = gitcmd.DeleteRef(ctx, dir, ref)
	require.NoError(t, err)

	// Verify it's gone
	_, err = gitcmd.ReadRef(ctx, dir, ref)
	assert.ErrorIs(t, err, gitcmd.ErrRefNotFound)
}

func TestCAS_Success(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()

	data := []byte("initial content")
	sha, err := gitcmd.WriteBlob(ctx, dir, data)
	require.NoError(t, err)

	ref := "refs/test/cas-success"
	err = gitcmd.WriteRef(ctx, dir, ref, sha)
	require.NoError(t, err)

	newData := []byte("updated content")
	newSHA, err := gitcmd.WriteBlob(ctx, dir, newData)
	require.NoError(t, err)

	// CAS from current SHA to new SHA
	err = gitcmd.CAS(ctx, dir, ref, newSHA, sha)
	require.NoError(t, err)

	// Verify the ref was updated
	gotSHA, err := gitcmd.ReadRef(ctx, dir, ref)
	require.NoError(t, err)
	assert.Equal(t, newSHA, gotSHA)
}

func TestCAS_Conflict(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()

	data := []byte("initial content for conflict test")
	sha, err := gitcmd.WriteBlob(ctx, dir, data)
	require.NoError(t, err)

	ref := "refs/test/cas-conflict"
	err = gitcmd.WriteRef(ctx, dir, ref, sha)
	require.NoError(t, err)

	// Update the ref to a new SHA (simulating another process)
	updatedData := []byte("someone else updated this")
	updatedSHA, err := gitcmd.WriteBlob(ctx, dir, updatedData)
	require.NoError(t, err)
	err = gitcmd.WriteRef(ctx, dir, ref, updatedSHA)
	require.NoError(t, err)

	// Try to CAS with the stale SHA — should fail with ErrCASConflict
	newerData := []byte("our update that should fail")
	newerSHA, err := gitcmd.WriteBlob(ctx, dir, newerData)
	require.NoError(t, err)

	err = gitcmd.CAS(ctx, dir, ref, newerSHA, sha)
	assert.ErrorIs(t, err, gitcmd.ErrCASConflict)
}

func TestCAS_Concurrent(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()

	// Write initial blob and ref
	initialData := []byte("initial for concurrent cas")
	initialSHA, err := gitcmd.WriteBlob(ctx, dir, initialData)
	require.NoError(t, err)

	ref := "refs/test/cas-concurrent"
	err = gitcmd.WriteRef(ctx, dir, ref, initialSHA)
	require.NoError(t, err)

	// Pre-create 5 different new SHAs
	newSHAs := make([]string, 5)
	for i := 0; i < 5; i++ {
		data := []byte(fmt.Sprintf("concurrent update %d", i))
		blobSHA, werr := gitcmd.WriteBlob(ctx, dir, data)
		require.NoError(t, werr)
		newSHAs[i] = blobSHA
	}

	var (
		wg           sync.WaitGroup
		successCount int64
	)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			casErr := gitcmd.CAS(ctx, dir, ref, newSHAs[idx], initialSHA)
			if casErr == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int64(1), successCount, "exactly one CAS should succeed")
}
