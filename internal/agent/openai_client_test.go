package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── constructor tests ─────────────────────────────────────────────────────────

func TestNewOpenAIChatClientFromEnvNoKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("HERMES_CONFIG", "/dev/null") // prevent falling back to real config file
	_, err := NewOpenAIChatClientFromEnv()
	if err == nil {
		t.Fatal("expected error when OPENAI_API_KEY is empty, got nil")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Errorf("error %q should mention OPENAI_API_KEY", err.Error())
	}
}

func TestNewOpenAIChatClientFromEnvDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("HERMES_CONFIG", "/dev/null") // ensure built-in defaults are used, not config file

	c, err := NewOpenAIChatClientFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.BaseURL != defaultOpenAIBaseURL {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, defaultOpenAIBaseURL)
	}
	if c.Model != defaultOpenAIModel {
		t.Errorf("Model = %q, want %q", c.Model, defaultOpenAIModel)
	}
	if c.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", c.APIKey, "sk-test")
	}
}

// ── Respond error paths ───────────────────────────────────────────────────────

func TestOpenAIChatClientRespondBadURL(t *testing.T) {
	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    "http://127.0.0.1:1", // nothing listening on port 1
		Model:      defaultOpenAIModel,
		httpClient: &http.Client{},
	}
	msgs := []Message{{Role: RoleUser, Content: "hi"}}
	_, err := c.Respond(context.Background(), msgs)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestOpenAIChatClientRespondNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "bad-key",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	_, err := c.Respond(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q should contain status code 401", err.Error())
	}
}

// ── Respond success paths ─────────────────────────────────────────────────────

func TestOpenAIChatClientRespondSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	got, err := c.Respond(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", got.Role, RoleAssistant)
	}
	if got.Content != "hello" {
		t.Errorf("Content = %q, want %q", got.Content, "hello")
	}
}

func TestOpenAIChatClientSystemMessagePassthrough(t *testing.T) {
	var captured openAIRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}

	msgs := []Message{
		{Role: RoleSystem, Content: "You are a test assistant."},
		{Role: RoleUser, Content: "hello"},
	}
	if _, err := c.Respond(context.Background(), msgs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(captured.Messages) != 2 {
		t.Fatalf("captured %d messages, want 2", len(captured.Messages))
	}
	if captured.Messages[0].Role != RoleSystem {
		t.Errorf("messages[0].role = %q, want %q", captured.Messages[0].Role, RoleSystem)
	}
	if captured.Messages[0].Content != "You are a test assistant." {
		t.Errorf("messages[0].content = %q, want %q", captured.Messages[0].Content, "You are a test assistant.")
	}
	if captured.Messages[1].Role != RoleUser {
		t.Errorf("messages[1].role = %q, want %q", captured.Messages[1].Role, RoleUser)
	}
}

// ── Phase 2 c1: tool-call wire parsing tests ──────────────────────────────────

// TestOpenAIChatClientRespondToolCallParsed verifies that when the API returns
// an assistant message with tool_calls (and null content), Respond correctly
// populates Message.ToolCalls and leaves Content empty.
func TestOpenAIChatClientRespondToolCallParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Realistic OpenAI response: content is null, tool_calls is populated.
		w.Write([]byte(`{
			"choices":[{
				"message":{
					"role":"assistant",
					"content":null,
					"tool_calls":[{
						"id":"call_abc123",
						"type":"function",
						"function":{
							"name":"echo",
							"arguments":"{\"text\":\"hello\"}"
						}
					}]
				},
				"finish_reason":"tool_calls"
			}]
		}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}

	got, err := c.Respond(context.Background(), []Message{{Role: RoleUser, Content: "echo hello"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", got.Role, RoleAssistant)
	}
	// content:null in wire → "" in Go (json decode behaviour); omitempty drops it
	if got.Content != "" {
		t.Errorf("Content = %q, want empty (tool_calls response has null content)", got.Content)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(got.ToolCalls))
	}
	tc := got.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("ToolCalls[0].ID = %q, want %q", tc.ID, "call_abc123")
	}
	if tc.Name != "echo" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", tc.Name, "echo")
	}
	if tc.Arguments != `{"text":"hello"}` {
		t.Errorf("ToolCalls[0].Arguments = %q, want %q", tc.Arguments, `{"text":"hello"}`)
	}
}

// TestOpenAIChatClientRespondToolResultSent verifies the outbound mapping:
// an assistant message with ToolCalls and a subsequent tool-result message are
// serialised to the correct OpenAI wire format (including tool_call_id and the
// nested function sub-object).
func TestOpenAIChatClientRespondToolResultSent(t *testing.T) {
	var capturedReq openAIRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}

	// Simulate the conversation history after one tool invocation:
	//   system → user → assistant(tool_call) → tool(result)
	msgs := []Message{
		{Role: RoleSystem, Content: "You are a test assistant."},
		{Role: RoleUser, Content: "echo hello"},
		// The assistant requested an echo tool call in the previous turn.
		{Role: RoleAssistant, ToolCalls: []ToolCall{{
			ID: "call_abc", Name: "echo", Arguments: `{"text":"hello"}`,
		}}},
		// The tool result is appended before the next LLM call.
		{Role: RoleTool, ToolCallID: "call_abc", Content: "hello"},
	}

	if _, err := c.Respond(context.Background(), msgs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedReq.Messages) != 4 {
		t.Fatalf("wire message count = %d, want 4", len(capturedReq.Messages))
	}

	// ── assistant message (index 2) ──────────────────────────────────────
	asstWire := capturedReq.Messages[2]
	if asstWire.Role != RoleAssistant {
		t.Errorf("messages[2].role = %q, want %q", asstWire.Role, RoleAssistant)
	}
	// content must be absent (empty after null decode → omitempty drops it)
	if asstWire.Content != "" {
		t.Errorf("messages[2].content = %q, want empty", asstWire.Content)
	}
	if len(asstWire.ToolCalls) != 1 {
		t.Fatalf("messages[2].tool_calls len = %d, want 1", len(asstWire.ToolCalls))
	}
	wtc := asstWire.ToolCalls[0]
	if wtc.ID != "call_abc" {
		t.Errorf("tool_calls[0].id = %q, want %q", wtc.ID, "call_abc")
	}
	if wtc.Type != "function" {
		t.Errorf("tool_calls[0].type = %q, want %q", wtc.Type, "function")
	}
	if wtc.Function.Name != "echo" {
		t.Errorf("tool_calls[0].function.name = %q, want %q", wtc.Function.Name, "echo")
	}
	if wtc.Function.Arguments != `{"text":"hello"}` {
		t.Errorf("tool_calls[0].function.arguments = %q, want %q",
			wtc.Function.Arguments, `{"text":"hello"}`)
	}

	// ── tool result message (index 3) ────────────────────────────────────
	toolWire := capturedReq.Messages[3]
	if toolWire.Role != RoleTool {
		t.Errorf("messages[3].role = %q, want %q", toolWire.Role, RoleTool)
	}
	if toolWire.ToolCallID != "call_abc" {
		t.Errorf("messages[3].tool_call_id = %q, want %q", toolWire.ToolCallID, "call_abc")
	}
	if toolWire.Content != "hello" {
		t.Errorf("messages[3].content = %q, want %q", toolWire.Content, "hello")
	}
}
