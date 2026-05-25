package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/acd19ml/hermes-go/internal/config"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultOpenAIModel   = "gpt-4o-mini"
)

// OpenAIChatClient sends messages to the OpenAI Chat Completions API.
//
// Internal format is already OpenAI-style, so the wire conversion is a
// direct mapping: Message → openAIMessage, no field renaming required.
//
// Resolution order for each field (first non-empty wins):
//
//	APIKey  : OPENAI_API_KEY env → ~/.hermes-go/config.json api_key
//	BaseURL : OPENAI_BASE_URL env → config.json base_url → https://api.openai.com/v1
//	Model   : OPENAI_MODEL env → config.json model → gpt-4o-mini
type OpenAIChatClient struct {
	APIKey     string
	BaseURL    string
	Model      string
	httpClient *http.Client
}

// NewOpenAIChatClientFromEnv constructs an OpenAIChatClient using the
// resolution order described on OpenAIChatClient:
//
//  1. Environment variables (OPENAI_API_KEY, OPENAI_BASE_URL, OPENAI_MODEL).
//  2. ~/.hermes-go/config.json written by --configure wizard.
//  3. Built-in defaults (base URL and model only).
//
// Returns an error if no API key is found in either source.
func NewOpenAIChatClientFromEnv() (*OpenAIChatClient, error) {
	// Load config file as fallback (errors silently ignored — file may not exist).
	// HERMES_CONFIG env overrides the path; tests set it to /dev/null to isolate.
	var cfg config.Config
	cfgPath := os.Getenv("HERMES_CONFIG")
	if cfgPath == "" {
		cfgPath, _ = config.DefaultPath()
	}
	if cfgPath != "" {
		cfg, _ = config.Load(cfgPath)
	}

	key := firstNonEmpty(os.Getenv("OPENAI_API_KEY"), cfg.APIKey)
	if key == "" {
		return nil, fmt.Errorf(
			"no API key found — set OPENAI_API_KEY or run: hermes-go --configure")
	}

	baseURL := firstNonEmpty(os.Getenv("OPENAI_BASE_URL"), cfg.BaseURL, defaultOpenAIBaseURL)
	model := firstNonEmpty(os.Getenv("OPENAI_MODEL"), cfg.Model, defaultOpenAIModel)

	return &OpenAIChatClient{
		APIKey:  key,
		BaseURL: baseURL,
		Model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// firstNonEmpty returns the first non-empty string from vals.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── unexported wire types ────────────────────────────────────────────────────

// openAIMessage is the per-turn object in an OpenAI Chat Completions request
// or response. Phase 1 only handles text content (no tool_calls on the wire).
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIRequest is the JSON body sent to /chat/completions.
type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

// openAIResponse is the top-level JSON body returned by /chat/completions.
// Only fields needed by Phase 1 are decoded.
type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ── public method ────────────────────────────────────────────────────────────

// Respond sends msgs to the OpenAI Chat Completions API and returns the
// assistant reply. The first element of msgs is conventionally a system
// message injected by AIAgent (Phase 1 c3); OpenAI accepts it verbatim as
// the first array element, so no special handling is needed here.
//
// On non-200 HTTP status the raw body is included in the returned error.
// On success, choices[0].message is wrapped into a Message{RoleAssistant}.
func (c *OpenAIChatClient) Respond(ctx context.Context, msgs []Message) (Message, error) {
	// ── build request body ────────────────────────────────────────────────
	wireMsgs := make([]openAIMessage, len(msgs))
	for i, m := range msgs {
		wireMsgs[i] = openAIMessage{Role: m.Role, Content: m.Content}
	}
	reqBody := openAIRequest{Model: c.Model, Messages: wireMsgs}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, fmt.Errorf("openai client: marshal request: %w", err)
	}

	// ── build HTTP request ────────────────────────────────────────────────
	url := c.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Message{}, fmt.Errorf("openai client: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	// ── execute ───────────────────────────────────────────────────────────
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("openai client: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, fmt.Errorf("openai client: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, respBody)
	}

	// ── decode ────────────────────────────────────────────────────────────
	var apiResp openAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return Message{}, fmt.Errorf("openai client: decode response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return Message{}, fmt.Errorf("openai client: response contained no choices")
	}

	return Message{
		Role:    RoleAssistant,
		Content: apiResp.Choices[0].Message.Content,
	}, nil
}
