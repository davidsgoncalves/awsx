package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

var menuActions = []string{"Acessar EC2", "Acessar banco/serviço (túnel)", "Rodar comando", "Sair"}

type menuScreen struct {
	profile string
	region  string
	id      awsx.Identity
	cursor  int
}

func newMenuScreen(profile, region string, id awsx.Identity) menuScreen {
	return menuScreen{profile: profile, region: region, id: id}
}

func (m menuScreen) Update(msg tea.Msg) (menuScreen, screen) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, screenMenu
	}
	switch km.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(menuActions)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		switch m.cursor {
		case 0:
			return m, screenInstances
		case 1:
			return m, screenRDS
		case 2:
			return m, screenExecInstance
		default:
			return m, screenQuit
		}
	}
	return m, screenMenu
}

func (m menuScreen) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("AWSX") + "\n\n")
	fmt.Fprintf(&b, "Perfil: %s\n", m.profile)
	fmt.Fprintf(&b, "Conta: %s\n", m.id.Account)
	if role := roleFromArn(m.id.Arn); role != "" {
		fmt.Fprintf(&b, "Role: %s\n", role)
	}
	fmt.Fprintf(&b, "Região: %s\n\n", m.region)
	for i, a := range menuActions {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		b.WriteString(cursor + a + "\n")
	}
	return b.String()
}

// roleFromArn extracts the role name from an assumed-role ARN, or "".
func roleFromArn(arn string) string {
	const marker = ":assumed-role/"
	i := strings.Index(arn, marker)
	if i < 0 {
		return ""
	}
	rest := arn[i+len(marker):]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		return rest[:slash]
	}
	return rest
}
