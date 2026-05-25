package agent

import (
	"context"
	"fmt"
)

// RunConversation runs the agent loop until the model returns a message with
// no tool_calls, or the iteration budget is exhausted.
//
// A fresh IterationBudget{Max: maxIter} is created per call so RunConversation
// is independent of the single-turn a.budget used by RunOnce; the two methods
// can coexist on the same AIAgent.
//
// Loop (each iteration = one LLM call):
//  1. First iteration only: prepend system message + initial user turn.
//  2. Consume one budget unit; return error if exhausted before calling LLM.
//  3. Call client.Respond with the full message history.
//  4. Append assistant response to history.
//  5. If assistant has no ToolCalls → return final answer.
//  6. Dispatch each ToolCall via DispatchTool; append ToolResult messages.
//  7. Go to step 2.
//
// Tool errors (IsError=true) are included in history as normal ToolResult
// messages so the model can observe what went wrong and decide next steps.
func (a *AIAgent) RunConversation(ctx context.Context, userMsg string, maxIter int) (Message, error) {
	budget := IterationBudget{Max: maxIter}

	// Build the initial history: byte-static system prompt + first user turn.
	// pre-allocate with capacity for a few rounds to avoid repeated re-allocs.
	msgs := make([]Message, 0, 8)
	if a.systemPrompt != "" {
		msgs = append(msgs, Message{Role: RoleSystem, Content: a.systemPrompt})
	}
	msgs = append(msgs, Message{Role: RoleUser, Content: userMsg})

	for {
		// Enforce budget before calling the LLM so an exhausted budget never
		// results in an unexpected API call.
		if err := budget.Consume(); err != nil {
			return Message{}, fmt.Errorf("agent: %w", err)
		}

		resp, err := a.client.Respond(ctx, msgs)
		if err != nil {
			return Message{}, fmt.Errorf("agent: llm call failed: %w", err)
		}
		msgs = append(msgs, resp)

		// No tool calls → the model has produced its final text answer.
		if len(resp.ToolCalls) == 0 {
			return resp, nil
		}

		// Execute each requested tool and append results to history so the
		// next LLM call can observe what happened.
		for _, tc := range resp.ToolCalls {
			result := DispatchTool(ctx, tc)
			msgs = append(msgs, Message{
				Role:       RoleTool,
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
			})
		}
		// Continue to the next LLM call with the updated history.
	}
}
