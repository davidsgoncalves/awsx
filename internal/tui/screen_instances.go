package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

type targetItem struct{ t awsx.Target }

func (i targetItem) Title() string { return awsx.DisplayName(i.t.Instance) }
func (i targetItem) Description() string {
	return fmt.Sprintf("%s   %s   %s   %s", i.t.State, i.t.Type, i.t.PrivateIP, i.t.ID)
}
func (i targetItem) FilterValue() string {
	return strings.Join([]string{
		awsx.DisplayName(i.t.Instance), i.t.ID, i.t.PrivateIP, i.t.Type,
	}, " ")
}

type instancesScreen struct {
	list list.Model
}

func newInstancesScreen(targets []awsx.Target) instancesScreen {
	items := make([]list.Item, len(targets))
	for i, t := range targets {
		items[i] = targetItem{t: t}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione uma instância"
	return instancesScreen{list: l}
}

func (s instancesScreen) Update(msg tea.Msg) (instancesScreen, *awsx.Target, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(targetItem); ok {
			t := it.t
			return s, &t, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s instancesScreen) View() string { return s.list.View() }
