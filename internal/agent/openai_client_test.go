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
