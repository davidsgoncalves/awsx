package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func TestAccountScreen_EnterSelects(t *testing.T) {
	s := newAccountScreen([]awsx.Account{
		{ID: "111111111111", Name: "prod"},
		{ID: "222222222222", Name: "staging"},
	})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || sel.ID != "111111111111" {
		t.Fatalf("want first account, got %+v", sel)
	}
}

func TestRoleScreen_EnterSelects(t *testing.T) {
	s := newRoleScreen([]awsx.Role{{Name: "SystemAdministrator"}, {Name: "ReadOnly"}})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || sel.Name != "SystemAdministrator" {
		t.Fatalf("want first role, got %+v", sel)
	}
}

func TestRegionScreen_EnterSelects(t *testing.T) {
	s := newRegionScreen([]string{"us-east-1", "sa-east-1"}, "")
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || *sel != "us-east-1" {
		t.Fatalf("want us-east-1, got %v", sel)
	}
}

func TestRegionScreen_Preselect(t *testing.T) {
	s := newRegionScreen([]string{"us-east-1", "sa-east-1", "eu-west-1"}, "sa-east-1")
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	// Enter without moving should pick the preselected region.
	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || *sel != "sa-east-1" {
		t.Fatalf("want preselected sa-east-1, got %v", sel)
	}
}

func TestContainersScreen_SelectReturnsContainer(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
		{ID: "def", Name: "myapp-sidekiq-1", Service: "sidekiq", Image: "ruby:3.2", Status: "Up 3 days"},
	})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil {
		t.Fatal("expected a selection")
	}
	if sel.Name != "myapp-web-1" {
		t.Fatalf("selected %q, want myapp-web-1", sel.Name)
	}
}

func TestContainersScreen_TitleUsesComposeServiceThenName(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{Name: "myapp-web-1", Service: "web"},
		{Name: "standalone"},
	})
	items := s.list.Items()

	if got := items[0].(containerItem).Title(); got != "web" {
		t.Fatalf("first title = %q, want web", got)
	}
	if got := items[1].(containerItem).Title(); got != "standalone" {
		t.Fatalf("second title = %q, want standalone", got)
	}
}

func TestContainersScreen_DescriptionShowsNameImageStatus(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
	})
	got := s.list.Items()[0].(containerItem).Description()
	for _, want := range []string{"myapp-web-1", "ruby:3.2", "Up 3 days"} {
		if !strings.Contains(got, want) {
			t.Fatalf("description %q missing %q", got, want)
		}
	}
}

func TestContainersScreen_FilterMatchesServiceNameAndImage(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{Name: "myapp-web-1", Service: "web", Image: "ruby:3.2"},
	})
	got := s.list.Items()[0].(containerItem).FilterValue()
	for _, want := range []string{"web", "myapp-web-1", "ruby:3.2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("filter value %q missing %q", got, want)
		}
	}
}
