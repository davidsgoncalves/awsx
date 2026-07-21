package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// --- accounts ---

type accountItem struct{ a awsx.Account }

func (i accountItem) Title() string {
	if i.a.Name != "" {
		return i.a.Name
	}
	return i.a.ID
}
func (i accountItem) Description() string { return i.a.ID }
func (i accountItem) FilterValue() string { return i.a.Name + " " + i.a.ID }

type accountScreen struct{ list list.Model }

func newAccountScreen(accounts []awsx.Account) accountScreen {
	items := make([]list.Item, len(accounts))
	for i, a := range accounts {
		items[i] = accountItem{a: a}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione uma conta"
	return accountScreen{list: l}
}

func (s accountScreen) Update(msg tea.Msg) (accountScreen, *awsx.Account, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(accountItem); ok {
			a := it.a
			return s, &a, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s accountScreen) View() string { return s.list.View() }

// --- roles ---

type roleItem struct{ r awsx.Role }

func (i roleItem) Title() string       { return i.r.Name }
func (i roleItem) Description() string  { return "" }
func (i roleItem) FilterValue() string { return i.r.Name }

type roleScreen struct{ list list.Model }

func newRoleScreen(roles []awsx.Role) roleScreen {
	items := make([]list.Item, len(roles))
	for i, r := range roles {
		items[i] = roleItem{r: r}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione uma role"
	return roleScreen{list: l}
}

func (s roleScreen) Update(msg tea.Msg) (roleScreen, *awsx.Role, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(roleItem); ok {
			r := it.r
			return s, &r, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s roleScreen) View() string { return s.list.View() }

// --- regions ---

type regionItem struct{ name string }

func (i regionItem) Title() string       { return i.name }
func (i regionItem) Description() string  { return "" }
func (i regionItem) FilterValue() string { return i.name }

type regionScreen struct{ list list.Model }

func newRegionScreen(names []string) regionScreen {
	items := make([]list.Item, len(names))
	for i, n := range names {
		items[i] = regionItem{name: n}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione uma região"
	return regionScreen{list: l}
}

func (s regionScreen) Update(msg tea.Msg) (regionScreen, *string, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(regionItem); ok {
			name := it.name
			return s, &name, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s regionScreen) View() string { return s.list.View() }
