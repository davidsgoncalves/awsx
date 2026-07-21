package tui

import (
	"strings"

	"github.com/davidsgoncalves/awsx/internal/deps"
)

type depsScreen struct {
	checks []deps.Dependency
}

func newDepsScreen(checks []deps.Dependency) depsScreen {
	return depsScreen{checks: checks}
}

func (d depsScreen) allFound() bool {
	return len(d.missing()) == 0
}

func (d depsScreen) missing() []deps.Dependency {
	var out []deps.Dependency
	for _, c := range d.checks {
		if !c.Found {
			out = append(out, c)
		}
	}
	return out
}

func (d depsScreen) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Verificando ambiente...") + "\n\n")
	for _, c := range d.checks {
		mark := styleOK.Render("✓")
		if !c.Found {
			mark = styleErr.Render("✗")
		}
		b.WriteString(mark + " " + c.Name + "\n")
	}
	return b.String()
}
