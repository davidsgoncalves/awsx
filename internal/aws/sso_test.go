package aws

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
)

func writeTokenCache(t *testing.T, sessionName, body string) {
	t.Helper()
	path, err := ssocreds.StandardCachedTokenFilepath(sessionName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadSSOToken_Valid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	writeTokenCache(t, "vakinha", `{"accessToken":"tok-abc","expiresAt":"2026-07-21T13:00:00Z"}`)

	got, err := readSSOToken("vakinha", now)
	if err != nil {
		t.Fatal(err)
	}
	if got != "tok-abc" {
		t.Fatalf("token = %q, want tok-abc", got)
	}
}

func TestReadSSOToken_Expired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	writeTokenCache(t, "vakinha", `{"accessToken":"tok-abc","expiresAt":"2026-07-21T11:00:00Z"}`)

	_, err := readSSOToken("vakinha", now)
	if !errors.Is(err, ErrTokenExpiredOrMissing) {
		t.Fatalf("err = %v, want ErrTokenExpiredOrMissing", err)
	}
}

func TestReadSSOToken_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := readSSOToken("vakinha", time.Now())
	if !errors.Is(err, ErrTokenExpiredOrMissing) {
		t.Fatalf("err = %v, want ErrTokenExpiredOrMissing", err)
	}
}
