package state

import "testing"

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s := Load()
	if s.Region(ProfileKey("sysadmin")) != "" {
		t.Fatal("expected empty region on fresh state")
	}

	s.SetRegion(ProfileKey("sysadmin"), "sa-east-1")
	s.SetRegion(SSOKey("vakinha", "111", "Admin"), "us-east-1")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded := Load()
	if got := reloaded.Region(ProfileKey("sysadmin")); got != "sa-east-1" {
		t.Fatalf("profile region = %q, want sa-east-1", got)
	}
	if got := reloaded.Region(SSOKey("vakinha", "111", "Admin")); got != "us-east-1" {
		t.Fatalf("sso region = %q, want us-east-1", got)
	}
}

func TestKeysDiffer(t *testing.T) {
	if ProfileKey("x") == SSOKey("x", "y", "z") {
		t.Fatal("profile and sso keys must not collide")
	}
}
