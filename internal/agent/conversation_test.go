package agent

import (
	"context"
	"encoding/json"
	"fmt"
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

// ── Phase 2 c5: no-progress guardrail ─────────────────────────────────────────

// TestToolCallKey verifies that toolCallKey produces stable, order-sensitive
// fingerprints.  The key must encode name and arguments but must be independent
// of the tool-call ID (IDs change on every generation, so including them would
// prevent loop detection even when the logical request is identical).
func TestToolCallKey(t *testing.T) {
	single := []ToolCall{{ID: "call_1", Name: "echo", Arguments: `{"text":"hi"}`}}

	// Stability: same input → same key.
	if k1, k2 := toolCallKey(single), toolCallKey(single); k1 != k2 {
		t.Errorf("same input produced different keys: %q vs %q", k1, k2)
	}

	// ID independence: changing only the ID must not change the key.
	diffID := []ToolCall{{ID: "call_999", Name: "echo", Arguments: `{"text":"hi"}`}}
	if toolCallKey(single) != toolCallKey(diffID) {
		t.Error("key should be independent of tool_call ID")
	}

	// Different name → different key.
	diffName := []ToolCall{{Name: "search", Arguments: `{"text":"hi"}`}}
	if toolCallKey(single) == toolCallKey(diffName) {
		t.Error("different names should produce different keys")
	}

	// Different arguments → different key.
	diffArgs := []ToolCall{{Name: "echo", Arguments: `{"text":"bye"}`}}
	if toolCallKey(single) == toolCallKey(diffArgs) {
		t.Error("different arguments should produce different keys")
	}

	// nil / empty → "".
	if got := toolCallKey(nil); got != "" {
		t.Errorf("nil slice: want \"\", got %q", got)
	}
	if got := toolCallKey([]ToolCall{}); got != "" {
		t.Errorf("empty slice: want \"\", got %q", got)
	}

	// Order sensitivity: [a,b] must differ from [b,a].
	ab := []ToolCall{{Name: "a"}, {Name: "b"}}
	ba := []ToolCall{{Name: "b"}, {Name: "a"}}
	if toolCallKey(ab) == toolCallKey(ba) {
		t.Error("key order should matter: [a,b] must differ from [b,a]")
	}
}

// TestNoProgressHardStop verifies that RunConversation returns a "no-progress"
// error when the model repeats the same tool call beyond maxNoProgressWarnings.
//
// Expected call count: 1 (first occurrence, no flag) + 3 (warned) + 1 (hard-stop) = 5.
func TestNoProgressHardStop(t *testing.T) {
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "application/json")
		// Always return the same name+args; IDs differ to match realistic behavior.
		fmt.Fprintf(w,
			`{"choices":[{"message":{"role":"assistant","content":null,`+
				`"tool_calls":[{"id":"call_%d","type":"function",`+
				`"function":{"name":"echo","arguments":"{\"text\":\"loop\"}"}}]},`+
				`"finish_reason":"tool_calls"}]}`,
			callNum,
		)
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	a := NewAIAgent(c)

	_, err := a.RunConversation(context.Background(), "keep looping", 10)
	if err == nil {
		t.Fatal("expected no-progress error, got nil")
	}
	if !strings.Contains(err.Error(), "no-progress") {
		t.Errorf("error = %q, want it to mention 'no-progress'", err.Error())
	}
	// 1 (normal) + 3 (warned) + 1 (hard-stop detection) = 5 total API calls.
	if callNum != 5 {
		t.Errorf("LLM call count = %d, want 5 (1 normal + 3 warned + 1 hard-stop)", callNum)
	}
}

