package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func ecsActionTask() awsx.ECSTask {
	return awsx.ECSTask{
		Cluster: "vakinha-stg", TaskARN: "arn/abc", Service: "stg-api-web",
		Container: "api-web", InstanceID: "i-aaa", Status: "RUNNING", ExecEnabled: true,
	}
}

func TestECSActionScreen_EnterPicksTheFirstAction(t *testing.T) {
	s := newECSActionScreen(ecsActionTask())
	_, action := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != ecsActionCommand {
		t.Fatalf("action = %v, want ecsActionCommand", action)
	}
}

func TestECSActionScreen_OffersShellAndHost(t *testing.T) {
	s := newECSActionScreen(ecsActionTask())

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, action := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != ecsActionShell {
		t.Fatalf("action = %v, want ecsActionShell", action)
	}

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, action = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != ecsActionHost {
		t.Fatalf("action = %v, want ecsActionHost", action)
	}
}

func TestECSActionScreen_FargateHasNoHost(t *testing.T) {
	task := ecsActionTask()
	task.InstanceID = ""
	s := newECSActionScreen(task)
	if len(s.actions) != 2 {
		t.Fatalf("actions = %v, want command and shell only", s.actions)
	}
	if strings.Contains(s.View(), "Sessão na instância") {
		t.Fatalf("fargate task offers a host session: %q", s.View())
	}
}

func TestECSActionScreen_CursorStopsAtTheLastAction(t *testing.T) {
	s := newECSActionScreen(ecsActionTask())
	for range 10 {
		s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, action := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != ecsActionHost {
		t.Fatalf("action = %v, want ecsActionHost", action)
	}
}

func TestECSActionScreen_ViewShowsPlacement(t *testing.T) {
	v := newECSActionScreen(ecsActionTask()).View()
	for _, want := range []string{"stg-api-web", "vakinha-stg", "api-web", "i-aaa"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view is missing %q: %q", want, v)
		}
	}
}
