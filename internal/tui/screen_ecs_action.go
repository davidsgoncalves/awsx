package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// ecsAction is what to do with the chosen ECS task.
type ecsAction int

const (
	ecsActionNone ecsAction = iota
	ecsActionCommand
	ecsActionShell
	ecsActionHost
)

// ecsActionScreen offers the ways into a task that has already been picked.
// The host entry is left out for Fargate tasks, which run on no instance of
// ours and so have nothing to open a session against.
type ecsActionScreen struct {
	task    awsx.ECSTask
	actions []ecsAction
	cursor  int
}

func newECSActionScreen(t awsx.ECSTask) ecsActionScreen {
	actions := []ecsAction{ecsActionCommand, ecsActionShell}
	if t.InstanceID != "" {
		actions = append(actions, ecsActionHost)
	}
	return ecsActionScreen{task: t, actions: actions}
}

func ecsActionLabel(a ecsAction) string {
	switch a {
	case ecsActionCommand:
		return "Rodar comando"
	case ecsActionShell:
		return "Shell no container"
	case ecsActionHost:
		return "Acessar host (SSM)"
	}
	return ""
}

func (s ecsActionScreen) Update(msg tea.Msg) (ecsActionScreen, ecsAction) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, ecsActionNone
	}
	switch km.Type {
	case tea.KeyUp:
		if s.cursor > 0 {
			s.cursor--
		}
	case tea.KeyDown:
		if s.cursor < len(s.actions)-1 {
			s.cursor++
		}
	case tea.KeyEnter:
		return s, s.actions[s.cursor]
	}
	return s, ecsActionNone
}

func (s ecsActionScreen) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(awsx.DisplayTask(s.task)) + "\n\n")
	b.WriteString(styleFaint.Render(taskPlacement(s.task)) + "\n\n")
	for i, a := range s.actions {
		cursor := "  "
		if i == s.cursor {
			cursor = "> "
		}
		b.WriteString(cursor + ecsActionLabel(a) + "\n")
	}
	b.WriteString("\n" + styleFaint.Render("enter confirma · esc volta") + "\n")
	return b.String()
}
