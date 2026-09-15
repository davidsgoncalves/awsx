package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func ecsTasks() []awsx.ECSTask {
	return []awsx.ECSTask{
		{Cluster: "vakinha-stg", TaskARN: "arn/abc123", Service: "stg-api-web", Container: "api-web",
			InstanceID: "i-aaa", Status: "RUNNING", ExecEnabled: true},
		{Cluster: "vakinha-stg", TaskARN: "arn/def456", Service: "stg-web", Container: "web",
			Status: "RUNNING", ExecEnabled: true},
	}
}

func TestECSTaskScreen_EnterSelects(t *testing.T) {
	s := newECSTaskScreen(ecsTasks())
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || sel.Service != "stg-api-web" {
		t.Fatalf("want stg-api-web selected, got %+v", sel)
	}
}

func TestECSTaskItem_DescriptionShowsNode(t *testing.T) {
	d := ecsTaskItem{t: ecsTasks()[0]}.Description()
	for _, want := range []string{"api-web", "i-aaa", "abc123"} {
		if !strings.Contains(d, want) {
			t.Fatalf("description %q missing %q", d, want)
		}
	}
}

func TestECSTaskItem_DescriptionMarksFargate(t *testing.T) {
	d := ecsTaskItem{t: ecsTasks()[1]}.Description()
	if !strings.Contains(d, "fargate") {
		t.Fatalf("a task with no node should read as fargate: %q", d)
	}
}

func TestECSClusterScreen_EnterSelects(t *testing.T) {
	s := newECSClusterScreen([]string{"metabase", "vakinha-stg"})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || *sel != "metabase" {
		t.Fatalf("want metabase, got %v", sel)
	}
}

func TestECSCommandScreen_LineIsTheBareCommand(t *testing.T) {
	s := newECSCommandScreen(ecsTasks()[0], nil)
	s.input.SetValue("rails c")
	if got := s.line(); got != "rails c" {
		t.Fatalf("ECS mode should not wrap the command in docker exec, got %q", got)
	}
	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil || sub.Line != "rails c" || sub.Inner != "rails c" {
		t.Fatalf("submit = %+v", sub)
	}
}

func TestECSCommandScreen_TabDoesNotSwitchMode(t *testing.T) {
	s := newECSCommandScreen(ecsTasks()[0], nil)
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	if s.fullLineMode() {
		t.Fatal("ECS mode has no full-line editing to switch to")
	}
	if got := s.line(); got != "rails c" {
		t.Fatalf("value changed after tab: %q", got)
	}
}

func TestECSCommandScreen_ViewShowsPlacement(t *testing.T) {
	v := newECSCommandScreen(ecsTasks()[0], nil).View()
	for _, want := range []string{"stg-api-web", "vakinha-stg", "abc123", "api-web"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view %q missing %q", v, want)
		}
	}
}

func TestECSCommandScreen_ViewShowsShellWrapping(t *testing.T) {
	s := newECSCommandScreen(ecsTasks()[0], nil)
	if !strings.Contains(s.View(), `/bin/sh -c 'export PATH="$PWD/bin:$PATH"; rails c'`) {
		t.Fatalf("view should show the wrapped line: %q", s.View())
	}
}

func TestECSCommandScreen_PrefillsAConsole(t *testing.T) {
	s := newECSCommandScreen(ecsTasks()[0], nil)
	if got := s.line(); got != "rails c" {
		t.Fatalf("want a console offered by default, got %q", got)
	}
	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil || sub.Line != "rails c" {
		t.Fatalf("enter on the default should submit it, got %+v", sub)
	}
}

func TestECSCommandScreen_HistoryWinsOverTheDefault(t *testing.T) {
	s := newECSCommandScreen(ecsTasks()[0], []string{"rake db:migrate:status"})
	if got := s.line(); got != "rake db:migrate:status" {
		t.Fatalf("want the last command, got %q", got)
	}
}
