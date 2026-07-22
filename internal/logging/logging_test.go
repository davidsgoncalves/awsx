package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestError_AlwaysWrites(t *testing.T) {
	var buf bytes.Buffer
	l := newWithWriter(&buf, false)
	l.Error("boom: %s", "start-session failed")
	if !strings.Contains(buf.String(), "start-session failed") {
		t.Fatalf("error not written: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "[ERROR]") {
		t.Fatalf("missing level: %q", buf.String())
	}
}

func TestDebug_OnlyWhenEnabled(t *testing.T) {
	var off bytes.Buffer
	newWithWriter(&off, false).Debug("verbose")
	if off.Len() != 0 {
		t.Fatalf("debug written while disabled: %q", off.String())
	}

	var on bytes.Buffer
	newWithWriter(&on, true).Debug("verbose")
	if !strings.Contains(on.String(), "verbose") {
		t.Fatalf("debug not written while enabled: %q", on.String())
	}
}

func TestRedact(t *testing.T) {
	cases := []string{
		"AWS_SECRET_ACCESS_KEY=abcd1234secret",
		"aws_session_token: verylongtokenvalue",
		"accessToken=xyz",
		"my key AKIA1234567890ABCDEF here",
	}
	leaks := []string{"abcd1234secret", "verylongtokenvalue", "xyz", "AKIA1234567890ABCDEF"}
	for i, c := range cases {
		got := redact(c)
		if strings.Contains(got, leaks[i]) {
			t.Fatalf("redact leaked %q: %q", leaks[i], got)
		}
	}
}
