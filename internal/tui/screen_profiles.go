package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

type profileItem struct{ p profiles.Profile }

func (i profileItem) FilterValue() string { return i.p.Name }
func (i profileItem) Title() string {
	if i.p.IsSSO {
		return i.p.Name + "  (SSO)"
	}
	return i.p.Name
}
func (i profileItem) Description() string { return i.p.Region }

type profileScreen struct {
	list list.Model
}

func newProfileScreen(ps []profiles.Profile) profileScreen {
	items := make([]list.Item, len(ps))
	for i, p := range ps {
		items[i] = profileItem{p: p}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione um perfil AWS"
	return profileScreen{list: l}
}

func (s profileScreen) Update(msg tea.Msg) (profileScreen, *profiles.Profile, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(profileItem); ok {
			p := it.p
			return s, &p, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s profileScreen) View() string { return s.list.View() }
