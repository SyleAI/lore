package templates

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// SkillFiles embeds all skill markdown templates.
//
//go:embed skills
var SkillFiles embed.FS

// PromptFiles embeds all default AI prompt templates.
//
//go:embed prompts
var PromptFiles embed.FS

// DefaultConfig is the default .lore/config.yaml template.
//
//go:embed defaults/config.yaml
var DefaultConfig []byte

// DefaultPolicy is the default refs/tickets/policy YAML blob.
//
//go:embed defaults/policy.yaml
var DefaultPolicy []byte

// DefaultGraph is the default refs/tickets/graph YAML blob.
//
//go:embed defaults/graph.yaml
var DefaultGraph []byte

// DefaultImprovement is the default improvement.yaml template.
//
//go:embed defaults/improvement.yaml
var DefaultImprovement []byte

// RenderPrompt renders prompt template name (e.g. "consolidate-classify") with data.
// It checks <loreDir>/prompts/<name>.md first, then falls back to the embedded default.
// If loreDir is empty, only the embedded default is used.
func RenderPrompt(name, loreDir string, data any) (string, error) {
	src, err := loadPromptSource(name, loreDir)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(name).Parse(src)
	if err != nil {
		return "", fmt.Errorf("templates: parse prompt %s: %w", name, err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("templates: render prompt %s: %w", name, err)
	}
	return sb.String(), nil
}

// loadPromptSource returns the raw template source for name.
func loadPromptSource(name, loreDir string) (string, error) {
	if loreDir != "" {
		userPath := filepath.Join(loreDir, "prompts", name+".md")
		if data, err := os.ReadFile(userPath); err == nil {
			return string(data), nil
		}
	}
	data, err := PromptFiles.ReadFile("prompts/" + name + ".md")
	if err != nil {
		return "", fmt.Errorf("templates: prompt %q not found: %w", name, err)
	}
	return string(data), nil
}

// PromptNames lists all embedded prompt template names (without .md extension).
func PromptNames() []string {
	entries, _ := PromptFiles.ReadDir("prompts")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ".md") {
			names = append(names, strings.TrimSuffix(n, ".md"))
		}
	}
	return names
}
