package cmd

import (
	"context"
	"encoding/json"

	"github.com/loreteam/lore/internal/ai"
	"github.com/loreteam/lore/internal/config"
)

// configFromContext retrieves the loaded config from the command context.
func configFromContext(ctx context.Context) *config.Config {
	if cfg, ok := ctx.Value(configKey{}).(*config.Config); ok {
		return cfg
	}
	return config.Default()
}

// newCompleter constructs a Completer from the config in ctx.
func newCompleter(ctx context.Context) ai.Completer {
	cfg := configFromContext(ctx)
	return ai.NewCompleter(cfg.AI.Reasoning)
}

// parseJSON unmarshals JSON data into v.
func parseJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
