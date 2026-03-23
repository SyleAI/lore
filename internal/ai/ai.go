package ai

import (
	"github.com/loreteam/lore/internal/config"
)

// Completer generates text completions.
type Completer interface {
	// Complete sends a user prompt and returns the model's response text.
	Complete(prompt string) (string, error)
}

// NewCompleter constructs a Completer from the given config.
// Falls back to the claude CLI completer when the provider is unrecognised or unconfigured.
func NewCompleter(cfg config.ReasoningConfig) Completer {
	switch cfg.Provider {
	case "anthropic":
		return newAnthropicCompleter(cfg.Model, cfg.MaxTokens)
	default:
		return claudeCLICompleter{}
	}
}
