package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// --- clusters ---

type clusterItem struct{ name string }

func (i clusterItem) Title() string       { return i.name }
func (i clusterItem) Description() string { return "" }
func (i clusterItem) FilterValue() string { return i.name }

type ecsClusterScreen struct{ list list.Model }

func newECSClusterScreen(names []string) ecsClusterScreen {
	items := make([]list.Item, len(names))
	for i, n := range names {
		items[i] = clusterItem{name: n}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione um cluster ECS"
	return ecsClusterScreen{list: l}
}

func (s ecsClusterScreen) Update(msg tea.Msg) (ecsClusterScreen, *string, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(clusterItem); ok {
			name := it.name
			return s, &name, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s ecsClusterScreen) View() string { return s.list.View() }

// --- tasks ---

type ecsTaskItem struct{ t awsx.ECSTask }

func (i ecsTaskItem) Title() string { return awsx.DisplayTask(i.t) }
func (i ecsTaskItem) Description() string {
	return fmt.Sprintf("%s   %s   task %s", i.t.Container, taskNode(i.t), awsx.TaskID(i.t.TaskARN))
}
func (i ecsTaskItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s %s",
		awsx.DisplayTask(i.t), i.t.Container, i.t.InstanceID, awsx.TaskID(i.t.TaskARN))
}

type ecsTaskScreen struct{ list list.Model }

func newECSTaskScreen(tasks []awsx.ECSTask) ecsTaskScreen {
	items := make([]list.Item, len(tasks))
	for i, t := range tasks {
		items[i] = ecsTaskItem{t: t}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione o container"
	return ecsTaskScreen{list: l}
}

func (s ecsTaskScreen) Update(msg tea.Msg) (ecsTaskScreen, *awsx.ECSTask, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(ecsTaskItem); ok {
			t := it.t
			return s, &t, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s ecsTaskScreen) View() string { return s.list.View() }

// taskPlacement names where a task runs, for the screens that show it before
// anything is executed against it.
func taskPlacement(t awsx.ECSTask) string {
	return fmt.Sprintf("cluster %s · task %s · container %s · %s",
		t.Cluster, awsx.TaskID(t.TaskARN), t.Container, taskNode(t))
}

// taskNode is the instance the task landed on, or "fargate" when it has none.
func taskNode(t awsx.ECSTask) string {
	if t.InstanceID == "" {
		return "fargate"
	}
	return t.InstanceID
}
