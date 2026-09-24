package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// ec2Action is what to do with the chosen EC2 instance.
type ec2Action int

const (
	ec2ActionNone ec2Action = iota
	ec2ActionSession
	ec2ActionExec
	ec2ActionTunnel
)

var ec2Actions = []ec2Action{ec2ActionSession, ec2ActionExec, ec2ActionTunnel}

// ec2ActionScreen offers the ways into an instance that has already been
// picked, mirroring what ecsActionScreen does for a task.
type ec2ActionScreen struct {
	target awsx.Target
	cursor int
}

func newEC2ActionScreen(t awsx.Target) ec2ActionScreen {
	return ec2ActionScreen{target: t}
}

func ec2ActionLabel(a ec2Action) string {
	switch a {
	case ec2ActionSession:
		return "Sessão na instância"
	case ec2ActionExec:
		return "Comando em container Docker"
	case ec2ActionTunnel:
		return "Túnel para banco RDS"
	}
	return ""
}

func (s ec2ActionScreen) Update(msg tea.Msg) (ec2ActionScreen, ec2Action) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, ec2ActionNone
	}
	switch km.Type {
	case tea.KeyUp:
		if s.cursor > 0 {
			s.cursor--
		}
	case tea.KeyDown:
		if s.cursor < len(ec2Actions)-1 {
			s.cursor++
		}
	case tea.KeyEnter:
		return s, ec2Actions[s.cursor]
	}
	return s, ec2ActionNone
}

func (s ec2ActionScreen) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(awsx.DisplayName(s.target.Instance)) + "\n\n")
	b.WriteString(styleFaint.Render(fmt.Sprintf("%s · %s", s.target.ID, s.target.PrivateIP)) + "\n\n")
	for i, a := range ec2Actions {
		cursor := "  "
		if i == s.cursor {
			cursor = "> "
		}
		b.WriteString(cursor + ec2ActionLabel(a) + "\n")
	}
	b.WriteString("\n" + styleFaint.Render("enter confirma · esc volta") + "\n")
	return b.String()
}
