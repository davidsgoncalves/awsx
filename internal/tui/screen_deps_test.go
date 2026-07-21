package tui

import (
	"strings"
	"testing"

	"github.com/davidsgoncalves/awsx/internal/deps"
)

func TestDepsScreen_AllFound(t *testing.T) {
	d := newDepsScreen([]deps.Dependency{
		{Name: "AWS CLI", Binary: "aws", Found: true},
		{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
	})
	if !d.allFound() {
		t.Fatal("want allFound true")
	}
	if len(d.missing()) != 0 {
		t.Fatalf("want no missing, got %v", d.missing())
	}
}

func TestDepsScreen_MissingListed(t *testing.T) {
	d := newDepsScreen([]deps.Dependency{
		{Name: "AWS CLI", Binary: "aws", Found: false},
		{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
	})
	if d.allFound() {
		t.Fatal("want allFound false")
	}
	m := d.missing()
	if len(m) != 1 || m[0].Binary != "aws" {
		t.Fatalf("want [aws] missing, got %v", m)
	}
	if !strings.Contains(d.View(), "AWS CLI") {
		t.Fatalf("view should mention AWS CLI: %q", d.View())
	}
}
