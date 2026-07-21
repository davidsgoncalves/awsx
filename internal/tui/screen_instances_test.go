package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func targets() []awsx.Target {
	return []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api-production", Type: "t3.large", PrivateIP: "10.0.1.15", State: "running"}, SSMOnline: true},
		{Instance: awsx.Instance{ID: "i-2", Name: "worker-production", Type: "t3.medium", PrivateIP: "10.0.1.22", State: "running"}, SSMOnline: true},
	}
}

func TestInstancesScreen_EnterSelects(t *testing.T) {
	s := newInstancesScreen(targets())
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || sel.ID != "i-1" {
		t.Fatalf("want i-1 selected, got %+v", sel)
	}
}

func TestTargetItem_FilterValueIncludesIPAndID(t *testing.T) {
	it := targetItem{t: targets()[0]}
	fv := it.FilterValue()
	for _, want := range []string{"api-production", "i-1", "10.0.1.15", "t3.large"} {
		if !strings.Contains(fv, want) {
			t.Fatalf("FilterValue %q missing %q", fv, want)
		}
	}
}
