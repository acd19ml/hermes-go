package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// errorCode parses the "code" field from a toolError JSON body.
// Returns "" if content is not valid JSON or the field is absent.
func errorCode(content string) string {
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(content), &body)
	return body.Code
}

// ── Registry.Register ─────────────────────────────────────────────────────────

// TestRegistryDuplicatePanic verifies that registering the same tool name
// twice panics immediately, surfacing the programming error at startup.
func TestRegistryDuplicatePanic(t *testing.T) {
	r := newRegistry()
	entry := ToolEntry{Name: "dup", Handler: func(_ context.Context, tc ToolCall) ToolResult {
		return ToolResult{ToolCallID: tc.ID, Name: tc.Name, Content: "ok"}
	}}
	r.Register(entry)

	defer func() {
		if rec := recover(); rec == nil {
			t.Error("expected panic on duplicate Register, got none")
		}
	}()
	r.Register(entry) // should panic
}

// ── Registry.Dispatch ─────────────────────────────────────────────────────────

// TestRegistryDispatchEcho verifies that globalRegistry routes "echo" to the
// correct handler and returns the echoed text.
func TestRegistryDispatchEcho(t *testing.T) {
	tc := ToolCall{ID: "reg_echo", Name: "echo", Arguments: `{"text":"registry test"}`}
	got := globalRegistry.Dispatch(context.Background(), tc)

	if got.IsError {
		t.Fatalf("IsError = true; content: %s", got.Content)
	}
	if got.Content != "registry test" {
		t.Errorf("Content = %q, want %q", got.Content, "registry test")
	}
	if got.ToolCallID != "reg_echo" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "reg_echo")
	}
}

// TestRegistryDispatchReadFile verifies that globalRegistry routes "read_file"
// to the correct handler.  Uses os.Chdir so getCwd() returns the temp dir.
func TestRegistryDispatchReadFile(t *testing.T) {
	dir := t.TempDir()
	content := "registry read_file test\n"
	if err := os.WriteFile(filepath.Join(dir, "reg_probe.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	tc := ToolCall{ID: "reg_rf", Name: "read_file", Arguments: `{"path":"reg_probe.txt"}`}
	got := globalRegistry.Dispatch(context.Background(), tc)

	if got.IsError {
		t.Fatalf("IsError = true; content: %s", got.Content)
	}
	if got.Content != content {
		t.Errorf("Content = %q, want %q", got.Content, content)
	}
}

// TestRegistryDispatchUnknown verifies that dispatching an unregistered tool
// returns IsError=true with the tool name in the error body.
func TestRegistryDispatchUnknown(t *testing.T) {
	tc := ToolCall{ID: "reg_unk", Name: "no_such_tool", Arguments: `{}`}
	got := globalRegistry.Dispatch(context.Background(), tc)

	if !got.IsError {
		t.Errorf("IsError = false, want true for unknown tool")
	}
	if c := errorCode(got.Content); c != "unknown_tool" {
		t.Errorf("code = %q, want %q; content: %s", c, "unknown_tool", got.Content)
	}
	if got.ToolCallID != "reg_unk" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "reg_unk")
	}
}

// ── Registry.GetSchemas ───────────────────────────────────────────────────────

// TestGetSchemasEmpty verifies that an empty enabledToolsets slice returns nil.
func TestGetSchemasEmpty(t *testing.T) {
	r := newRegistry()
	r.Register(ToolEntry{Name: "t1", Toolset: "core", Parameters: json.RawMessage(`{}`),
		Handler: func(_ context.Context, tc ToolCall) ToolResult { return ToolResult{} }})

	got := r.GetSchemas(nil)
	if got != nil {
		t.Errorf("GetSchemas(nil) = %v, want nil", got)
	}
	got = r.GetSchemas([]string{})
	if got != nil {
		t.Errorf("GetSchemas([]) = %v, want nil", got)
	}
}

