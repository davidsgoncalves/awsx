package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func ec2ActionFixture() ec2ActionScreen {
	return newEC2ActionScreen(awsx.Target{
		Instance: awsx.Instance{ID: "i-1", Name: "api", PrivateIP: "10.0.1.15"}, SSMOnline: true,
	})
}

func TestEC2ActionScreen_OffersEachAction(t *testing.T) {
	s := ec2ActionFixture()
	for i, want := range []ec2Action{ec2ActionSession, ec2ActionExec, ec2ActionTunnel} {
		if i > 0 {
			s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
		}
		if _, got := s.Update(tea.KeyMsg{Type: tea.KeyEnter}); got != want {
			t.Fatalf("action %d = %v, want %v", i, got, want)
		}
	}
}

func TestEC2ActionScreen_CursorStopsAtTheLastAction(t *testing.T) {
	s := ec2ActionFixture()
	for range 10 {
		s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if _, got := s.Update(tea.KeyMsg{Type: tea.KeyEnter}); got != ec2ActionTunnel {
		t.Fatalf("action = %v, want ec2ActionTunnel", got)
	}
}

func TestEC2ActionScreen_ViewNamesTheInstance(t *testing.T) {
	v := ec2ActionFixture().View()
	for _, want := range []string{"api", "i-1", "10.0.1.15", "Túnel para banco RDS"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view missing %q: %q", want, v)
		}
	}
}
