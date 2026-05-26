package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

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
// The switch is intentionally hardcoded for Phase 2: only one tool exists, so
// the "two production implementations before interface" rule is not yet
// satisfied.  Phase 3 will introduce a second tool and replace this switch
// with Registry.Dispatch.
//
// ctx is accepted for forward-compatibility: future tools (web fetch,
// terminal) will need it for timeout / cancellation.  The echo tool ignores
// it.
//
// Errors from tool execution (bad arguments, unknown tool) are returned as a
// ToolResult with IsError=true and a JSON-encoded error in Content — NOT as a
// Go error.  The tool result must still enter the conversation history so the
// model can observe and react to the failure.
func DispatchTool(_ context.Context, tc ToolCall) ToolResult {
	switch tc.Name {
	case "echo":
		return echoTool(tc)
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
