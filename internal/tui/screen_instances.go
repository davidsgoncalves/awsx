package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

const (
	tagPanelRows   = 4
	tagPanelHeight = tagPanelRows + 2
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
	list  list.Model
	width int
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
		s.width = ws.Width
		s.list.SetSize(ws.Width, ws.Height-2-tagPanelHeight)
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

func (s instancesScreen) View() string { return s.list.View() + "\n" + s.tagPanel() }

func (s instancesScreen) tagPanel() string {
	width := s.width
	if width <= 0 {
		width = 80
	}
	lines := []string{styleFaint.Render(strings.Repeat("─", width))}
	it, ok := s.list.SelectedItem().(targetItem)
	if !ok {
		return strings.Join(lines, "\n")
	}
	lines = append(lines, styleTitle.Render("Tags")+"  "+styleFaint.Render(it.t.ID))
	tags := awsx.SortedTags(it.t.Instance)
	if len(tags) == 0 {
		return strings.Join(append(lines, styleFaint.Render("  (sem tags)")), "\n")
	}
	return strings.Join(append(lines, tagRows(tags, width)...), "\n")
}

func tagRows(tags []awsx.TagPair, width int) []string {
	limit := max(width-2, 8)
	cells := make([]string, len(tags))
	cellWidth := 0
	for i, t := range tags {
		cells[i] = truncate(t.Key+"="+t.Value, limit)
		cellWidth = max(cellWidth, lipgloss.Width(cells[i]))
	}
	cellWidth += 3
	cols := max(limit/cellWidth, 1)

	var rows []string
	for i := 0; i < len(cells); i += cols {
		end := min(i+cols, len(cells))
		if len(rows) == tagPanelRows-1 && end < len(cells) {
			rows = append(rows, styleFaint.Render(fmt.Sprintf("  +%d tags", len(cells)-i)))
			break
		}
		var b strings.Builder
		b.WriteString("  ")
		for _, c := range cells[i:end] {
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", cellWidth-lipgloss.Width(c)))
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	return rows
}

func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "…"
}
