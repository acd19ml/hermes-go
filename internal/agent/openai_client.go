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
//
// tools holds the tool schemas included in every request (Phase 2 c2
// populates this with the echo tool via echoToolSpecs(); Phase 3 will
// replace with Registry lookup). When nil, the "tools" field is omitted
// from the wire — the model behaves as if no tools are available.
type OpenAIChatClient struct {
	APIKey     string
	BaseURL    string
	Model      string
	httpClient *http.Client
	tools      []openAIToolSpec // nil until c2 sets echoToolSpecs()
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
		tools: echoToolSpecs(), // Phase 2 c2: hardcoded; Phase 3 replaces with Registry
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

// openAIToolCallFunction mirrors OpenAI's function sub-object inside a
// tool_call entry.  Arguments is a JSON-encoded string — the same shape as
// our internal ToolCall.Arguments, so no extra parse/re-encode is needed.
type openAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded, e.g. `{"text":"hi"}`
}

// openAIToolCall is one entry in the tool_calls array that the API returns
// when the model decides to invoke a tool.
type openAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`     // always "function"
	Function openAIToolCallFunction `json:"function"`
}

// openAIToolSpecBody is the function definition nested inside openAIToolSpec.
type openAIToolSpecBody struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema object
}

// openAIToolSpec is the wire representation of a tool definition sent to the
// API in the "tools" request field.  Phase 2 c1 defines the type; c2 fills
// OpenAIChatClient.tools with the echo tool via echoToolSpecs().
type openAIToolSpec struct {
	Type     string             `json:"type"` // always "function"
	Function openAIToolSpecBody `json:"function"`
}

// openAIMessage is the per-turn object in an OpenAI Chat Completions request
// or response.  Phase 2 extends it to carry tool_calls and tool_call_id.
//
// Content is tagged omitempty for two reasons:
//  1. When the model responds with tool_calls, the API returns content:null;
//     Go's json package decodes null into "" for a string field, and omitempty
//     then drops the empty field when we re-serialise the message in a later
//     turn — OpenAI accepts content being absent in that case.
//  2. Tool-result messages (role:"tool") set content but not the other fields;
//     omitempty keeps the wire clean for the fields that are zero.
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// openAIRequest is the JSON body sent to /chat/completions.
// Tools carries tool definitions when the client has any registered;
// omitempty ensures the field is absent from the wire when nil (no-tools mode).
type openAIRequest struct {
	Model    string           `json:"model"`
	Messages []openAIMessage  `json:"messages"`
	Tools    []openAIToolSpec `json:"tools,omitempty"`
}

// openAIResponse is the top-level JSON body returned by /chat/completions.
// openAIMessage now decodes both content and tool_calls automatically.
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
// assistant reply.  The mapping is bidirectional:
//
// Outbound (internal → wire):
//   - Message.ToolCalls  → openAIMessage.ToolCalls   (assistant turn in history)
//   - Message.ToolCallID → openAIMessage.ToolCallID  (role:"tool" result messages)
//   - c.tools            → openAIRequest.Tools        (tool definitions; nil = absent)
//
// Inbound (wire → internal):
//   - choices[0].message.tool_calls → Message.ToolCalls
//   - choices[0].message.content    → Message.Content ("" when API returns null)
//
// On non-200 HTTP status the raw body is included in the returned error.
func (c *OpenAIChatClient) Respond(ctx context.Context, msgs []Message) (Message, error) {
	// ── build request body ────────────────────────────────────────────────
	wireMsgs := make([]openAIMessage, len(msgs))
	for i, m := range msgs {
		wm := openAIMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		// Map internal ToolCalls → wire function sub-object.
		// Only assistant messages carry ToolCalls; other roles will have nil.
		if len(m.ToolCalls) > 0 {
			wm.ToolCalls = make([]openAIToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				wm.ToolCalls[j] = openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIToolCallFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}
		wireMsgs[i] = wm
	}
	reqBody := openAIRequest{
		Model:    c.Model,
		Messages: wireMsgs,
		Tools:    c.tools, // nil → omitted from JSON (no-tools mode)
	}

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

	// Map wire response → internal Message.
	// Content: Go's json package decodes JSON null into "" for a string
	// field, which is correct — empty string means "no text, only tool calls".
	wm := apiResp.Choices[0].Message
	result := Message{
		Role:    RoleAssistant,
		Content: wm.Content,
	}
	if len(wm.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, len(wm.ToolCalls))
		for i, wtc := range wm.ToolCalls {
			result.ToolCalls[i] = ToolCall{
				ID:        wtc.ID,
				Name:      wtc.Function.Name,
				Arguments: wtc.Function.Arguments,
			}
		}
	}
	return result, nil
}