// TestGetSchemasFiltersToolset verifies that GetSchemas returns only the tools
// belonging to the requested toolset.
func TestGetSchemasFiltersToolset(t *testing.T) {
	r := newRegistry()
	r.Register(ToolEntry{Name: "alpha", Toolset: "core", Description: "a", Parameters: json.RawMessage(`{}`),
		Handler: func(_ context.Context, tc ToolCall) ToolResult { return ToolResult{} }})
	r.Register(ToolEntry{Name: "beta", Toolset: "file", Description: "b", Parameters: json.RawMessage(`{}`),
		Handler: func(_ context.Context, tc ToolCall) ToolResult { return ToolResult{} }})

	// Only "core" requested — should get alpha only.
	got := r.GetSchemas([]string{"core"})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; schemas: %v", len(got), got)
	}
	if got[0].Function.Name != "alpha" {
		t.Errorf("Function.Name = %q, want %q", got[0].Function.Name, "alpha")
	}

	// Only "file" requested — should get beta only.
	got = r.GetSchemas([]string{"file"})
	if len(got) != 1 || got[0].Function.Name != "beta" {
		t.Errorf("GetSchemas([file]) = %v, want [beta]", got)
	}

	// Both requested — should get both, sorted.
	got = r.GetSchemas([]string{"core", "file"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Function.Name != "alpha" || got[1].Function.Name != "beta" {
		t.Errorf("names = [%q, %q], want [alpha, beta]", got[0].Function.Name, got[1].Function.Name)
	}
}

// TestGetSchemasDebuggingAlias verifies that the "debugging" toolset alias
// expands to both "core" and "file".
func TestGetSchemasDebuggingAlias(t *testing.T) {
	r := newRegistry()
	r.Register(ToolEntry{Name: "e", Toolset: "core", Parameters: json.RawMessage(`{}`),
		Handler: func(_ context.Context, tc ToolCall) ToolResult { return ToolResult{} }})
	r.Register(ToolEntry{Name: "r", Toolset: "file", Parameters: json.RawMessage(`{}`),
		Handler: func(_ context.Context, tc ToolCall) ToolResult { return ToolResult{} }})

	got := r.GetSchemas([]string{"debugging"})
	if len(got) != 2 {
		t.Errorf("debugging alias: len = %d, want 2; schemas: %v", len(got), got)
	}
}

// TestGetSchemasStableOrder verifies that GetSchemas returns tools sorted by
// Name regardless of registration order.
func TestGetSchemasStableOrder(t *testing.T) {
	r := newRegistry()
	for _, name := range []string{"zebra", "apple", "mango"} {
		n := name
		r.Register(ToolEntry{Name: n, Toolset: "core", Parameters: json.RawMessage(`{}`),
			Handler: func(_ context.Context, tc ToolCall) ToolResult { return ToolResult{} }})
	}
	got := r.GetSchemas([]string{"core"})
	want := []string{"apple", "mango", "zebra"}
	for i, w := range want {
		if got[i].Function.Name != w {
			t.Errorf("index %d: Name = %q, want %q", i, got[i].Function.Name, w)
		}
	}
}

// ── globalRegistry toolset wiring ─────────────────────────────────────────────
// These tests verify that production tool entries have the correct Toolset
// values so GetSchemas routes them to the right groups.

// TestGlobalRegistryCoreToolset verifies that GetSchemas(["core"]) returns
// only the echo tool from globalRegistry.
func TestGlobalRegistryCoreToolset(t *testing.T) {
	got := globalRegistry.GetSchemas([]string{"core"})
	if len(got) != 1 {
		t.Fatalf("core toolset: len = %d, want 1; schemas: %v", len(got), got)
	}
	if got[0].Function.Name != "echo" {
		t.Errorf("core toolset: Name = %q, want %q", got[0].Function.Name, "echo")
	}
}

// TestGlobalRegistryFileToolset verifies that GetSchemas(["file"]) returns
// only the read_file tool from globalRegistry.
func TestGlobalRegistryFileToolset(t *testing.T) {
	got := globalRegistry.GetSchemas([]string{"file"})
	if len(got) != 1 {
		t.Fatalf("file toolset: len = %d, want 1; schemas: %v", len(got), got)
	}
	if got[0].Function.Name != "read_file" {
		t.Errorf("file toolset: Name = %q, want %q", got[0].Function.Name, "read_file")
	}
}

// TestGlobalRegistryDebuggingToolset verifies that GetSchemas(["debugging"])
// returns both echo and read_file (alias expands to core + file).
func TestGlobalRegistryDebuggingToolset(t *testing.T) {
	got := globalRegistry.GetSchemas([]string{"debugging"})
	if len(got) != 2 {
		t.Fatalf("debugging toolset: len = %d, want 2; schemas: %v", len(got), got)
	}
	// Results are sorted by name: echo < read_file.
	if got[0].Function.Name != "echo" || got[1].Function.Name != "read_file" {
		t.Errorf("debugging toolset: names = [%q, %q], want [echo, read_file]",
			got[0].Function.Name, got[1].Function.Name)
	}
}

// TestGlobalRegistryCoreAndFile verifies that GetSchemas(["core","file"])
// returns both tools, same as debugging.
func TestGlobalRegistryCoreAndFile(t *testing.T) {
	got := globalRegistry.GetSchemas([]string{"core", "file"})
	if len(got) != 2 {
		t.Fatalf("core+file: len = %d, want 2", len(got))
	}
	if got[0].Function.Name != "echo" || got[1].Function.Name != "read_file" {
		t.Errorf("core+file: names = [%q, %q], want [echo, read_file]",
			got[0].Function.Name, got[1].Function.Name)
	}
}
