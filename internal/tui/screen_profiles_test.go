package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

func TestProfileScreen_EnterSelects(t *testing.T) {
	s := newProfileScreen([]profiles.Profile{
		{Name: "alpha-sso", IsSSO: true, Region: "sa-east-1"},
		{Name: "beta"},
	})
	// list needs a size to render/select
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil {
		t.Fatal("want a selected profile, got nil")
	}
	if sel.Name != "alpha-sso" {
		t.Fatalf("selected %q, want alpha-sso", sel.Name)
	}
}
