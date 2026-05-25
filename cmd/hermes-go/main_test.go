package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Existing tests (unchanged) ────────────────────────────────────────────────

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "hermes-go v0.0.1\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunWithoutArgsPrintsVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "hermes-go v0.0.1\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunUnknownFlagReturnsError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--unknown"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run returned 0, want non-zero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q, want unknown flag error", stderr.String())
	}
}

// ── --msg error path: missing API key ────────────────────────────────────────

func TestRunMsgNoAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--msg", "hi"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run returned 0, want non-zero when API key is missing")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on error", stdout.String())
	}
	if !strings.Contains(stderr.String(), "OPENAI_API_KEY") {
		t.Errorf("stderr = %q, should mention OPENAI_API_KEY", stderr.String())
	}
}

func TestRunMsgAndUnknownFlag(t *testing.T) {
	// Failure path: unknown flag alongside --msg must reject with non-zero exit.
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--msg", "hi", "--unknown"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run returned 0, want non-zero")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on flag error", stdout.String())
	}
}

// ── --msg success path via httptest ─────────────────────────────────────────

// fakeOpenAIReply creates an httptest server that always responds with the
// given content string as an OpenAI Chat Completions reply.
func fakeOpenAIReply(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"role": "assistant", "content": content},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
}

func TestRunMsgBasic(t *testing.T) {
	srv := fakeOpenAIReply(t, "hello from model")
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--msg", "hi"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "hello from model\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunMsgEmpty(t *testing.T) {
	srv := fakeOpenAIReply(t, "empty reply")
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// --msg "" is an explicitly empty message; must not panic.
	code := run([]string{"--msg", ""}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "empty reply\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
