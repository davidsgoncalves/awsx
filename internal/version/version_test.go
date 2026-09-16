package version

import "testing"

func TestResolve_PrefersTheStampedVersion(t *testing.T) {
	if got := resolve("0.6.0", func() string { return "v0.1.0" }); got != "0.6.0" {
		t.Fatalf("got %q", got)
	}
}

func TestResolve_FallsBackToTheBuildInfo(t *testing.T) {
	if got := resolve("", func() string { return "v0.5.0" }); got != "v0.5.0" {
		t.Fatalf("got %q", got)
	}
}

func TestResolve_LocalBuildIsDev(t *testing.T) {
	if got := resolve("", func() string { return "(devel)" }); got != "dev" {
		t.Fatalf("got %q", got)
	}
	if got := resolve("", func() string { return "" }); got != "dev" {
		t.Fatalf("got %q", got)
	}
}
