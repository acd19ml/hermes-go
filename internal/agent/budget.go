package agent

import "fmt"

// IterationBudget tracks how many LLM calls an AIAgent is allowed to make.
//
// Max is the ceiling; used counts consumed iterations (unexported).
// Phase 1 always constructs with Max=1 (single-turn); Phase 2 will pass a
// larger value to RunConversation.
//
// Zero-value Max=0 means no calls allowed — callers must set Max≥1.
// The zero value of IterationBudget (Max=0, used=0) will reject every Consume.
type IterationBudget struct {
	Max  int
	used int
}

// Remaining returns the number of iterations left (Max − used), floored at 0.
func (b *IterationBudget) Remaining() int {
	r := b.Max - b.used
	if r < 0 {
		return 0
	}
	return r
}

// Consume records one iteration use. Returns an error if the budget is already
// exhausted, leaving used unchanged so callers can still inspect the state.
func (b *IterationBudget) Consume() error {
	if b.used >= b.Max {
		return fmt.Errorf("iteration budget exhausted (max=%d)", b.Max)
	}
	b.used++
	return nil
}
