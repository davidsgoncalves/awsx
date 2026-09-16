package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/update"
)

// updateStage is where the update flow currently is.
type updateStage int

const (
	updateChecking updateStage = iota
	updateFound
	updateCurrent
	updateApplying
	updateDone
)

// updateScreen shows which version is running, which one is published, and
// how this copy can be replaced. Nothing reaches the network until the screen
// is opened.
type updateScreen struct {
	stage   updateStage
	current string
	latest  string
	method  update.Method
	path    string
	err     error
}

func newUpdateScreen(current string) updateScreen {
	return updateScreen{stage: updateChecking, current: current}
}

// applied records the result of the check, moving the screen to the state the
// answer calls for.
func (s updateScreen) applied(msg updateCheckMsg) updateScreen {
	if msg.err != nil {
		s.stage, s.err = updateDone, msg.err
		return s
	}
	s.latest, s.method, s.path = msg.latest, msg.method, msg.path
	if update.Compare(s.current, msg.latest) >= 0 {
		s.stage = updateCurrent
		return s
	}
	s.stage = updateFound
	return s
}

// canApply reports whether enter starts an upgrade, as opposed to the screen
// only naming the command to run.
func (s updateScreen) canApply() bool {
	return s.stage == updateFound && s.method != update.MethodGoInstall
}

func (s updateScreen) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Atualizar AWSX") + "\n\n")

	switch s.stage {
	case updateChecking:
		b.WriteString(styleFaint.Render("Consultando releases...") + "\n")
		return b.String()

	case updateApplying:
		b.WriteString(styleFaint.Render("Atualizando...") + "\n")
		return b.String()

	case updateDone:
		if s.err != nil {
			b.WriteString(styleErr.Render("Não foi possível atualizar.") + "\n\n")
			b.WriteString(s.err.Error() + "\n\n")
			if s.method == update.MethodArchive {
				b.WriteString(styleFaint.Render("Alternativa:") + "\n")
				b.WriteString(update.InstallerLine() + "\n\n")
			}
			b.WriteString(styleFaint.Render("esc volta") + "\n")
			return b.String()
		}
		b.WriteString(styleOK.Render("Atualizado para "+s.latest+".") + "\n\n")
		b.WriteString(styleFaint.Render("Feche e abra o AWSX para usar a versão nova.") + "\n\n")
		b.WriteString(styleFaint.Render("esc volta") + "\n")
		return b.String()

	case updateCurrent:
		fmt.Fprintf(&b, "Versão %s — nenhuma atualização disponível.\n\n", s.current)
		b.WriteString(styleFaint.Render("esc volta") + "\n")
		return b.String()
	}

	fmt.Fprintf(&b, "%s  →  %s disponível\n\n", s.current, s.latest)
	switch s.method {
	case update.MethodBrew:
		b.WriteString(styleFaint.Render("instalado via Homebrew") + "\n")
		b.WriteString("brew " + strings.Join(update.BrewUpgradeArgs(), " ") + "\n\n")
		b.WriteString(styleFaint.Render("enter atualiza · esc volta") + "\n")
	case update.MethodGoInstall:
		b.WriteString(styleFaint.Render("instalado via go install — o comando abaixo atualiza") + "\n")
		b.WriteString(update.GoInstallLine() + "\n\n")
		b.WriteString(styleFaint.Render("esc volta") + "\n")
	default:
		b.WriteString(styleFaint.Render(s.path) + "\n\n")
		b.WriteString(styleFaint.Render("enter baixa, confere o checksum e substitui o binário · esc volta") + "\n")
	}
	return b.String()
}

// Update handles enter on the screen; every other key is ignored, and esc is
// handled by the root so it can route back to the menu.
func (s updateScreen) Update(msg tea.Msg) (updateScreen, bool) {
	km, ok := msg.(tea.KeyMsg)
	if !ok || km.Type != tea.KeyEnter || !s.canApply() {
		return s, false
	}
	s.stage = updateApplying
	return s, true
}
