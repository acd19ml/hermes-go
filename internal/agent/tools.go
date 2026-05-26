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

// echoToolSpecs returns the OpenAI wire schemas for the echo tool.
//
// Still used by OpenAIChatClient.tools until Phase 3 c5 replaces the field
// with enabledToolsets + globalRegistry.GetSchemas().  Kept here alongside
// echoEntry() to keep schema and handler co-located; c5 will delete it.
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

// DispatchTool delegates to globalRegistry.Dispatch.
//
// The switch introduced in Phase 2 has been replaced now that two production
// tools exist and the Registry abstraction is justified.  All tool routing
// happens inside Registry.Dispatch; adding a new tool only requires a new
// ToolEntry registered in init() — DispatchTool never needs to change again.
func DispatchTool(ctx context.Context, tc ToolCall) ToolResult {
	return globalRegistry.Dispatch(ctx, tc)
}

// ── echo tool ─────────────────────────────────────────────────────────────────

// echoEntry returns the ToolEntry for the echo tool, bundling its schema and
// handler into one value so they are always in sync.
func echoEntry() ToolEntry {
	params := json.RawMessage(`{` +
		`"type":"object",` +
		`"properties":{"text":{"type":"string","description":"The text to echo back"}},` +
		`"required":["text"]` +
		`}`)
	return ToolEntry{
		Name:        "echo",
		Description: "Return the input text unchanged. Useful for verifying tool dispatch.",
		Parameters:  params,
		Handler: func(_ context.Context, tc ToolCall) ToolResult {
			return echoTool(tc)
		},
	}
}

// echoTool implements the echo tool: parses the "text" argument from tc and
// returns it unchanged.  Arguments must be a JSON object {"text":"<string>"}.
func echoTool(tc ToolCall) ToolResult {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
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

// readFileEntry returns the ToolEntry for the read_file tool, bundling its
// schema and handler.  The handler closure calls getCwd() at invocation time
// (not at registration time) so the working directory is always current.
func readFileEntry() ToolEntry {
	params := json.RawMessage(`{` +
		`"type":"object",` +
		`"properties":{"path":{"type":"string","description":"Relative path to the file (within the current working directory)"}},` +
		`"required":["path"]` +
		`}`)
	return ToolEntry{
		Name:        "read_file",
		Description: "Read the contents of a file at the given path (relative to the current working directory). Returns the file contents as a string.",
		Parameters:  params,
		Handler: func(_ context.Context, tc ToolCall) ToolResult {
			return readFileTool(tc, getCwd())
		},
	}
}

// readFileTool implements the read_file tool.
//
// cwd is injected so the function is testable without touching os.Getwd;
// production callers pass getCwd() via the readFileEntry handler closure.
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
