package cmd

import (
	"context"

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

// newEmbedder constructs an Embedder from the config in ctx.
func newEmbedder(ctx context.Context) ai.Embedder {
	cfg := configFromContext(ctx)
	return ai.NewEmbedder(cfg.AI.Embeddings)
}

// newCompleter constructs a Completer from the config in ctx.
func newCompleter(ctx context.Context) ai.Completer {
	cfg := configFromContext(ctx)
	return ai.NewCompleter(cfg.AI.Reasoning)
}
