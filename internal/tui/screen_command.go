package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// commandSubmit is a confirmed command. Line is what actually runs; Inner is
// the in-container command to remember, empty when the user edited the whole
// line by hand (there is nothing container-scoped to record).
type commandSubmit struct {
	Line  string
	Inner string
}

// commandScreen asks for the command to run inside a container. By default the
// input holds only the in-container part and the full docker exec line is
// derived from it; tab switches to editing that line verbatim, for instances
// whose setup does not match the sudo docker exec assumption.
type commandScreen struct {
	container awsx.Container
	input     textinput.Model
	history   []string
	// cursor indexes history; len(history) means "not browsing".
	cursor int
	// fullLine is true while the whole command line is being edited.
	fullLine bool
	// savedInner keeps the in-container value while in full-line mode.
	savedInner string
	// manual means no container was chosen (the listing failed), so there is
	// no in-container mode to switch back to and no history to record.
	manual bool
	// ecsTask is set when the command runs through ECS Exec, where the input
	// is always the in-container command and there is no docker exec line to
	// edit.
	ecsTask *awsx.ECSTask
	// target labels the header in manual mode, where there is no container.
	target string
}

func newCommandScreen(c awsx.Container, history []string) commandScreen {
	ti := newCommandInput("rails c")
	cursor := len(history)
	if len(history) > 0 {
		ti.SetValue(history[0])
		cursor = 0
	}
	return commandScreen{container: c, input: ti, history: history, cursor: cursor}
}

// newECSCommandScreen asks for the command to run through ECS Exec. The task
// already names the container and the node, so only the in-container command
// is typed.
func newECSCommandScreen(t awsx.ECSTask, history []string) commandScreen {
	ti := newCommandInput("bin/rails c")
	cursor := len(history)
	if len(history) > 0 {
		ti.SetValue(history[0])
		cursor = 0
	}
	return commandScreen{input: ti, history: history, cursor: cursor, ecsTask: &t}
}

// newManualCommandScreen is the fallback when the container list could not be
// fetched: the whole command line is typed by hand.
func newManualCommandScreen(instanceName string) commandScreen {
	return commandScreen{
		input:    newCommandInput("sudo docker exec -it web rails c"),
		fullLine: true,
		manual:   true,
		target:   instanceName,
	}
}

func newCommandInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = "> "
	ti.Width = 60
	ti.Focus()
	return ti
}

func (s commandScreen) fullLineMode() bool { return s.fullLine }

// line is the full command that would run right now.
func (s commandScreen) line() string {
	v := strings.TrimSpace(s.input.Value())
	if s.fullLine || s.ecsTask != nil {
		return v
	}
	if v == "" {
		return ""
	}
	return awsx.DockerExecLine(s.container.Name, v)
}

func (s commandScreen) Update(msg tea.Msg) (commandScreen, *commandSubmit, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, nil, cmd
	}

	switch km.Type {
	case tea.KeyEnter:
		line := s.line()
		if line == "" {
			return s, nil, nil
		}
		sub := commandSubmit{Line: line}
		if s.ecsTask != nil {
			sub.Inner = line
			return s, &sub, nil
		}
		if !s.fullLine {
			sub.Inner = strings.TrimSpace(s.input.Value())
		}
		return s, &sub, nil

	case tea.KeyTab:
		if s.ecsTask != nil {
			return s, nil, nil
		}
		return s.toggleMode(), nil, nil

	case tea.KeyUp:
		if s.fullLine {
			return s, nil, nil
		}
		return s.moveHistory(-1), nil, nil

	case tea.KeyDown:
		if s.fullLine {
			return s, nil, nil
		}
		return s.moveHistory(1), nil, nil
	}

	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return s, nil, cmd
}

// toggleMode switches between editing the in-container command and the whole
// line, seeding the new value from the current one. It is a no-op in manual
// mode, where there is no container to build an in-container command against.
func (s commandScreen) toggleMode() commandScreen {
	if s.manual {
		return s
	}
	if s.fullLine {
		s.fullLine = false
		s.input.SetValue(s.savedInner)
		s.input.CursorEnd()
		return s
	}
	s.savedInner = strings.TrimSpace(s.input.Value())
	s.fullLine = true
	// With nothing typed there is no line to seed: a bare "docker exec -it X "
	// would just have to be deleted before writing something else.
	if s.savedInner == "" {
		s.input.SetValue("")
		return s
	}
	s.input.SetValue(awsx.DockerExecLine(s.container.Name, s.savedInner))
	s.input.CursorEnd()
	return s
}

// moveHistory steps the history cursor by delta, clamped to the list.
func (s commandScreen) moveHistory(delta int) commandScreen {
	if len(s.history) == 0 {
		return s
	}
	next := s.cursor + delta
	if next < 0 {
		next = 0
	}
	if next > len(s.history)-1 {
		next = len(s.history) - 1
	}
	s.cursor = next
	s.input.SetValue(s.history[next])
	s.input.CursorEnd()
	return s
}

func (s commandScreen) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", styleTitle.Render(s.header()))
	b.WriteString(s.input.View() + "\n\n")

	if s.ecsTask != nil {
		b.WriteString(styleFaint.Render(s.ecsPreview()) + "\n")
		if line := s.line(); line != "" {
			b.WriteString(styleFaint.Render(awsx.ECSShellLine(line)) + "\n\n")
		} else {
			b.WriteString("\n")
		}
		b.WriteString(styleFaint.Render("enter executa · ↑↓ histórico · esc volta") + "\n")
		return b.String()
	}

	if s.manual {
		b.WriteString(styleFaint.Render("digite a linha completa que será executada na instância") + "\n\n")
		b.WriteString(styleFaint.Render("enter executa · tab indisponível · esc volta") + "\n")
		return b.String()
	}

	if s.fullLine {
		b.WriteString(styleFaint.Render("editando a linha completa") + "\n\n")
		b.WriteString(styleFaint.Render("enter executa · tab volta ao comando · esc volta") + "\n")
		return b.String()
	}

	if line := s.line(); line != "" {
		b.WriteString(styleFaint.Render(line) + "\n\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString(styleFaint.Render("enter executa · tab edita a linha toda · ↑↓ histórico · esc volta") + "\n")
	return b.String()
}

// ecsPreview shows the ECS Exec call that will run, so the cluster, task and
// container are visible before confirming.
func (s commandScreen) ecsPreview() string {
	t := s.ecsTask
	node := t.InstanceID
	if node == "" {
		node = "fargate"
	}
	return fmt.Sprintf("ecs execute-command · cluster %s · task %s · container %s · %s",
		t.Cluster, awsx.TaskID(t.TaskARN), t.Container, node)
}

// header names what the command will run against: the container, or the
// instance when no container could be listed.
func (s commandScreen) header() string {
	if s.ecsTask != nil {
		return "Rodar comando em " + awsx.DisplayTask(*s.ecsTask)
	}
	if s.manual {
		return "Rodar comando em " + s.target
	}
	return fmt.Sprintf("Rodar comando em %s (%s)",
		awsx.DisplayContainer(s.container), s.container.Name)
}
