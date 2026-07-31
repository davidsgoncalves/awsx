package state

import (
	"slices"
	"testing"
)

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

func TestCommandHistory_MostRecentFirst(t *testing.T) {
	s := Load()
	k := ContainerKey("web")

	s.PushCommand(k, "rails c")
	s.PushCommand(k, "bash")

	want := []string{"bash", "rails c"}
	if got := s.CommandHistory(k); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCommandHistory_DedupesAndPromotes(t *testing.T) {
	s := Load()
	k := ContainerKey("web")

	s.PushCommand(k, "rails c")
	s.PushCommand(k, "bash")
	s.PushCommand(k, "rails c")

	want := []string{"rails c", "bash"}
	if got := s.CommandHistory(k); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCommandHistory_CapsAtFive(t *testing.T) {
	s := Load()
	k := ContainerKey("web")
	for _, c := range []string{"c1", "c2", "c3", "c4", "c5", "c6"} {
		s.PushCommand(k, c)
	}

	want := []string{"c6", "c5", "c4", "c3", "c2"}
	if got := s.CommandHistory(k); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCommandHistory_IgnoresBlank(t *testing.T) {
	s := Load()
	k := ContainerKey("web")
	s.PushCommand(k, "   ")
	s.PushCommand(k, "")
	if got := s.CommandHistory(k); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestCommandHistory_SurvivesSaveLoad(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s := Load()
	s.PushCommand(ContainerKey("web"), "rails c")
	s.SetRegion(ProfileKey("prod"), "sa-east-1")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded := Load()
	if got := reloaded.CommandHistory(ContainerKey("web")); !slices.Equal(got, []string{"rails c"}) {
		t.Fatalf("history = %v, want [rails c]", got)
	}
	if got := reloaded.Region(ProfileKey("prod")); got != "sa-east-1" {
		t.Fatalf("region = %q, want sa-east-1", got)
	}
}

func TestContainerKeyDoesNotCollideWithRegionKeys(t *testing.T) {
	if ContainerKey("x") == ProfileKey("x") {
		t.Fatal("container and profile keys must not collide")
	}
}
