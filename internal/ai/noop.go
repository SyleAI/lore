package ai

import "fmt"

// NoopEmbedder returns an error on every call. Used when no AI provider is configured.
type NoopEmbedder struct{}

func (NoopEmbedder) Embed(texts []string) ([]Embedding, error) {
	return nil, fmt.Errorf("ai: no embeddings provider configured")
}

type NoopCompleter struct{}

func (NoopCompleter) Complete(prompt string) (string, error) {
	return "", fmt.Errorf("ai: no reasoning provider configured")
}
