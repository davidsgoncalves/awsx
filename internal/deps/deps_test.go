package deps

import (
	"strings"
	"testing"
)

func TestInstallHint_MacBrew(t *testing.T) {
	got := InstallHint("aws", "darwin", true)
	if !strings.Contains(got, "brew install awscli") {
		t.Fatalf("hint should suggest brew, got: %q", got)
	}
}

func TestInstallHint_MacNoBrew(t *testing.T) {
	got := InstallHint("aws", "darwin", false)
	if strings.Contains(got, "brew") {
		t.Fatalf("no-brew hint should not mention brew, got: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "awscli.amazonaws.com") &&
		!strings.Contains(strings.ToLower(got), "official") {
		t.Fatalf("hint should point to official installer, got: %q", got)
	}
}

func TestInstallHint_LinuxPlugin(t *testing.T) {
	got := InstallHint("session-manager-plugin", "linux", false)
	if !strings.Contains(strings.ToLower(got), "session-manager-plugin") {
		t.Fatalf("hint should mention the plugin, got: %q", got)
	}
}

func TestCheck_ReturnsBothDeps(t *testing.T) {
	got := Check()
	if len(got) != 2 {
		t.Fatalf("want 2 deps, got %d", len(got))
	}
	names := map[string]bool{got[0].Binary: true, got[1].Binary: true}
	if !names["aws"] || !names["session-manager-plugin"] {
		t.Fatalf("unexpected deps: %+v", got)
	}
}
