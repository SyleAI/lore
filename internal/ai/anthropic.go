package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// doPost sends a JSON POST request with the given headers and returns the response body.
func doPost(url string, headers map[string]string, reqBody []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

// voyageEmbedder calls the Voyage AI embeddings API.
// API key is read from VOYAGE_API_KEY, falling back to ANTHROPIC_API_KEY.
type voyageEmbedder struct {
	model  string
	apiKey string
}

func newVoyageEmbedder(model string) *voyageEmbedder {
	if model == "" {
		model = "voyage-3"
	}
	apiKey := os.Getenv("VOYAGE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &voyageEmbedder{model: model, apiKey: apiKey}
}

func (v *voyageEmbedder) Embed(texts []string) ([]Embedding, error) {
	if v.apiKey == "" {
		return nil, fmt.Errorf("ai: voyage: VOYAGE_API_KEY or ANTHROPIC_API_KEY required")
	}

	reqBody, err := json.Marshal(map[string]any{
		"model": v.model,
		"input": texts,
	})
	if err != nil {
		return nil, fmt.Errorf("ai: voyage: marshal request: %w", err)
	}

	respBody, err := doPost(
		"https://api.voyageai.com/v1/embeddings",
		map[string]string{"Authorization": "Bearer " + v.apiKey},
		reqBody,
	)
	if err != nil {
		return nil, fmt.Errorf("ai: voyage: %w", err)
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("ai: voyage: parse response: %w", err)
	}

	embeddings := make([]Embedding, len(texts))
	for _, d := range result.Data {
		if d.Index >= len(embeddings) {
			return nil, fmt.Errorf("ai: voyage: unexpected index %d", d.Index)
		}
		embeddings[d.Index] = Embedding(d.Embedding)
	}
	return embeddings, nil
}

// anthropicCompleter calls the Anthropic Messages API.
type anthropicCompleter struct {
	model     string
	maxTokens int
	apiKey    string
}

func newAnthropicCompleter(model string, maxTokens int) *anthropicCompleter {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if maxTokens == 0 {
		maxTokens = 1024
	}
	return &anthropicCompleter{
		model:     model,
		maxTokens: maxTokens,
		apiKey:    os.Getenv("ANTHROPIC_API_KEY"),
	}
}

func (a *anthropicCompleter) Complete(prompt string) (string, error) {
	if a.apiKey == "" {
		return "", fmt.Errorf("ai: anthropic: ANTHROPIC_API_KEY required")
	}

	reqBody, err := json.Marshal(map[string]any{
		"model":      a.model,
		"max_tokens": a.maxTokens,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ai: anthropic: marshal request: %w", err)
	}

	respBody, err := doPost(
		"https://api.anthropic.com/v1/messages",
		map[string]string{
			"x-api-key":         a.apiKey,
			"anthropic-version": "2023-06-01",
		},
		reqBody,
	)
	if err != nil {
		return "", fmt.Errorf("ai: anthropic: %w", err)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("ai: anthropic: parse response: %w", err)
	}
	for _, c := range result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("ai: anthropic: no text content in response")
}
