package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// getCwd returns the process working directory, falling back to "." on error.
// DispatchTool calls this so tool handlers always receive a usable cwd string.
func getCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// echoToolSpecs returns the OpenAI wire schema for the echo tool.
//
// The returned slice is assigned to OpenAIChatClient.tools so that every
// request includes the echo tool definition.  Phase 3 will replace this
// hardcoded slice with Registry.GetSchemas() once a second tool exists and
// the registry abstraction is justified.
//
// Parameters is a json.RawMessage so the JSON Schema object is embedded
// verbatim — we never need to parse its contents, only forward it to the API.
func echoToolSpecs() []openAIToolSpec {
	params := json.RawMessage(`{` +
		`"type":"object",` +
		`"properties":{"text":{"type":"string","description":"The text to echo back"}},` +
		`"required":["text"]` +
		`}`)
	return []openAIToolSpec{{
		Type: "function",
		Function: openAIToolSpecBody{
			Name:        "echo",
			Description: "Return the input text unchanged. Useful for verifying tool dispatch.",
			Parameters:  params,
		},
	}}
}

// DispatchTool executes the tool identified by tc.Name and returns the result.
//
// Phase 3 c1 adds "read_file" alongside "echo"; both are hardcoded in the
// switch.  Phase 3 c2 will replace this switch with Registry.Dispatch once
// the Registry abstraction is introduced (two production implementations
// satisfy the "don't abstract prematurely" rule).
//
// ctx is accepted for forward-compatibility: future tools (web fetch,
// terminal) will need it for timeout / cancellation.
//
// Errors from tool execution (bad arguments, unknown tool) are returned as a
// ToolResult with IsError=true and a JSON-encoded error in Content — NOT as a
// Go error.  The tool result must still enter the conversation history so the
// model can observe and react to the failure.
func DispatchTool(_ context.Context, tc ToolCall) ToolResult {
	switch tc.Name {
	case "echo":
		return echoTool(tc)
	case "read_file":
		return readFileTool(tc, getCwd())
	default:
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       tc.Name,
			Content:    fmt.Sprintf(`{"error":"unknown tool %q"}`, tc.Name),
			IsError:    true,
		}
	}
}

// echoTool implements the echo tool: parses the "text" argument from tc and
// returns it unchanged.  Arguments must be a JSON object {"text":"<string>"}.
func echoTool(tc ToolCall) ToolResult {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		// Bad JSON — return a structured error result so the model can see
		// what went wrong and (optionally) retry with corrected arguments.
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "echo",
			Content:    fmt.Sprintf(`{"error":"echo: bad arguments: %v"}`, err),
			IsError:    true,
		}
	}
	return ToolResult{
		ToolCallID: tc.ID,
		Name:       "echo", // use literal, not tc.Name, to guard against case drift
		Content:    args.Text,
	}
}

// ── read_file tool ────────────────────────────────────────────────────────────

// readFileToolSpec returns the OpenAI wire schema for the read_file tool.
// The tool accepts a single "path" argument (a relative path within cwd).
func readFileToolSpec() openAIToolSpec {
	params := json.RawMessage(`{` +
		`"type":"object",` +
		`"properties":{"path":{"type":"string","description":"Relative path to the file (within the current working directory)"}},` +
		`"required":["path"]` +
		`}`)
	return openAIToolSpec{
		Type: "function",
		Function: openAIToolSpecBody{
			Name:        "read_file",
			Description: "Read the contents of a file at the given path (relative to the current working directory). Returns the file contents as a string.",
			Parameters:  params,
		},
	}
}

// readFileTool implements the read_file tool.
//
// cwd is injected so the function is testable without touching os.Getwd;
// production callers pass getCwd().
//
// Path safety: the resolved absolute path must remain inside cwd.
// Any path that escapes (e.g. "../secret") is rejected with IsError=true.
//
// Arguments must be a JSON object {"path":"<relative-path>"}.
func readFileTool(tc ToolCall, cwd string) ToolResult {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    fmt.Sprintf(`{"error":"read_file: bad arguments: %v"}`, err),
			IsError:    true,
		}
	}
	if args.Path == "" {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    `{"error":"read_file: path must not be empty"}`,
			IsError:    true,
		}
	}

	// Resolve to absolute path and verify it stays within cwd.
	abs, err := filepath.Abs(filepath.Join(cwd, args.Path))
	if err != nil {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    fmt.Sprintf(`{"error":"read_file: cannot resolve path: %v"}`, err),
			IsError:    true,
		}
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    `{"error":"read_file: path escapes working directory"}`,
			IsError:    true,
		}
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    fmt.Sprintf(`{"error":"read_file: %v"}`, err),
			IsError:    true,
		}
	}
	return ToolResult{
		ToolCallID: tc.ID,
		Name:       "read_file",
		Content:    string(data),
	}
}
