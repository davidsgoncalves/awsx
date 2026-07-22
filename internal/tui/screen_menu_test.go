package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func TestMenuScreen_SelectEC2AndQuit(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"})
	_, next := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cursor 0 = Acessar EC2
	if next != screenInstances {
		t.Fatalf("next = %v, want screenInstances", next)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 1 = túnel
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenRDS {
		t.Fatalf("next = %v, want screenRDS", next)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 2 = Sair
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenQuit {
		t.Fatalf("next = %v, want screenQuit", next)
	}
}

func TestMenuScreen_ViewShowsHeader(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "947592431146"})
	v := m.View()
	if !strings.Contains(v, "prod") || !strings.Contains(v, "947592431146") || !strings.Contains(v, "us-east-1") {
		t.Fatalf("header missing fields: %q", v)
	}
}

func TestRoleFromArn(t *testing.T) {
	arn := "arn:aws:sts::123:assumed-role/AWSReservedSSO_SystemAdministrator_abc/david"
	if got := roleFromArn(arn); got != "AWSReservedSSO_SystemAdministrator_abc" {
		t.Fatalf("got %q", got)
	}
	if got := roleFromArn("garbage"); got != "" {
		t.Fatalf("want empty for unparseable, got %q", got)
	}
}
