package aws

import (
	"os"
	"strings"
	"testing"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

func TestWriteEphemeralProfile(t *testing.T) {
	session := profiles.SSOSession{
		Name:     "vakinha",
		StartURL: "https://example.awsapps.com/start",
		Region:   "us-east-1",
	}
	e, err := WriteEphemeralProfile(session, "947592431146", "SystemAdministrator", "sa-east-1")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if e.Profile != "_awsx" {
		t.Fatalf("profile = %q, want _awsx", e.Profile)
	}

	data, err := os.ReadFile(e.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"[sso-session vakinha]",
		"sso_start_url = https://example.awsapps.com/start",
		"[profile _awsx]",
		"sso_session = vakinha",
		"sso_account_id = 947592431146",
		"sso_role_name = SystemAdministrator",
		"region = sa-east-1",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("config missing %q:\n%s", want, content)
		}
	}
}

func TestEphemeral_CloseRemovesDir(t *testing.T) {
	e, err := WriteEphemeralProfile(profiles.SSOSession{Name: "x"}, "1", "r", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	path := e.ConfigPath
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file still exists after Close: %v", err)
	}
}
