package aws

import (
	"slices"
	"testing"
)

func TestLoginArgs(t *testing.T) {
	got := loginArgs("prod")
	want := []string{"sso", "login", "--profile", "prod"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSessionArgs(t *testing.T) {
	got := sessionArgs("prod", "i-abc")
	want := []string{"ssm", "start-session", "--profile", "prod", "--target", "i-abc"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
