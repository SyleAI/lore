package ai

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/loreteam/lore/internal/config"
)

// Embedding is a dense float32 vector returned by an embeddings model.
type Embedding []float32

// Embedder computes embeddings for text inputs.
type Embedder interface {
	// Embed returns one embedding per input string, in the same order.
	Embed(texts []string) ([]Embedding, error)
}

// Completer generates text completions.
type Completer interface {
	// Complete sends a user prompt and returns the model's response text.
	Complete(prompt string) (string, error)
}

// CosineSimilarity returns the cosine similarity between two vectors.
// Both vectors must have the same length. Returns 0 for zero-length vectors.
func CosineSimilarity(a, b Embedding) float64 {
	var dot, normA, normB float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// EncodeVector serialises an Embedding to a little-endian byte slice.
func EncodeVector(e Embedding) []byte {
	buf := make([]byte, 4*len(e))
	for i, v := range e {
		binary.LittleEndian.PutUint32(buf[4*i:], math.Float32bits(v))
	}
	return buf
}

// DecodeVector deserialises a byte slice produced by EncodeVector.
func DecodeVector(b []byte) (Embedding, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("ai: decode vector: length %d is not a multiple of 4", len(b))
	}
	e := make(Embedding, len(b)/4)
	for i := range e {
		e[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return e, nil
}

// NewEmbedder constructs an Embedder from the given config.
// Returns a NoopEmbedder when the provider is unrecognised or unconfigured.
func NewEmbedder(cfg config.EmbeddingsConfig) Embedder {
	switch cfg.Provider {
	case "anthropic", "voyage":
		return newVoyageEmbedder(cfg.Model)
	default:
		return NoopEmbedder{}
	}
}

// NewCompleter constructs a Completer from the given config.
// Returns a NoopCompleter when the provider is unrecognised or unconfigured.
func NewCompleter(cfg config.ReasoningConfig) Completer {
	switch cfg.Provider {
	case "claude-cli":
		return claudeCLICompleter{}
	case "anthropic":
		return newAnthropicCompleter(cfg.Model, cfg.MaxTokens)
	default:
		return NoopCompleter{}
	}
}
