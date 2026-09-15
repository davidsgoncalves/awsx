package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func targets() []awsx.Target {
	return []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api-production", Type: "t3.large", PrivateIP: "10.0.1.15", State: "running", Tags: map[string]string{"Name": "api-production", "Environment": "production", "Project": "vakinha-api"}}, SSMOnline: true},
		{Instance: awsx.Instance{ID: "i-2", Name: "worker-production", Type: "t3.medium", PrivateIP: "10.0.1.22", State: "running"}, SSMOnline: true},
	}
}

func TestInstancesScreen_EnterSelects(t *testing.T) {
	s := newInstancesScreen(targets())
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

func TestInstancesScreen_TagPanelShowsSelectedInstanceTags(t *testing.T) {
	s := newInstancesScreen(targets())
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	panel := s.tagPanel()
	for _, want := range []string{"i-1", "Environment=production", "Project=vakinha-api"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("tag panel %q missing %q", panel, want)
		}
	}
	if strings.Contains(panel, "Name=api-production") {
		t.Fatalf("tag panel %q should omit the Name tag", panel)
	}
}

func TestInstancesScreen_TagPanelFollowsCursor(t *testing.T) {
	s := newInstancesScreen(targets())
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	panel := s.tagPanel()
	if !strings.Contains(panel, "i-2") || !strings.Contains(panel, "(sem tags)") {
		t.Fatalf("tag panel %q should describe i-2 with no tags", panel)
	}
}

func TestTagRows_TruncatesBeyondPanelHeight(t *testing.T) {
	var tags []awsx.TagPair
	for i := range 40 {
		tags = append(tags, awsx.TagPair{Key: fmt.Sprintf("key%02d", i), Value: "value"})
	}
	rows := tagRows(tags, 40)
	if len(rows) != tagPanelRows {
		t.Fatalf("want %d rows, got %d", tagPanelRows, len(rows))
	}
	if !strings.Contains(rows[tagPanelRows-1], "tags") {
		t.Fatalf("last row %q should report the remaining tags", rows[tagPanelRows-1])
	}
}
