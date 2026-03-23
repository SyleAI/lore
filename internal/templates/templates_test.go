package templates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loreteam/lore/internal/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPromptEmbeddedFallback(t *testing.T) {
	result, err := templates.RenderPrompt("search", "", map[string]any{
		"Query": "auth timeout",
		"Tickets": []map[string]string{
			{"ID": "abc123", "Status": "open", "Content": "Fix login bug"},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, result, "auth timeout")
	assert.Contains(t, result, "abc123")
}

func TestRenderPromptUserOverride(t *testing.T) {
	loreDir := t.TempDir()
	promptsDir := filepath.Join(loreDir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0755))

	customContent := "Custom prompt for {{ .Name }}"
	require.NoError(t, os.WriteFile(
		filepath.Join(promptsDir, "my-custom.md"),
		[]byte(customContent), 0644,
	))

	result, err := templates.RenderPrompt("my-custom", loreDir, map[string]any{
		"Name": "test",
	})
	require.NoError(t, err)
	assert.Equal(t, "Custom prompt for test", result)
}

func TestRenderPromptUserOverrideTakesPrecedence(t *testing.T) {
	loreDir := t.TempDir()
	promptsDir := filepath.Join(loreDir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0755))

	override := "OVERRIDDEN: {{ .Query }}"
	require.NoError(t, os.WriteFile(
		filepath.Join(promptsDir, "search.md"),
		[]byte(override), 0644,
	))

	result, err := templates.RenderPrompt("search", loreDir, map[string]any{
		"Query":   "test query",
		"Tickets": []any{},
	})
	require.NoError(t, err)
	assert.Equal(t, "OVERRIDDEN: test query", result)
}

func TestRenderPromptUnknownName(t *testing.T) {
	_, err := templates.RenderPrompt("does-not-exist", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestPromptNames(t *testing.T) {
	names := templates.PromptNames()
	assert.Contains(t, names, "search")
	for _, n := range names {
		assert.False(t, strings.HasSuffix(n, ".md"), "names should not include .md extension")
	}
}