// TestNoProgressWarningInjected verifies that after detecting a repeated
// tool-call pattern, a warning message is injected into the history so the
// next LLM call can observe it.
//
// The server returns the same tool_call on calls 1–2 and a final text answer
// on call 3.  The body of the third request is decoded and checked for a
// user-role warning with the expected content.
func TestNoProgressWarningInjected(t *testing.T) {
	callNum := 0
	var thirdBody openAIRequest
	var decErr error

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "application/json")

		if callNum == 3 {
			// Capture the third request to inspect injected warning.
			decErr = json.NewDecoder(r.Body).Decode(&thirdBody)
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)) //nolint:errcheck
			return
		}
		// Calls 1 and 2: same name+args, unique IDs (realistic).
		fmt.Fprintf(w,
			`{"choices":[{"message":{"role":"assistant","content":null,`+
				`"tool_calls":[{"id":"call_%d","type":"function",`+
				`"function":{"name":"echo","arguments":"{\"text\":\"loop\"}"}}]},`+
				`"finish_reason":"tool_calls"}]}`,
			callNum,
		)
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	a := NewAIAgent(c)

	got, err := a.RunConversation(context.Background(), "warn me", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content != "done" {
		t.Errorf("Content = %q, want %q", got.Content, "done")
	}
	if decErr != nil {
		t.Fatalf("failed to decode third request: %v", decErr)
	}

	// The third call's history must contain the warning injected after call 2's
	// repeated tool-call detection.  The warning must be a user-role message
	// (not system or assistant) so it feeds back to the model as user input.
	found := false
	for _, m := range thirdBody.Messages {
		if strings.Contains(m.Content, "no-progress warning") {
			found = true
			if m.Role != RoleUser {
				t.Errorf("warning message role = %q, want %q", m.Role, RoleUser)
			}
			break
		}
	}
	if !found {
		t.Errorf("no-progress warning not found in messages sent to 3rd LLM call; messages = %+v",
			thirdBody.Messages)
	}
}

// TestNoProgressResetOnChange verifies that the no-progress counter is reset
// to zero when the model switches to a different tool-call pattern.  Without
// the reset, a 2-warning sequence on pattern A followed by a 2-warning
// sequence on pattern B would incorrectly trigger the hard stop on the fourth
// detection.
func TestNoProgressResetOnChange(t *testing.T) {
	// Sequence:
	//  Call 1: echo("a")  → first occurrence, no flag
	//  Call 2: echo("a")  → noProgress, warnCount=1 (warn)
	//  Call 3: echo("b")  → different pattern, warnCount resets to 0
	//  Call 4: echo("b")  → noProgress, warnCount=1 (warn)
	//  Call 5: final text → conversation ends cleanly
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "application/json")
		var resp string
		switch callNum {
		case 1, 2:
			resp = fmt.Sprintf(
				`{"choices":[{"message":{"role":"assistant","content":null,`+
					`"tool_calls":[{"id":"call_%d","type":"function",`+
					`"function":{"name":"echo","arguments":"{\"text\":\"a\"}"}}]},`+
					`"finish_reason":"tool_calls"}]}`,
				callNum)
		case 3, 4:
			resp = fmt.Sprintf(
				`{"choices":[{"message":{"role":"assistant","content":null,`+
					`"tool_calls":[{"id":"call_%d","type":"function",`+
					`"function":{"name":"echo","arguments":"{\"text\":\"b\"}"}}]},`+
					`"finish_reason":"tool_calls"}]}`,
				callNum)
		default:
			resp = `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`
		}
		w.Write([]byte(resp)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	a := NewAIAgent(c)

	got, err := a.RunConversation(context.Background(), "switch tools", 10)
	if err != nil {
		t.Fatalf("unexpected error (counter should have reset on pattern change): %v", err)
	}
	if got.Content != "done" {
		t.Errorf("Content = %q, want %q", got.Content, "done")
	}
	if callNum != 5 {
		t.Errorf("LLM call count = %d, want 5", callNum)
	}
}

// TestNoProgressFirstOccurrenceNoWarning verifies that the first occurrence of
// a tool-call pattern is never flagged as a loop: a single tool call followed
// by a final text answer must produce no warning in the history sent to the
// second LLM call.
func TestNoProgressFirstOccurrenceNoWarning(t *testing.T) {
	callNum := 0
	var secondBody openAIRequest
	var decErr error

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "application/json")
		if callNum == 1 {
			w.Write([]byte( //nolint:errcheck
				`{"choices":[{"message":{"role":"assistant","content":null,` +
					`"tool_calls":[{"id":"call_x","type":"function",` +
					`"function":{"name":"echo","arguments":"{\"text\":\"hi\"}"}}]},` +
					`"finish_reason":"tool_calls"}]}`))
			return
		}
		decErr = json.NewDecoder(r.Body).Decode(&secondBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &OpenAIChatClient{
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		Model:      defaultOpenAIModel,
		httpClient: srv.Client(),
	}
	a := NewAIAgent(c)

	if _, err := a.RunConversation(context.Background(), "single echo", 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decErr != nil {
		t.Fatalf("decode error: %v", decErr)
	}

	// The history sent to the second (final) LLM call must not contain any
	// no-progress warning — the first occurrence is never a loop.
	for _, m := range secondBody.Messages {
		if strings.Contains(m.Content, "no-progress") {
			t.Errorf("unexpected no-progress warning in second call; message: %+v", m)
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
