package config

import (
	"errors"
	"testing"
)

func TestResolveRegion_EnvWins(t *testing.T) {
	t.Setenv("AWS_REGION", "ap-south-1")
	got, err := ResolveRegion("us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ap-south-1" {
		t.Fatalf("got %q, want ap-south-1", got)
	}
}

func TestResolveRegion_FallsBackToProfile(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	got, err := ResolveRegion("us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "us-east-1" {
		t.Fatalf("got %q, want us-east-1", got)
	}
}

func TestResolveRegion_NoneIsError(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	_, err := ResolveRegion("")
	if !errors.Is(err, ErrNoRegion) {
		t.Fatalf("got %v, want ErrNoRegion", err)
	}
}

func TestDebugEnabled(t *testing.T) {
	t.Setenv("AWSX_DEBUG", "true")
	if !DebugEnabled() {
		t.Fatal("want debug enabled")
	}
	t.Setenv("AWSX_DEBUG", "")
	if DebugEnabled() {
		t.Fatal("want debug disabled")
	}
}
