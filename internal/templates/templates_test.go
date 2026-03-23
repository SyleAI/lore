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
	result, err := templates.RenderPrompt("consolidate-classify", "", map[string]any{
		"Strength": 0.85,
		"Tickets": []map[string]string{
			{"ID": "abc123", "Title": "Fix login bug"},
			{"ID": "def456", "Title": "Fix authentication issue"},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, result, "0.85")
	assert.Contains(t, result, "abc123")
	assert.Contains(t, result, "Fix login bug")
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

	// Override the embedded consolidate-classify template.
	override := "OVERRIDDEN: {{ .Strength }}"
	require.NoError(t, os.WriteFile(
		filepath.Join(promptsDir, "consolidate-classify.md"),
		[]byte(override), 0644,
	))

	result, err := templates.RenderPrompt("consolidate-classify", loreDir, map[string]any{
		"Strength": 0.9,
		"Tickets":  []any{},
	})
	require.NoError(t, err)
	assert.Equal(t, "OVERRIDDEN: 0.9", result)
}

func TestRenderPromptUnknownName(t *testing.T) {
	_, err := templates.RenderPrompt("does-not-exist", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestPromptNames(t *testing.T) {
	names := templates.PromptNames()
	assert.Contains(t, names, "consolidate-classify")
	assert.Contains(t, names, "consolidate-find-groups")
	for _, n := range names {
		assert.False(t, strings.HasSuffix(n, ".md"), "names should not include .md extension")
	}
}
