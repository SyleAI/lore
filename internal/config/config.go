package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for lore.
type Config struct {
	AI      AIConfig     `yaml:"ai"`
	Events  EventsConfig `yaml:"events"`
	AgentID string       `yaml:"agent_id,omitempty"`
}

// AIConfig holds AI provider configuration.
type AIConfig struct {
	Embeddings EmbeddingsConfig `yaml:"embeddings"`
	Reasoning  ReasoningConfig  `yaml:"reasoning"`
}

// EmbeddingsConfig configures the embeddings provider.
type EmbeddingsConfig struct {
	Provider   string `yaml:"provider"`
	Model      string `yaml:"model"`
	Dimensions int    `yaml:"dimensions"`
}

// ReasoningConfig configures the reasoning/LLM provider.
type ReasoningConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

// EventsConfig configures the event adapter.
type EventsConfig struct {
	Adapter string `yaml:"adapter"`
}

// Default returns a Config populated with sensible defaults.
// By default only claude-cli is required (Claude Code); embeddings are opt-in.
func Default() *Config {
	return &Config{
		AI: AIConfig{
			Embeddings: EmbeddingsConfig{
				Provider:   "",
				Model:      "",
				Dimensions: 0,
			},
			Reasoning: ReasoningConfig{
				Provider:  "claude-cli",
				Model:     "",
				MaxTokens: 0,
			},
		},
		Events: EventsConfig{
			Adapter: "stdout",
		},
		AgentID: "",
	}
}

// Load reads loreDir/config.yaml, unmarshals it on top of Default(), and returns the result.
func Load(loreDir string) (*Config, error) {
	cfg := Default()

	path := filepath.Join(loreDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return cfg, nil
}
