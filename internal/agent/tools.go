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
func getCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// toolError returns a JSON-encoded error body with a human message and a
// machine-readable code.  All tool error Content fields use this format so the
// model can parse both fields and decide how to respond.
//
// Format: {"error":"<msg>","code":"<code>"}
// Codes:  unknown_tool | bad_arguments | path_escape | execution_error
func toolError(code, msg string) string {
	return fmt.Sprintf(`{"error":%s,"code":%s}`,
		jsonString(msg), jsonString(code))
}

// jsonString returns a JSON-encoded string literal for s, using
// json.Marshal to handle escaping correctly.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// echoToolSpecs returns the OpenAI wire schemas for the echo tool.
// Still used by OpenAIChatClient.tools until Phase 3 c5 replaces it.
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
func DispatchTool(ctx context.Context, tc ToolCall) ToolResult {
	return globalRegistry.Dispatch(ctx, tc)
}

// ── echo tool ─────────────────────────────────────────────────────────────────

func echoEntry() ToolEntry {
	params := json.RawMessage(`{` +
		`"type":"object",` +
		`"properties":{"text":{"type":"string","description":"The text to echo back"}},` +
		`"required":["text"]` +
		`}`)
	return ToolEntry{
		Name:        "echo",
		Toolset:     "core",
		Description: "Return the input text unchanged. Useful for verifying tool dispatch.",
		Parameters:  params,
		Handler: func(_ context.Context, tc ToolCall) ToolResult {
			return echoTool(tc)
		},
	}
}

func echoTool(tc ToolCall) ToolResult {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "echo",
			Content:    toolError("bad_arguments", "echo: bad arguments: "+err.Error()),
			IsError:    true,
		}
	}
	return ToolResult{
		ToolCallID: tc.ID,
		Name:       "echo",
		Content:    args.Text,
	}
}

// ── read_file tool ────────────────────────────────────────────────────────────

func readFileEntry() ToolEntry {
	params := json.RawMessage(`{` +
		`"type":"object",` +
		`"properties":{"path":{"type":"string","description":"Relative path to the file (within the current working directory)"}},` +
		`"required":["path"]` +
		`}`)
	return ToolEntry{
		Name:        "read_file",
		Toolset:     "file",
		Description: "Read the contents of a file at the given path (relative to the current working directory). Returns the file contents as a string.",
		Parameters:  params,
		Handler: func(_ context.Context, tc ToolCall) ToolResult {
			return readFileTool(tc, getCwd())
		},
	}
}

func readFileTool(tc ToolCall, cwd string) ToolResult {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    toolError("bad_arguments", "read_file: bad arguments: "+err.Error()),
			IsError:    true,
		}
	}
	if args.Path == "" {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    toolError("bad_arguments", "read_file: path must not be empty"),
			IsError:    true,
		}
	}

	abs, err := filepath.Abs(filepath.Join(cwd, args.Path))
	if err != nil {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    toolError("execution_error", "read_file: cannot resolve path: "+err.Error()),
			IsError:    true,
		}
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    toolError("path_escape", "read_file: path escapes working directory"),
			IsError:    true,
		}
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       "read_file",
			Content:    toolError("execution_error", "read_file: "+err.Error()),
			IsError:    true,
		}
	}
	return ToolResult{
		ToolCallID: tc.ID,
		Name:       "read_file",
		Content:    string(data),
	}
}
