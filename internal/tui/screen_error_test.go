package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestErrorScreen_EnterReturnsSelectedNext(t *testing.T) {
	e := newErrorScreen("Boom", "it broke", []errorAction{
		{label: "Retry", next: screenChecking},
		{label: "Quit", next: screenQuit},
	})
	// move down to "Quit", press enter
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, next := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenQuit {
		t.Fatalf("next = %v, want screenQuit", next)
	}
}

func TestErrorScreen_ViewShowsDetail(t *testing.T) {
	e := newErrorScreen("Denied", "Permission needed: ec2:DescribeInstances", nil)
	if !strings.Contains(e.View(), "ec2:DescribeInstances") {
		t.Fatalf("view missing detail: %q", e.View())
	}
}
