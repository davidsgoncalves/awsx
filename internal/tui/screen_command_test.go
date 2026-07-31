package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

var webContainer = awsx.Container{
	ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days",
}

// typeText feeds each rune of text to the screen as a key message.
func typeText(s commandScreen, text string) commandScreen {
	for _, r := range text {
		s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return s
}

func TestCommandScreen_PrefillsMostRecentHistoryEntry(t *testing.T) {
	s := newCommandScreen(webContainer, []string{"rails c", "bash"})
	if got := s.line(); got != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q", got)
	}
}

func TestCommandScreen_EmptyHistoryStartsBlank(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub != nil {
		t.Fatalf("expected no submit on a blank input, got %+v", sub)
	}
}

func TestCommandScreen_SubmitsDerivedLineAndInnerCommand(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "rails c")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Line != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q", sub.Line)
	}
	if sub.Inner != "rails c" {
		t.Fatalf("inner = %q, want 'rails c'", sub.Inner)
	}
}

func TestCommandScreen_PreviewMatchesDockerExecLine(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "bash")
	if !strings.Contains(s.View(), awsx.DockerExecLine("myapp-web-1", "bash")) {
		t.Fatalf("view does not show the derived line: %q", s.View())
	}
}

func TestCommandScreen_ViewNamesServiceAndContainer(t *testing.T) {
	v := newCommandScreen(webContainer, nil).View()
	if !strings.Contains(v, "web") || !strings.Contains(v, "myapp-web-1") {
		t.Fatalf("view missing container identity: %q", v)
	}
}

func TestCommandScreen_TabSeedsFullLineMode(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "rails c")

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !s.fullLineMode() {
		t.Fatal("expected full-line mode after tab")
	}
	if got := s.line(); got != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("seeded line = %q", got)
	}
}

func TestCommandScreen_FullLineModeSubmitsVerbatimWithNoHistory(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = typeText(s, "docker logs -f web")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Line != "docker logs -f web" {
		t.Fatalf("line = %q, want verbatim", sub.Line)
	}
	if sub.Inner != "" {
		t.Fatalf("inner = %q, want empty in full-line mode", sub.Inner)
	}
}

func TestCommandScreen_TabBackRestoresInnerValue(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "rails c")

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab}) // to full line
	s = typeText(s, " --sandbox")
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab}) // back to inner

	if s.fullLineMode() {
		t.Fatal("expected inner mode after the second tab")
	}
	if got := s.line(); got != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q, want the original inner command", got)
	}
}

func TestCommandScreen_HistoryNavigation(t *testing.T) {
	s := newCommandScreen(webContainer, []string{"rails c", "bash", "sh"})

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown}) // -> bash
	if got := s.line(); got != awsx.DockerExecLine("myapp-web-1", "bash") {
		t.Fatalf("after down: %q", got)
	}

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown}) // -> sh
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown}) // clamped at the last entry
	if got := s.line(); got != awsx.DockerExecLine("myapp-web-1", "sh") {
		t.Fatalf("after clamping down: %q", got)
	}

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp}) // -> bash
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp}) // -> rails c
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp}) // clamped at the first entry
	if got := s.line(); got != awsx.DockerExecLine("myapp-web-1", "rails c") {
		t.Fatalf("after clamping up: %q", got)
	}
}

func TestCommandScreen_HistoryIgnoredInFullLineMode(t *testing.T) {
	s := newCommandScreen(webContainer, []string{"rails c", "bash"})
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})

	before := s.line()
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	if s.line() != before {
		t.Fatalf("history moved in full-line mode: %q -> %q", before, s.line())
	}
}

func TestCommandScreen_TrimsWhitespaceOnSubmit(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "  rails c  ")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Inner != "rails c" {
		t.Fatalf("inner = %q, want trimmed", sub.Inner)
	}
	if sub.Line != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q", sub.Line)
	}
}

func TestCommandScreen_ViewShowsKeyHints(t *testing.T) {
	v := newCommandScreen(webContainer, nil).View()
	for _, want := range []string{"enter", "tab", "esc"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view missing %q hint: %q", want, v)
		}
	}
}

func TestManualCommandScreen_StartsInFullLineModeAndNamesInstance(t *testing.T) {
	s := newManualCommandScreen("api")
	if !s.fullLineMode() {
		t.Fatal("expected full-line mode")
	}
	if !strings.Contains(s.View(), "api") {
		t.Fatalf("view does not name the instance: %q", s.View())
	}
}

func TestManualCommandScreen_TabIsInert(t *testing.T) {
	s := newManualCommandScreen("api")
	s = typeText(s, "sudo docker exec -it web bash")

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !s.fullLineMode() {
		t.Fatal("tab must not leave full-line mode when no container was chosen")
	}
	if got := s.line(); got != "sudo docker exec -it web bash" {
		t.Fatalf("line = %q, want unchanged", got)
	}
}

func TestManualCommandScreen_SubmitsVerbatimWithNoHistory(t *testing.T) {
	s := newManualCommandScreen("api")
	s = typeText(s, "sudo docker exec -it web rails c")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Line != "sudo docker exec -it web rails c" {
		t.Fatalf("line = %q", sub.Line)
	}
	if sub.Inner != "" {
		t.Fatalf("inner = %q, want empty", sub.Inner)
	}
}
