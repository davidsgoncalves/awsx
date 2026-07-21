package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func TestAccountScreen_EnterSelects(t *testing.T) {
	s := newAccountScreen([]awsx.Account{
		{ID: "111111111111", Name: "prod"},
		{ID: "222222222222", Name: "staging"},
	})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || sel.ID != "111111111111" {
		t.Fatalf("want first account, got %+v", sel)
	}
}

func TestRoleScreen_EnterSelects(t *testing.T) {
	s := newRoleScreen([]awsx.Role{{Name: "SystemAdministrator"}, {Name: "ReadOnly"}})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || sel.Name != "SystemAdministrator" {
		t.Fatalf("want first role, got %+v", sel)
	}
}

func TestRegionScreen_EnterSelects(t *testing.T) {
	s := newRegionScreen([]string{"us-east-1", "sa-east-1"})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || *sel != "us-east-1" {
		t.Fatalf("want us-east-1, got %v", sel)
	}
}
