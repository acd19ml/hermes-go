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

		// Remove orphan tool results before every API call.
		// In normal flow orphans cannot appear (DispatchTool always preserves
		// the tool_call_id), but future features (session replay, memory
		// injection) may introduce stale history — clean defensively.
		msgs = dropOrphanToolResults(msgs)

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

// ── history integrity helpers ─────────────────────────────────────────────────

// dropOrphanToolResults returns a new slice with all orphan tool-result
// messages removed.  A tool-result message (Role==RoleTool) is an orphan when
// its ToolCallID does not appear in any *preceding* assistant message's
// ToolCalls list.
//
// The function processes messages in document order: assistant messages
// accumulate their tool_call IDs into a running set; each subsequent
// RoleTool message is kept only if its ToolCallID is in that set.
//
// Non-tool messages (system, user, assistant) are always kept.
// A RoleTool message with an empty ToolCallID is also dropped (malformed).
func dropOrphanToolResults(msgs []Message) []Message {
	validIDs := make(map[string]struct{}, 8)
	out := make([]Message, 0, len(msgs))

	for _, m := range msgs {
		if m.Role == RoleAssistant {
			for _, tc := range m.ToolCalls {
				validIDs[tc.ID] = struct{}{}
			}
		}
		if m.Role == RoleTool {
			if _, ok := validIDs[m.ToolCallID]; !ok {
				continue // orphan (includes empty ToolCallID — malformed)
			}
		}
		out = append(out, m)
	}
	return out
}

// validateToolPairing returns an error if any tool-result message in msgs has
// a ToolCallID not present in any preceding assistant message's ToolCalls.
// It mirrors the logic of dropOrphanToolResults but reports rather than
// silently removes, making it useful for assertions and debugging.
func validateToolPairing(msgs []Message) error {
	validIDs := make(map[string]struct{}, 8)

	for _, m := range msgs {
		if m.Role == RoleAssistant {
			for _, tc := range m.ToolCalls {
				validIDs[tc.ID] = struct{}{}
			}
		}
		if m.Role == RoleTool {
			if _, ok := validIDs[m.ToolCallID]; !ok {
				return fmt.Errorf(
					"orphan tool result: tool_call_id %q has no matching preceding assistant tool_call",
					m.ToolCallID,
				)
			}
		}
	}
	return nil
}
