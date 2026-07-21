package cli

import "testing"

func TestNewRootCmd_Use(t *testing.T) {
	cmd := NewRootCmd()
	if cmd.Use != "awsx" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "awsx")
	}
	if cmd.RunE == nil {
		t.Fatal("RunE is nil, want a runnable root")
	}
}
