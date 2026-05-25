package main

import (
	"bytes"
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

// ── New --msg tests ───────────────────────────────────────────────────────────

func TestRunMsgBasic(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--msg", "hi"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "hermes-go (static): hi\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunMsgEmpty(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// --msg "" is an explicitly empty message; must not panic.
	code := run([]string{"--msg", ""}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "hermes-go (static): \n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
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
