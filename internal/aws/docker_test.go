package aws

import (
	"strings"
	"testing"
)

func TestParsePS_ComposeContainers(t *testing.T) {
	out := strings.Join([]string{
		"abc123\tmyapp-web-1\tweb\truby:3.2\tUp 3 days",
		"def456\tmyapp-sidekiq-1\tsidekiq\truby:3.2\tUp 3 days",
	}, "\n")

	got := parsePS(out)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	want := Container{ID: "abc123", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
	if got[1].Service != "sidekiq" {
		t.Fatalf("second service = %q, want sidekiq", got[1].Service)
	}
}

func TestParsePS_NoComposeLabel(t *testing.T) {
	got := parsePS("abc123\tstandalone\t\tnginx:latest\tUp 1 hour")
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Service != "" {
		t.Fatalf("service = %q, want empty", got[0].Service)
	}
	if got[0].Name != "standalone" {
		t.Fatalf("name = %q, want standalone", got[0].Name)
	}
}

func TestParsePS_EmptyAndMalformed(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"whitespace":     "   \n\n  ",
		"too few fields": "abc123\tweb",
	}
	for name, in := range cases {
		if got := parsePS(in); len(got) != 0 {
			t.Fatalf("%s: got %d containers, want 0", name, len(got))
		}
	}
}

func TestParsePS_SkipsBadLinesKeepsGoodOnes(t *testing.T) {
	out := "garbage\nabc123\tmyapp-web-1\tweb\truby:3.2\tUp 3 days\n"
	got := parsePS(out)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "myapp-web-1" {
		t.Fatalf("name = %q", got[0].Name)
	}
}

func TestParsePS_TrimsCarriageReturns(t *testing.T) {
	got := parsePS("abc123\tmyapp-web-1\tweb\truby:3.2\tUp 3 days\r\n")
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Status != "Up 3 days" {
		t.Fatalf("status = %q, want %q", got[0].Status, "Up 3 days")
	}
}

func TestDisplayContainer(t *testing.T) {
	if got := DisplayContainer(Container{Name: "myapp-web-1", Service: "web"}); got != "web" {
		t.Fatalf("got %q, want web", got)
	}
	if got := DisplayContainer(Container{Name: "standalone"}); got != "standalone" {
		t.Fatalf("got %q, want standalone", got)
	}
}

func TestDockerExecLine(t *testing.T) {
	got := DockerExecLine("myapp-web-1", "rails c")
	want := "sudo docker exec -it myapp-web-1 rails c"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDockerExecLine_PreservesQuotesVerbatim(t *testing.T) {
	got := DockerExecLine("web", `rails runner "puts User.count"`)
	want := `sudo docker exec -it web rails runner "puts User.count"`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDockerPSCommandAsksForComposeServiceLabel(t *testing.T) {
	if !strings.Contains(dockerPSCommand, "com.docker.compose.service") {
		t.Fatalf("docker ps command does not request the compose service label: %q", dockerPSCommand)
	}
}
