package aws

import "testing"

func TestJoin_OnlyRunningAndOnline_SortedByName(t *testing.T) {
	instances := []Instance{
		{ID: "i-3", Name: "worker", State: "running", Type: "t3.medium", PrivateIP: "10.0.1.22"},
		{ID: "i-1", Name: "api", State: "running", Type: "t3.large", PrivateIP: "10.0.1.15"},
		{ID: "i-2", Name: "stopped-box", State: "stopped", Type: "t3.small", PrivateIP: "10.0.1.9"},
		{ID: "i-4", Name: "no-ssm", State: "running", Type: "t3.small", PrivateIP: "10.0.1.30"},
	}
	online := map[string]bool{"i-1": true, "i-3": true, "i-4": false}

	got := Join(instances, online)

	if len(got) != 2 {
		t.Fatalf("want 2 targets, got %d: %+v", len(got), got)
	}
	if got[0].Name != "api" || got[1].Name != "worker" {
		t.Fatalf("unexpected order: %q, %q", got[0].Name, got[1].Name)
	}
	if !got[0].SSMOnline {
		t.Fatal("api should be SSMOnline")
	}
}

func TestDisplayName_FallsBackToID(t *testing.T) {
	if got := DisplayName(Instance{ID: "i-9"}); got != "i-9" {
		t.Fatalf("got %q, want i-9", got)
	}
	if got := DisplayName(Instance{ID: "i-9", Name: "api"}); got != "api" {
		t.Fatalf("got %q, want api", got)
	}
}
