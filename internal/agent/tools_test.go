package agent

import (
	"context"
	"strings"
	"testing"
)

// ── DispatchTool / echo tool ──────────────────────────────────────────────────

func TestDispatchToolEchoOK(t *testing.T) {
	tc := ToolCall{ID: "call_1", Name: "echo", Arguments: `{"text":"hello world"}`}
	got := DispatchTool(context.Background(), tc)

	if got.IsError {
		t.Errorf("IsError = true, want false; content: %s", got.Content)
	}
	if got.ToolCallID != "call_1" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "call_1")
	}
	if got.Name != "echo" {
		t.Errorf("Name = %q, want %q", got.Name, "echo")
	}
	if got.Content != "hello world" {
		t.Errorf("Content = %q, want %q", got.Content, "hello world")
	}
}

// TestDispatchToolEchoEmptyText verifies that an empty "text" argument is
// treated as a valid call (not an error) — empty string is a legitimate echo.
func TestDispatchToolEchoEmptyText(t *testing.T) {
	tc := ToolCall{ID: "call_2", Name: "echo", Arguments: `{"text":""}`}
	got := DispatchTool(context.Background(), tc)

	if got.IsError {
		t.Errorf("IsError = true for empty text, want false")
	}
	if got.Content != "" {
		t.Errorf("Content = %q, want empty string", got.Content)
	}
}

// TestDispatchToolEchoBadArgs covers the branch where Arguments is not valid
// JSON. The result must have IsError=true and a non-empty Content that
// mentions the parse failure — so the model can read the error and retry.
func TestDispatchToolEchoBadArgs(t *testing.T) {
	tc := ToolCall{ID: "call_3", Name: "echo", Arguments: `not-json`}
	got := DispatchTool(context.Background(), tc)

	if !got.IsError {
		t.Errorf("IsError = false, want true for invalid JSON arguments")
	}
	if got.ToolCallID != "call_3" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "call_3")
	}
	if got.Name != "echo" {
		t.Errorf("Name = %q, want %q", got.Name, "echo")
	}
	if !strings.Contains(got.Content, "error") {
		t.Errorf("Content %q should contain 'error'", got.Content)
	}
}

// TestDispatchToolUnknown covers the default switch branch: any tool name not
// registered returns IsError=true and mentions the tool name in Content.
func TestDispatchToolUnknown(t *testing.T) {
	tc := ToolCall{ID: "call_4", Name: "nonexistent_tool", Arguments: `{}`}
	got := DispatchTool(context.Background(), tc)

	if !got.IsError {
		t.Errorf("IsError = false, want true for unknown tool")
	}
	if got.ToolCallID != "call_4" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "call_4")
	}
	if !strings.Contains(got.Content, "nonexistent_tool") {
		t.Errorf("Content %q should mention the unknown tool name", got.Content)
	}
}

// TestDispatchToolPreservesToolCallID verifies that DispatchTool always copies
// tc.ID into the result, even for error paths — the conversation loop needs the
// ID to pair the result with the original tool_call in history.
func TestDispatchToolPreservesToolCallID(t *testing.T) {
	cases := []struct {
		name string
		tc   ToolCall
	}{
		{"echo ok", ToolCall{ID: "id_ok", Name: "echo", Arguments: `{"text":"x"}`}},
		{"echo bad args", ToolCall{ID: "id_bad", Name: "echo", Arguments: `bad`}},
		{"unknown", ToolCall{ID: "id_unk", Name: "mystery", Arguments: `{}`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DispatchTool(context.Background(), c.tc)
			if got.ToolCallID != c.tc.ID {
				t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, c.tc.ID)
			}
		})
	}
}
