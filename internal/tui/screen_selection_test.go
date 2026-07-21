package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

func TestSelectionScreen_SelectsSSOSessionFirst(t *testing.T) {
	s := newSelectionScreen(
		[]profiles.Profile{{Name: "prod"}},
		[]profiles.SSOSession{{Name: "vakinha", StartURL: "https://x/start", Region: "us-east-1"}},
	)
	s, _, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, prof, sess, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sess == nil {
		t.Fatalf("want sso-session selected first, got prof=%v sess=%v", prof, sess)
	}
	if prof != nil {
		t.Fatalf("profile should be nil when a session is selected")
	}
	if sess.Name != "vakinha" {
		t.Fatalf("session = %q, want vakinha", sess.Name)
	}
}

func TestSelectionScreen_SelectsProfile(t *testing.T) {
	s := newSelectionScreen(
		[]profiles.Profile{{Name: "prod"}},
		nil,
	)
	s, _, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, prof, sess, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if prof == nil || prof.Name != "prod" {
		t.Fatalf("want profile prod, got prof=%v sess=%v", prof, sess)
	}
	if sess != nil {
		t.Fatalf("session should be nil when a profile is selected")
	}
}
