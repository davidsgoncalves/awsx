package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

// profileItem is a regular AWS profile in the selection list.
type profileItem struct{ p profiles.Profile }

func (i profileItem) FilterValue() string { return i.p.Name }
func (i profileItem) Title() string {
	if i.p.IsSSO {
		return i.p.Name + "  (SSO)"
	}
	return i.p.Name
}
func (i profileItem) Description() string { return i.p.Region }

// ssoSessionItem is an [sso-session] entry in the selection list. Selecting it
// branches into the account/role/region pickers.
type ssoSessionItem struct{ s profiles.SSOSession }

func (i ssoSessionItem) Title() string       { return i.s.Name + "  (SSO session)" }
func (i ssoSessionItem) Description() string  { return i.s.StartURL }
func (i ssoSessionItem) FilterValue() string { return i.s.Name }

// selectionScreen is the entry list: SSO sessions (first) plus regular
// profiles. Selecting a session yields *profiles.SSOSession; selecting a
// profile yields *profiles.Profile.
type selectionScreen struct {
	list list.Model
}

func newSelectionScreen(ps []profiles.Profile, sessions []profiles.SSOSession) selectionScreen {
	items := make([]list.Item, 0, len(ps)+len(sessions))
	for _, s := range sessions {
		items = append(items, ssoSessionItem{s: s})
	}
	for _, p := range ps {
		items = append(items, profileItem{p: p})
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione um perfil ou sessão SSO"
	return selectionScreen{list: l}
}

func (s selectionScreen) Update(msg tea.Msg) (selectionScreen, *profiles.Profile, *profiles.SSOSession, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		switch it := s.list.SelectedItem().(type) {
		case ssoSessionItem:
			sess := it.s
			return s, nil, &sess, nil
		case profileItem:
			p := it.p
			return s, &p, nil, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, nil, cmd
}

func (s selectionScreen) View() string { return s.list.View() }
