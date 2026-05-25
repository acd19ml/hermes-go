package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ModelInfo is a single entry from the GET /models response.
// Only fields needed by the configure wizard are decoded.
type ModelInfo struct {
	ID            string     `json:"id"`
	Status        bool       `json:"status"`
	ModelType     ModelType  `json:"model_type"`
	Price         ModelPrice `json:"price"`
	ContextLength []int      `json:"context_length_range"`
}

// ModelPrice holds per-million-token price ranges (¥).
type ModelPrice struct {
	Input  []float64 `json:"input_price_range"`
	Output []float64 `json:"output_price_range"`
}

// IsLLM reports whether this model is a text completion model usable
// with the Chat Completions API.
func (m ModelInfo) IsLLM() bool {
	for _, t := range m.ModelType {
		if t == "llm" {
			return true
		}
	}
	return false
}

// MaxContextK returns the upper bound of context_length_range in K (÷1024),
// or 0 if the range is absent.
func (m ModelInfo) MaxContextK() int {
	if len(m.ContextLength) < 2 {
		return 0
	}
	return m.ContextLength[1] / 1024
}

// ── ModelType: string or []string in the API ─────────────────────────────────

// ModelType handles the quirk in the aiping.cn API where model_type can be
// either a plain string ("llm") or an array (["text2image","image2image"]).
type ModelType []string

// UnmarshalJSON implements json.Unmarshaler for ModelType.
func (mt *ModelType) UnmarshalJSON(b []byte) error {
	// Try string first.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s != "" {
			*mt = ModelType{s}
		}
		return nil
	}
	// Try array.
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*mt = ModelType(arr)
		return nil
	}
	// Unknown shape — treat as unknown rather than hard-failing.
	*mt = ModelType{}
	return nil
}

// ── FetchModels ───────────────────────────────────────────────────────────────

// FetchModels calls GET baseURL/models and returns all models.
// The endpoint is public; apiKey is included for implicit key validation.
func FetchModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	url := baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("models: build request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("models: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("models: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models: api error (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("models: parse response: %w", err)
	}
	return result.Data, nil
}

// LLMOnly filters models to those where IsLLM() is true.
func LLMOnly(models []ModelInfo) []ModelInfo {
	out := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		if m.IsLLM() {
			out = append(out, m)
		}
	}
	return out
}
