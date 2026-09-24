package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenDeps screen = iota
	screenProfiles
	screenChecking
	screenLogin
	screenMenu
	screenInstances
	screenError
	screenQuit
	// Appended after screenQuit to preserve the numeric values of the states
	// above (some tests assert on them).
	screenAccounts
	screenRoles
	screenRegion
	screenRDS
	screenEC2Action
	screenContainers
	screenCommand
	screenECSCluster
	screenECSTask
	screenECSAction
	screenUpdate
)

type errorAction struct {
	label string
	next  screen
}

type errorScreen struct {
	title   string
	detail  string
	actions []errorAction
	cursor  int
}

func newErrorScreen(title, detail string, actions []errorAction) errorScreen {
	if len(actions) == 0 {
		actions = []errorAction{{label: "Sair", next: screenQuit}}
	}
	return errorScreen{title: title, detail: detail, actions: actions}
}

func (e errorScreen) Update(msg tea.Msg) (errorScreen, screen) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return e, screenError
	}
	switch km.Type {
	case tea.KeyUp:
		if e.cursor > 0 {
			e.cursor--
		}
	case tea.KeyDown:
		if e.cursor < len(e.actions)-1 {
			e.cursor++
		}
	case tea.KeyEnter:
		return e, e.actions[e.cursor].next
	}
	return e, screenError
}

func (e errorScreen) View() string {
	var b strings.Builder
	b.WriteString(styleErr.Render(e.title) + "\n\n")
	if e.detail != "" {
		b.WriteString(e.detail + "\n\n")
	}
	for i, a := range e.actions {
		cursor := "  "
		if i == e.cursor {
			cursor = "> "
		}
		b.WriteString(cursor + a.label + "\n")
	}
	return b.String()
}
