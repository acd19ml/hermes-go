package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── basic termination ─────────────────────────────────────────────────────────

// TestRunConversationNoTools verifies that when the client returns no
// ToolCalls, RunConversation terminates after exactly one LLM call and
// returns the assistant message.
func TestRunConversationNoTools(t *testing.T) {
	a := NewAIAgent(StaticResponder{})

	got, err := a.RunConversation(context.Background(), "hello", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", got.Role, RoleAssistant)
	}
	if len(got.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %v, want empty", got.ToolCalls)
	}
	// StaticResponder echoes the user message
	if !strings.Contains(got.Content, "hello") {
		t.Errorf("Content = %q, should contain the echoed user message", got.Content)
	}
}

// ── system prompt prepend ─────────────────────────────────────────────────────

// TestRunConversationPrependsSystemPrompt verifies that RunConversation places
// the system message as messages[0] before the user message, matching the
// byte-static invariant established in Phase 1.
func TestRunConversationPrependsSystemPrompt(t *testing.T) {
	var allCalls [][]Message
	spy := spyClient{
		captureAll: &allCalls,
		reply:      Message{Role: RoleAssistant, Content: "done"},
	}

	a := NewAIAgent(spy)
	if _, err := a.RunConversation(context.Background(), "ping", 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(allCalls) != 1 {
		t.Fatalf("LLM call count = %d, want 1", len(allCalls))
	}
	msgs := allCalls[0]
	if len(msgs) != 2 {
		t.Fatalf("message count in first call = %d, want 2 (system+user)", len(msgs))
	}
	if msgs[0].Role != RoleSystem {
		t.Errorf("messages[0].role = %q, want %q", msgs[0].Role, RoleSystem)
	}
	if msgs[0].Content != defaultSystemPrompt {
		t.Errorf("messages[0].content = %q, want defaultSystemPrompt", msgs[0].Content)
	}
	if msgs[1].Role != RoleUser {
		t.Errorf("messages[1].role = %q, want %q", msgs[1].Role, RoleUser)
	}
	if msgs[1].Content != "ping" {
		t.Errorf("messages[1].content = %q, want %q", msgs[1].Content, "ping")
	}
}

// ── full tool-call round trip ─────────────────────────────────────────────────

// TestRunConversationEchoOneRound exercises the complete
//
//	LLM → tool_call → echo dispatch → tool result → LLM → final text
//
// loop using a two-response httptest server.  The second request is also
// captured and inspected to ensure the full conversation history is forwarded
// (system + user + assistant-with-tool_calls + tool-result).
func TestRunConversationEchoOneRound(t *testing.T) {
	const toolCallID = "call_echo_01"

	// pre-encode the two canned responses
	firstResp := `{"choices":[{"message":{"role":"assistant","content":null,` +
		`"tool_calls":[{"id":"` + toolCallID + `","type":"function",` +
		`"function":{"name":"echo","arguments":"{\"text\":\"ping\"}"}}]},` +
		`"finish_reason":"tool_calls"}]}`
	secondResp := `{"choices":[{"message":{"role":"assistant","content":"echo returned: ping"}}]}`

	callNum := 0
	var (
		secondReq    openAIRequest
		secondDecErr error
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch callNum {
		case 1:
			w.Write([]byte(firstResp)) //nolint:errcheck
		case 2:
			// Capture the second request to verify conversation history.
			secondDecErr = json.NewDecoder(r.Body).Decode(&secondReq)
			w.Write([]byte(secondResp)) //nolint:errcheck
		default:
			http.Error(w, "unexpected call", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	a := NewAIAgent(c)

	got, err := a.RunConversation(context.Background(), "echo ping", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ── final answer ──────────────────────────────────────────────────────
	if got.Role != RoleAssistant {
		t.Errorf("final Role = %q, want %q", got.Role, RoleAssistant)
	}
	if got.Content != "echo returned: ping" {
		t.Errorf("final Content = %q, want %q", got.Content, "echo returned: ping")
	}
	if callNum != 2 {
		t.Errorf("LLM call count = %d, want 2", callNum)
	}

	// ── second call's history ─────────────────────────────────────────────
	if secondDecErr != nil {
		t.Fatalf("failed to decode second request body: %v", secondDecErr)
	}
	// history must be: system(0) + user(1) + assistant-tool_call(2) + tool-result(3)
	if len(secondReq.Messages) != 4 {
		t.Fatalf("second call message count = %d, want 4", len(secondReq.Messages))
	}

	sys := secondReq.Messages[0]
	if sys.Role != RoleSystem {
		t.Errorf("history[0].role = %q, want %q", sys.Role, RoleSystem)
	}

	usr := secondReq.Messages[1]
	if usr.Role != RoleUser {
		t.Errorf("history[1].role = %q, want %q", usr.Role, RoleUser)
	}

	asst := secondReq.Messages[2]
	if asst.Role != RoleAssistant {
		t.Errorf("history[2].role = %q, want %q", asst.Role, RoleAssistant)
	}
	if len(asst.ToolCalls) != 1 {
		t.Fatalf("history[2].tool_calls len = %d, want 1", len(asst.ToolCalls))
	}
	if asst.ToolCalls[0].ID != toolCallID {
		t.Errorf("history[2].tool_calls[0].id = %q, want %q", asst.ToolCalls[0].ID, toolCallID)
	}

	tr := secondReq.Messages[3]
	if tr.Role != RoleTool {
		t.Errorf("history[3].role = %q, want %q", tr.Role, RoleTool)
	}
	if tr.ToolCallID != toolCallID {
		t.Errorf("history[3].tool_call_id = %q, want %q", tr.ToolCallID, toolCallID)
	}
	// echo tool returns the text argument unchanged
	if tr.Content != "ping" {
		t.Errorf("history[3].content = %q, want %q (echo result)", tr.Content, "ping")
	}
}

// ── budget exhaustion ─────────────────────────────────────────────────────────

// TestRunConversationBudgetExhausted verifies that when the model keeps
// returning tool_calls and the iteration budget is consumed, RunConversation
// returns an error mentioning "budget exhausted" and does not loop forever.
func TestRunConversationBudgetExhausted(t *testing.T) {
	// Always return a tool_call — the loop can never reach a final answer.
	alwaysToolCall := `{"choices":[{"message":{"role":"assistant","content":null,` +
		`"tool_calls":[{"id":"call_loop","type":"function",` +
		`"function":{"name":"echo","arguments":"{\"text\":\"loop\"}"}}]},` +
		`"finish_reason":"tool_calls"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(alwaysToolCall)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	a := NewAIAgent(c)

	// maxIter=1: one LLM call is made, returns tool_call, tool is executed,
	// then the next Consume() fails → error.
	_, err := a.RunConversation(context.Background(), "keep looping", 1)
	if err == nil {
		t.Fatal("expected error when budget exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "budget exhausted") {
		t.Errorf("error = %q, want it to mention 'budget exhausted'", err.Error())
	}
}

// ── Phase 2 c4: tool-result integrity ─────────────────────────────────────────

// TestDropOrphanToolResultsNoOrphans verifies that a well-formed history is
// returned unchanged: valid tool results (ToolCallID present in a preceding
// assistant message) must survive the cleanup.
func TestDropOrphanToolResultsNoOrphans(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "u"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1", Name: "echo"}}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "result"},
	}
	got := dropOrphanToolResults(msgs)
	if len(got) != len(msgs) {
		t.Errorf("len(got) = %d, want %d (no orphans should be removed)", len(got), len(msgs))
	}
}

// TestDropOrphanToolResultsWithOrphan verifies that a tool-result whose
// ToolCallID does not appear in any preceding assistant message is removed.
func TestDropOrphanToolResultsWithOrphan(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "u"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1", Name: "echo"}}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "ok"},
		{Role: RoleTool, ToolCallID: "call_STALE", Content: "??"},  // orphan
	}
	got := dropOrphanToolResults(msgs)
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4 (orphan removed)", len(got))
	}
	for _, m := range got {
		if m.Role == RoleTool && m.ToolCallID == "call_STALE" {
			t.Error("orphan tool result still present after dropOrphanToolResults")
		}
	}
}

// TestDropOrphanToolResultsEmptyToolCallID verifies that a tool-result with an
// empty ToolCallID (malformed) is treated as an orphan and dropped.
func TestDropOrphanToolResultsEmptyToolCallID(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "u"},
		{Role: RoleTool, ToolCallID: "", Content: "malformed"}, // no matching assistant
	}
	got := dropOrphanToolResults(msgs)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (empty-ID tool result dropped)", len(got))
	}
	if got[0].Role != RoleUser {
		t.Errorf("got[0].role = %q, want user", got[0].Role)
	}
}

// TestDropOrphanToolResultsMultipleCalls verifies that tool results from
// multiple sequential assistant messages are all retained when valid.
func TestDropOrphanToolResultsMultipleCalls(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "u"},
		// first assistant turn
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1"}}},
		{Role: RoleTool, ToolCallID: "c1", Content: "r1"},
		// second assistant turn
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c2"}}},
		{Role: RoleTool, ToolCallID: "c2", Content: "r2"},
		// orphan injected between them
		{Role: RoleTool, ToolCallID: "GHOST", Content: "boo"},
	}
	got := dropOrphanToolResults(msgs)
	if len(got) != 5 {
		t.Fatalf("len(got) = %d, want 5 (one orphan removed)", len(got))
	}
}

// TestValidateToolPairingOK verifies that a valid history (all tool results
// have a matching preceding assistant tool_call) returns nil.
func TestValidateToolPairingOK(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_a"}}},
		{Role: RoleTool, ToolCallID: "call_a", Content: "ok"},
	}
	if err := validateToolPairing(msgs); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateToolPairingOrphan verifies that an orphan tool result causes
// validateToolPairing to return a non-nil error mentioning the offending ID.
func TestValidateToolPairingOrphan(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_a"}}},
		{Role: RoleTool, ToolCallID: "call_a", Content: "ok"},
		{Role: RoleTool, ToolCallID: "call_ORPHAN", Content: "??"},
	}
	err := validateToolPairing(msgs)
	if err == nil {
		t.Fatal("expected error for orphan tool result, got nil")
	}
	if !strings.Contains(err.Error(), "call_ORPHAN") {
		t.Errorf("error %q should mention the orphan ID", err.Error())
	}
}

// TestValidateToolPairingEmpty verifies that an empty or tool-free history
// is considered valid (no orphans to report).
func TestValidateToolPairingEmpty(t *testing.T) {
	cases := [][]Message{
		nil,
		{},
		{{Role: RoleSystem, Content: "sys"}, {Role: RoleUser, Content: "u"}},
	}
	for _, msgs := range cases {
		if err := validateToolPairing(msgs); err != nil {
			t.Errorf("validateToolPairing(%v) = %v, want nil", msgs, err)
		}
	}
}

// TestRunConversationOrphanCleaned verifies that the msgs passed to each LLM
// call inside RunConversation contain no orphan tool results.  The httptest
// server runs validateToolPairing on the decoded request and returns HTTP 400
// on failure, which causes RunConversation to return an error — making the
// test fail if orphans reach the API.
func TestRunConversationOrphanCleaned(t *testing.T) {
	const toolCallID = "call_clean_01"

	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++

		var req openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "decode error", http.StatusBadRequest)
			return
		}

		// Reconstruct internal Messages from the wire format so we can run
		// validateToolPairing (which operates on []Message, not []openAIMessage).
		internal := make([]Message, len(req.Messages))
		for i, wm := range req.Messages {
			internal[i] = Message{Role: wm.Role, ToolCallID: wm.ToolCallID}
			for _, wtc := range wm.ToolCalls {
				internal[i].ToolCalls = append(internal[i].ToolCalls, ToolCall{ID: wtc.ID})
			}
		}
		if err := validateToolPairing(internal); err != nil {
			// Orphan found — fail the HTTP call so RunConversation returns an error.
			http.Error(w, "orphan: "+err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch callNum {
		case 1:
			// First call: model requests echo tool.
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,` + //nolint:errcheck
				`"tool_calls":[{"id":"` + toolCallID + `","type":"function",` +
				`"function":{"name":"echo","arguments":"{\"text\":\"ok\"}"}}]},` +
				`"finish_reason":"tool_calls"}]}`))
		default:
			// Second call: final answer.
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)) //nolint:errcheck
		}
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	a := NewAIAgent(c)

	_, err := a.RunConversation(context.Background(), "clean history test", 5)
	if err != nil {
		t.Fatalf("RunConversation failed — possibly orphan tool result in request: %v", err)
	}
	if callNum != 2 {
		t.Errorf("LLM call count = %d, want 2", callNum)
	}
}
