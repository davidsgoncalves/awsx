package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func TestMenuScreen_SelectsEachAction(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"})

	_, next := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cursor 0 = EC2
	if next != screenInstances {
		t.Fatalf("next = %v, want screenInstances", next)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 1 = ECS
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenECSCluster {
		t.Fatalf("next = %v, want screenECSCluster", next)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 2 = Atualizar AWSX
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenUpdate {
		t.Fatalf("next = %v, want screenUpdate", next)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 3 = Sair
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenQuit {
		t.Fatalf("next = %v, want screenQuit", next)
	}
}

func TestMenuScreen_ViewGroupsByService(t *testing.T) {
	v := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"}).View()
	for _, want := range []string{"EC2", "ECS", "enter confirma"} {
		if !strings.Contains(v, want) {
			t.Fatalf("menu does not show %q: %q", want, v)
		}
	}
	if strings.Contains(v, "Rodar comando") {
		t.Fatalf("run-command belongs under a service, not the main menu: %q", v)
	}
}

func TestMenuScreen_CursorStopsAtLastAction(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"})
	for range 10 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, next := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
