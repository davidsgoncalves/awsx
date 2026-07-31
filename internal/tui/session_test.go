package tui

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// fakeSessioner records the interactive command it was asked to build.
type fakeSessioner struct {
	gotInstance string
	gotCommand  string
}

func (f *fakeSessioner) StartSession(string) error { return nil }

func (f *fakeSessioner) InteractiveCommand(instanceID, command string) *exec.Cmd {
	f.gotInstance, f.gotCommand = instanceID, command
	return exec.Command("true")
}

func TestInteractiveExec_UsesSessionerWhenAvailable(t *testing.T) {
	f := &fakeSessioner{}
	if cmd := interactiveExec(f, "i-1", "sudo docker exec -it web bash"); cmd == nil {
		t.Fatal("expected a command")
	}
	if f.gotInstance != "i-1" || f.gotCommand != "sudo docker exec -it web bash" {
		t.Fatalf("got instance=%q command=%q", f.gotInstance, f.gotCommand)
	}
}

// plainSessioner does not expose InteractiveCommand, exercising the fallback.
type plainSessioner struct{}

func (plainSessioner) StartSession(string) error { return nil }

func TestInteractiveExec_FallsBackToDirectAWSInvocation(t *testing.T) {
	cmd := interactiveExec(plainSessioner{}, "i-1", "bash")
	if !slices.Contains(cmd.Args, "AWS-StartInteractiveCommand") {
		t.Fatalf("fallback args missing document name: %v", cmd.Args)
	}
	if !slices.Contains(cmd.Args, "i-1") {
		t.Fatalf("fallback args missing target: %v", cmd.Args)
	}
}

type fakeContainerLister struct {
	cs  []awsx.Container
	err error
	got string
}

func (f *fakeContainerLister) Containers(_ context.Context, instanceID string) ([]awsx.Container, error) {
	f.got = instanceID
	return f.cs, f.err
}

func TestLoadContainersCmd_Success(t *testing.T) {
	f := &fakeContainerLister{cs: []awsx.Container{{Name: "myapp-web-1", Service: "web"}}}

	msg := loadContainersCmd(f, "i-1")()
	got, ok := msg.(containersMsg)
	if !ok {
		t.Fatalf("msg = %T, want containersMsg", msg)
	}
	if len(got.containers) != 1 || got.containers[0].Service != "web" {
		t.Fatalf("containers = %+v", got.containers)
	}
	if f.got != "i-1" {
		t.Fatalf("instance = %q, want i-1", f.got)
	}
}

func TestLoadContainersCmd_ErrorBecomesErrMsg(t *testing.T) {
	sentinel := errors.New("boom")
	msg := loadContainersCmd(&fakeContainerLister{err: sentinel}, "i-1")()

	got, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("msg = %T, want errMsg", msg)
	}
	if !errors.Is(got.err, sentinel) {
		t.Fatalf("err = %v, want %v", got.err, sentinel)
	}
}
