package agent

// Role constants for Message.Role.
// Use these instead of bare strings to prevent typos and enable grep.
const (
	RoleSystem    = "system"    // OpenAI-style system prompt; first message in the list
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall represents a single tool invocation requested by the model.
//
// Internal format follows OpenAI Chat Completions style (matching Python
// Hermes's internal representation): Arguments is a JSON-encoded string,
// e.g. `{"location":"SF"}`. Callers use json.Unmarshal([]byte(tc.Arguments))
// to parse it. Name and ID are flattened from OpenAI's nested
// function:{name,arguments} — the Transport layer (Phase 6) re-wraps them
// for the wire.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string, e.g. `{"key":"val"}`
}

// ToolResult carries the output of a tool execution back to the model.
//
// Name is the tool's registered name; required by OpenAI wire format and
// used for session-DB logging (mirrors Python's make_tool_result_message
// which always includes "name").
//
// IsError distinguishes error results from successful ones. It is omitted
// from JSON when false (OpenAI does not use this field); the Anthropic
// Transport reads it when building tool_result blocks.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// Message is the shared envelope for all conversation turns.
//
// Roles:
//   - RoleUser      — human input or injected tool results (role:"tool")
//   - RoleAssistant — model response, may carry ToolCalls
//   - RoleTool      — tool execution result; ToolCallID pairs it with
//     the corresponding ToolCall in the preceding assistant message
//
// Content and ToolCalls are mutually exclusive in practice but both carry
// omitempty so zero values are invisible in JSON. ToolCallID is only
// meaningful on role:"tool" messages.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
