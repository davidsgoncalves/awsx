package aws

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// fakeSSMRunner returns a fixed SendCommand id, then walks the scripted
// invocation results one per GetCommandInvocation call, repeating the last.
// missingFor makes the first N invocation lookups report InvocationDoesNotExist.
type fakeSSMRunner struct {
	sendErr     error
	invocations []*ssm.GetCommandInvocationOutput
	invokeErr   error
	missingFor  int
	calls       int
	gotDocument string
	gotCommands []string
	gotTargets  []string
}

func (f *fakeSSMRunner) SendCommand(_ context.Context, in *ssm.SendCommandInput, _ ...func(*ssm.Options)) (*ssm.SendCommandOutput, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	if in.DocumentName != nil {
		f.gotDocument = *in.DocumentName
	}
	f.gotCommands = in.Parameters["commands"]
	f.gotTargets = in.InstanceIds
	id := "cmd-1"
	return &ssm.SendCommandOutput{Command: &ssmtypes.Command{CommandId: &id}}, nil
}

func (f *fakeSSMRunner) GetCommandInvocation(context.Context, *ssm.GetCommandInvocationInput, ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error) {
	if f.invokeErr != nil {
		return nil, f.invokeErr
	}
	i := f.calls
	f.calls++
	if i < f.missingFor {
		return nil, &ssmtypes.InvocationDoesNotExist{}
	}
	i -= f.missingFor
	if i >= len(f.invocations) {
		i = len(f.invocations) - 1
	}
	return f.invocations[i], nil
}

func invocation(status ssmtypes.CommandInvocationStatus, stdout, stderr string) *ssm.GetCommandInvocationOutput {
	return &ssm.GetCommandInvocationOutput{
		Status:                status,
		StandardOutputContent: &stdout,
		StandardErrorContent:  &stderr,
	}
}

func fastPoll(t *testing.T) {
	t.Helper()
	old := psPollInterval
	psPollInterval = time.Millisecond
	t.Cleanup(func() { psPollInterval = old })
}

func TestRunDockerPS_Success(t *testing.T) {
	fastPoll(t)
	f := &fakeSSMRunner{invocations: []*ssm.GetCommandInvocationOutput{
		invocation(ssmtypes.CommandInvocationStatusInProgress, "", ""),
		invocation(ssmtypes.CommandInvocationStatusSuccess, "abc\tmyapp-web-1\tweb\truby:3.2\tUp 3 days", ""),
	}}

	got, err := runDockerPS(context.Background(), f, "i-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Service != "web" {
		t.Fatalf("got %+v", got)
	}
	if f.gotDocument != "AWS-RunShellScript" {
		t.Fatalf("document = %q, want AWS-RunShellScript", f.gotDocument)
	}
	if len(f.gotCommands) != 1 || f.gotCommands[0] != dockerPSCommand {
		t.Fatalf("commands = %v, want [%s]", f.gotCommands, dockerPSCommand)
	}
	if len(f.gotTargets) != 1 || f.gotTargets[0] != "i-1" {
		t.Fatalf("targets = %v, want [i-1]", f.gotTargets)
	}
}

func TestRunDockerPS_FailedInvocationSurfacesStderr(t *testing.T) {
	fastPoll(t)
	f := &fakeSSMRunner{invocations: []*ssm.GetCommandInvocationOutput{
		invocation(ssmtypes.CommandInvocationStatusFailed, "", "docker: command not found"),
	}}

	_, err := runDockerPS(context.Background(), f, "i-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("err = %v, want ErrCommandFailed", err)
	}
	if got := err.Error(); !strings.Contains(got, "docker: command not found") {
		t.Fatalf("err %q does not carry the stderr", got)
	}
}

func TestRunDockerPS_SendCommandErrorPropagates(t *testing.T) {
	fastPoll(t)
	sentinel := errors.New("access denied")
	f := &fakeSSMRunner{sendErr: sentinel}

	if _, err := runDockerPS(context.Background(), f, "i-1"); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestRunDockerPS_TimesOutWhileInProgress(t *testing.T) {
	fastPoll(t)
	f := &fakeSSMRunner{invocations: []*ssm.GetCommandInvocationOutput{
		invocation(ssmtypes.CommandInvocationStatusInProgress, "", ""),
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := runDockerPS(ctx, f, "i-1"); !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("err = %v, want ErrCommandTimeout", err)
	}
}

// InvocationDoesNotExist is returned for a short window right after
// SendCommand; it must be retried, not treated as a failure.
func TestRunDockerPS_RetriesInvocationDoesNotExist(t *testing.T) {
	fastPoll(t)
	f := &fakeSSMRunner{
		missingFor: 2,
		invocations: []*ssm.GetCommandInvocationOutput{
			invocation(ssmtypes.CommandInvocationStatusSuccess, "abc\tweb-1\tweb\tnginx\tUp", ""),
		},
	}

	got, err := runDockerPS(context.Background(), f, "i-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d containers, want 1", len(got))
	}
	if f.calls != 3 {
		t.Fatalf("calls = %d, want 3 (two misses then success)", f.calls)
	}
}

var _ ContainerLister = (*Clients)(nil)
