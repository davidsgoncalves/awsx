package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParse_SSOFirstThenAlpha(t *testing.T) {
	body := `
[default]
region = us-east-1

[profile zeta]
region = eu-west-1

[profile alpha-sso]
sso_session = corp
region = sa-east-1

[profile beta]
region = us-west-2
`
	got, err := Parse(writeConfig(t, body))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha-sso", "beta", "default", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("pos %d = %q, want %q (full: %v)", i, got[i].Name, want[i], got)
		}
	}
	if !got[0].IsSSO {
		t.Fatal("alpha-sso should be flagged IsSSO")
	}
	if got[0].Region != "sa-east-1" {
		t.Fatalf("alpha-sso region = %q, want sa-east-1", got[0].Region)
	}
}

func TestParseSSOSessions(t *testing.T) {
	body := `
[profile p]
region = us-east-1

[sso-session vakinha]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1

[sso-session acme]
sso_start_url = https://acme.awsapps.com/start
sso_region = eu-west-1
`
	got, err := ParseSSOSessions(writeConfig(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sessions, got %d: %v", len(got), got)
	}
	// sorted by name: acme, vakinha
	if got[0].Name != "acme" || got[1].Name != "vakinha" {
		t.Fatalf("unexpected order: %v", got)
	}
	if got[1].StartURL != "https://example.awsapps.com/start" || got[1].Region != "us-east-1" {
		t.Fatalf("vakinha fields wrong: %+v", got[1])
	}
}

func TestParseSSOSessions_MissingFileReturnsEmpty(t *testing.T) {
	got, err := ParseSSOSessions("/does/not/exist/config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

func TestParse_MissingFileReturnsEmpty(t *testing.T) {
	got, err := Parse(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}
