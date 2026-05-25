package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMessageUserRoundTrip verifies that a plain user message survives a
// JSON marshal → unmarshal cycle and that empty optional fields are absent
// from the serialised form.
func TestMessageUserRoundTrip(t *testing.T) {
	orig := Message{
		Role:    RoleUser,
		Content: "hello world",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// omitempty fields must not appear in JSON
	raw := string(data)
	if strings.Contains(raw, "tool_calls") {
		t.Errorf("JSON contains tool_calls but should not: %s", raw)
	}
	if strings.Contains(raw, "tool_call_id") {
		t.Errorf("JSON contains tool_call_id but should not: %s", raw)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Role != orig.Role {
		t.Errorf("Role = %q, want %q", got.Role, orig.Role)
	}
	if got.Content != orig.Content {
		t.Errorf("Content = %q, want %q", got.Content, orig.Content)
	}
	if len(got.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %v, want empty", got.ToolCalls)
	}
	if got.ToolCallID != "" {
		t.Errorf("ToolCallID = %q, want empty", got.ToolCallID)
	}
}

// TestMessageAssistantWithToolCallsRoundTrip verifies that an assistant
// message carrying ToolCalls round-trips correctly, and that Arguments is
// preserved as a literal JSON string without double-encoding.
func TestMessageAssistantWithToolCallsRoundTrip(t *testing.T) {
	orig := Message{
		Role:    RoleAssistant,
		Content: "Let me check the weather.",
		ToolCalls: []ToolCall{
			{
				ID:        "call_001",
				Name:      "get_weather",
				Arguments: `{"location":"San Francisco","unit":"celsius"}`,
			},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", got.Role, RoleAssistant)
	}
	if got.Content != orig.Content {
		t.Errorf("Content = %q, want %q", got.Content, orig.Content)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(got.ToolCalls))
	}

	tc := got.ToolCalls[0]
	if tc.ID != "call_001" {
		t.Errorf("ToolCall.ID = %q, want %q", tc.ID, "call_001")
	}
	if tc.Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want %q", tc.Name, "get_weather")
	}
	// Arguments must survive as-is: no double-encoding, no escaping of inner quotes.
	if tc.Arguments != orig.ToolCalls[0].Arguments {
		t.Errorf("ToolCall.Arguments = %q, want %q", tc.Arguments, orig.ToolCalls[0].Arguments)
	}

	// Sanity-check: Arguments must be valid JSON parseable into a map.
	var args map[string]string
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		t.Errorf("ToolCall.Arguments is not valid JSON: %v", err)
	}
	if args["location"] != "San Francisco" {
		t.Errorf("args[location] = %q, want %q", args["location"], "San Francisco")
	}
}

// TestMessageToolRoundTrip verifies that a tool-result message (role:"tool")
// round-trips with ToolCallID intact.
func TestMessageToolRoundTrip(t *testing.T) {
	orig := Message{
		Role:       RoleTool,
		ToolCallID: "call_001",
		Content:    "20°C and sunny",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Role != RoleTool {
		t.Errorf("Role = %q, want %q", got.Role, RoleTool)
	}
	if got.ToolCallID != orig.ToolCallID {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, orig.ToolCallID)
	}
	if got.Content != orig.Content {
		t.Errorf("Content = %q, want %q", got.Content, orig.Content)
	}
}

// TestToolResultRoundTrip verifies that ToolResult survives a round-trip
// and that Name and IsError:true both appear in JSON.
func TestToolResultRoundTrip(t *testing.T) {
	orig := ToolResult{
		ToolCallID: "call_001",
		Name:       "get_weather",
		Content:    "error: location not found",
		IsError:    true,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// is_error must appear when true
	if !strings.Contains(string(data), `"is_error":true`) {
		t.Errorf("JSON missing is_error:true: %s", string(data))
	}

	var got ToolResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ToolCallID != orig.ToolCallID {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, orig.ToolCallID)
	}
	if got.Name != orig.Name {
		t.Errorf("Name = %q, want %q", got.Name, orig.Name)
	}
	if got.Content != orig.Content {
		t.Errorf("Content = %q, want %q", got.Content, orig.Content)
	}
	if !got.IsError {
		t.Errorf("IsError = false, want true")
	}
}

// TestToolResultOmitEmptyIsError verifies that IsError:false is absent from
// JSON output (omitempty), keeping the wire format clean for OpenAI providers
// that do not recognise this field.
func TestToolResultOmitEmptyIsError(t *testing.T) {
	tr := ToolResult{
		ToolCallID: "call_002",
		Name:       "read_file",
		Content:    "file contents",
		IsError:    false,
	}

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "is_error") {
		t.Errorf("JSON contains is_error but IsError is false — omitempty not working: %s", string(data))
	}
}

// TestToolResultMissingToolCallID is the required failure-path test.
// When JSON lacks tool_call_id, unmarshal must produce an empty string (Go
// zero value) rather than an error or panic. This documents the zero-value
// contract: callers are responsible for validating non-empty ToolCallID
// before appending a ToolResult to history (enforced in PR 2.4).
func TestToolResultMissingToolCallID(t *testing.T) {
	raw := `{"name":"echo","content":"hello"}`

	var tr ToolResult
	if err := json.Unmarshal([]byte(raw), &tr); err != nil {
		t.Fatalf("unmarshal failed unexpectedly: %v", err)
	}
	if tr.ToolCallID != "" {
		t.Errorf("ToolCallID = %q, want empty string (zero value)", tr.ToolCallID)
	}
	if tr.Name != "echo" {
		t.Errorf("Name = %q, want %q", tr.Name, "echo")
	}
}
