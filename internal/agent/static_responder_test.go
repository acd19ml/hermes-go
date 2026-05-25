package agent

import "testing"

func TestStaticResponderReturnsAssistantRole(t *testing.T) {
	r := StaticResponder{}
	got, err := r.Respond(Message{Role: RoleUser, Content: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", got.Role, RoleAssistant)
	}
}

func TestStaticResponderEchoesContent(t *testing.T) {
	r := StaticResponder{}
	got, err := r.Respond(Message{Role: RoleUser, Content: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "hermes-go (static): hi"
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}

func TestStaticResponderNeverErrors(t *testing.T) {
	r := StaticResponder{}
	_, err := r.Respond(Message{Role: RoleUser, Content: "anything"})
	if err != nil {
		t.Errorf("error = %v, want nil", err)
	}
}

func TestStaticResponderEmptyMessage(t *testing.T) {
	r := StaticResponder{}
	got, err := r.Respond(Message{Role: RoleUser, Content: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "hermes-go (static): "
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}
