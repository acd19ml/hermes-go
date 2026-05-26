package agent

import (
	"context"
	"os"
	"path/filepath"
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

// ── read_file tool ────────────────────────────────────────────────────────────

// TestReadFileToolSuccess creates a temporary file and verifies that
// readFileTool returns its contents correctly when the path is within cwd.
func TestReadFileToolSuccess(t *testing.T) {
	dir := t.TempDir()
	content := "hello from read_file\n"
	fpath := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: write temp file: %v", err)
	}

	tc := ToolCall{ID: "rf_ok", Name: "read_file", Arguments: `{"path":"target.txt"}`}
	got := readFileTool(tc, dir)

	if got.IsError {
		t.Fatalf("IsError = true, want false; content: %s", got.Content)
	}
	if got.ToolCallID != "rf_ok" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "rf_ok")
	}
	if got.Name != "read_file" {
		t.Errorf("Name = %q, want %q", got.Name, "read_file")
	}
	if got.Content != content {
		t.Errorf("Content = %q, want %q", got.Content, content)
	}
}

// TestReadFileToolPathEscape verifies that paths escaping the cwd (e.g. "../secret")
// are rejected with IsError=true.
func TestReadFileToolPathEscape(t *testing.T) {
	dir := t.TempDir()
	cases := []string{"../secret", "../../etc/passwd", "subdir/../../outside"}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			tc := ToolCall{ID: "rf_esc", Name: "read_file", Arguments: `{"path":"` + p + `"}`}
			got := readFileTool(tc, dir)
			if !got.IsError {
				t.Errorf("path %q: IsError = false, want true (path escapes cwd)", p)
			}
			if !strings.Contains(got.Content, "error") {
				t.Errorf("path %q: Content %q should contain 'error'", p, got.Content)
			}
		})
	}
}

// TestReadFileToolBadArgs verifies that non-JSON arguments return IsError=true
// with an error message the model can read.
func TestReadFileToolBadArgs(t *testing.T) {
	dir := t.TempDir()
	tc := ToolCall{ID: "rf_bad", Name: "read_file", Arguments: `not-json`}
	got := readFileTool(tc, dir)

	if !got.IsError {
		t.Errorf("IsError = false, want true for invalid JSON")
	}
	if got.ToolCallID != "rf_bad" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "rf_bad")
	}
	if !strings.Contains(got.Content, "error") {
		t.Errorf("Content %q should contain 'error'", got.Content)
	}
}

// TestReadFileToolMissing verifies that a non-existent file returns IsError=true.
func TestReadFileToolMissing(t *testing.T) {
	dir := t.TempDir()
	tc := ToolCall{ID: "rf_miss", Name: "read_file", Arguments: `{"path":"does_not_exist.txt"}`}
	got := readFileTool(tc, dir)

	if !got.IsError {
		t.Errorf("IsError = false, want true for missing file")
	}
	if got.ToolCallID != "rf_miss" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "rf_miss")
	}
	if !strings.Contains(got.Content, "error") {
		t.Errorf("Content %q should contain 'error'", got.Content)
	}
}

// TestReadFileToolEmptyPath verifies that an empty path is rejected with IsError=true.
func TestReadFileToolEmptyPath(t *testing.T) {
	dir := t.TempDir()
	tc := ToolCall{ID: "rf_empty", Name: "read_file", Arguments: `{"path":""}`}
	got := readFileTool(tc, dir)

	if !got.IsError {
		t.Errorf("IsError = false, want true for empty path")
	}
}

// TestDispatchToolReadFile verifies that DispatchTool routes "read_file" to
// the correct handler (integration with the switch).
func TestDispatchToolReadFile(t *testing.T) {
	// Write a file in a temp dir and temporarily change working directory.
	// getCwd() is called inside DispatchTool; we point os.Chdir there for the test.
	dir := t.TempDir()
	content := "dispatched!\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.txt"), []byte(content), 0o644); err != nil {
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

	tc := ToolCall{ID: "rf_dispatch", Name: "read_file", Arguments: `{"path":"probe.txt"}`}
	got := DispatchTool(context.Background(), tc)

	if got.IsError {
		t.Fatalf("IsError = true, want false; content: %s", got.Content)
	}
	if got.Content != content {
		t.Errorf("Content = %q, want %q", got.Content, content)
	}
}
