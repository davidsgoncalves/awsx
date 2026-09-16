package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/update"
)

func TestUpdateScreen_NewerReleaseIsOffered(t *testing.T) {
	s := newUpdateScreen("v0.5.0").applied(updateCheckMsg{
		latest: "v0.6.0", method: update.MethodArchive, path: "/usr/local/bin/awsx",
	})
	if s.stage != updateFound {
		t.Fatalf("stage = %v, want updateFound", s.stage)
	}
	if !s.canApply() {
		t.Fatal("an archive install should be upgradable in place")
	}
	v := s.View()
	if !strings.Contains(v, "v0.6.0") || !strings.Contains(v, "/usr/local/bin/awsx") {
		t.Fatalf("view is missing the version or the path: %q", v)
	}
}

func TestUpdateScreen_SameVersionSaysSo(t *testing.T) {
	s := newUpdateScreen("v0.6.0").applied(updateCheckMsg{latest: "v0.6.0"})
	if s.stage != updateCurrent {
		t.Fatalf("stage = %v, want updateCurrent", s.stage)
	}
	if s.canApply() {
		t.Fatal("nothing to apply when the version is current")
	}
	if !strings.Contains(s.View(), "nenhuma atualização") {
		t.Fatalf("view = %q", s.View())
	}
}

func TestUpdateScreen_GoInstallOnlyShowsTheCommand(t *testing.T) {
	s := newUpdateScreen("v0.5.0").applied(updateCheckMsg{
		latest: "v0.6.0", method: update.MethodGoInstall,
	})
	if s.canApply() {
		t.Fatal("a go install build must not be replaced by awsx")
	}
	if !strings.Contains(s.View(), update.GoInstallLine()) {
		t.Fatalf("view does not name the command: %q", s.View())
	}
	if _, apply := s.Update(tea.KeyMsg{Type: tea.KeyEnter}); apply {
		t.Fatal("enter must not start an upgrade")
	}
}

func TestUpdateScreen_BrewShowsTheUpgradeCommand(t *testing.T) {
	s := newUpdateScreen("v0.5.0").applied(updateCheckMsg{
		latest: "v0.6.0", method: update.MethodBrew,
	})
	if !s.canApply() {
		t.Fatal("brew installs are upgradable")
	}
	if !strings.Contains(s.View(), "brew upgrade --cask") {
		t.Fatalf("view = %q", s.View())
	}
}

func TestUpdateScreen_EnterMovesToApplying(t *testing.T) {
	s := newUpdateScreen("v0.5.0").applied(updateCheckMsg{
		latest: "v0.6.0", method: update.MethodArchive, path: "/usr/local/bin/awsx",
	})
	s, apply := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !apply || s.stage != updateApplying {
		t.Fatalf("apply = %v, stage = %v", apply, s.stage)
	}
}

func TestUpdateScreen_CheckFailureIsShown(t *testing.T) {
	s := newUpdateScreen("v0.5.0").applied(updateCheckMsg{err: errors.New("sem rede")})
	if s.stage != updateDone || s.err == nil {
		t.Fatalf("stage = %v, err = %v", s.stage, s.err)
	}
	if !strings.Contains(s.View(), "sem rede") {
		t.Fatalf("view = %q", s.View())
	}
}

func TestUpdateScreen_ArchiveFailureOffersTheInstaller(t *testing.T) {
	s := newUpdateScreen("v0.5.0").applied(updateCheckMsg{
		latest: "v0.6.0", method: update.MethodArchive, path: "/usr/local/bin/awsx",
	})
	s.stage, s.err = updateDone, errors.New("permission denied")
	if !strings.Contains(s.View(), update.InstallerLine()) {
		t.Fatalf("view does not offer the installer: %q", s.View())
	}
}
