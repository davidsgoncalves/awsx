# Run a Command in a Container ("Rodar comando") Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fourth main-menu entry that picks an EC2 instance, lists its Docker containers, and opens an interactive `docker exec` inside the chosen one with a user-typed command.

**Architecture:** Container discovery goes through `ssm:SendCommand` running `docker ps` (no TTY needed, runs as root). Execution goes through `aws ssm start-session --document-name AWS-StartInteractiveCommand`, which hands a real TTY to the terminal via `tea.ExecProcess` — the same mechanism the existing SSM session and port-forward flows use. All command-string construction lives in one pure function so the on-screen preview and the executed command cannot diverge.

**Tech Stack:** Go 1.24.2, Bubble Tea 1.3.10, Bubbles 1.0.0 (`list`, `textinput`), AWS SDK v2 (`ssm`), AWS CLI v2 (shelled out).

## Global Constraints

- Go module: `github.com/davidsgoncalves/awsx`. The `internal/aws` package is imported as `awsx` everywhere in `internal/tui`.
- No new third-party dependencies. `textinput` is already available via `github.com/charmbracelet/bubbles v1.0.0`.
- All code comments and commit messages in English. All user-facing TUI strings in Portuguese, matching the existing screens.
- Tests are standard library only (`testing`, `slices`, `strings`) — no assertion libraries. Table tests where the project already uses them; otherwise one function per behavior, `t.Fatalf` with `got/want`.
- Lint: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused` (golangci-lint v2). Never leave an unchecked error return.
- Verification commands for every task: `go test ./...`, `go vet ./...`, `go build ./...`.
- New `screen` constants are appended at the end of the `const` block in `internal/tui/screen_error.go`, never inserted — existing tests depend on the numeric values of the earlier states.
- The interactive command is always `sudo docker exec -it <container> <command>`. Never `sudo su - <user>`, never `docker compose exec`.

---

### Task 1: Container type and pure command builders

**Files:**
- Modify: `internal/aws/types.go` (append after the `RDSLister` block, around line 60)
- Create: `internal/aws/docker.go`
- Test: `internal/aws/docker_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `aws.Container` struct with fields `ID, Name, Service, Image, Status string`
  - `aws.ContainerLister` interface with `Containers(ctx context.Context, instanceID string) ([]Container, error)`
  - `aws.DisplayContainer(c Container) string` — the Compose service, or the container name when the service is empty
  - `aws.DockerExecLine(container, command string) string`
  - unexported `parsePS(out string) []Container`
  - unexported const `dockerPSCommand string`

- [ ] **Step 1: Write the failing test**

Create `internal/aws/docker_test.go`:

```go
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
		"empty":         "",
		"whitespace":    "   \n\n  ",
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/aws/ -run 'ParsePS|DisplayContainer|DockerExec|DockerPS' -v`
Expected: FAIL to compile — `undefined: parsePS`, `undefined: Container`, `undefined: DisplayContainer`, `undefined: DockerExecLine`, `undefined: dockerPSCommand`.

- [ ] **Step 3: Add the type and interface**

Append to `internal/aws/types.go`, after the `RDSLister` interface:

```go
// Container is a Docker container running on an EC2 instance. Service is the
// Compose service label (com.docker.compose.service) and is empty for
// containers not managed by Compose.
type Container struct {
	ID      string
	Name    string
	Service string
	Image   string
	Status  string
}

// ContainerLister lists the Docker containers running on an instance.
type ContainerLister interface {
	Containers(ctx context.Context, instanceID string) ([]Container, error)
}
```

- [ ] **Step 4: Write the pure builders**

Create `internal/aws/docker.go`:

```go
package aws

import "strings"

// dockerPSCommand lists running containers in a tab-separated form that
// parsePS understands. The Compose service label gives a readable name for
// containers started by docker compose, whose container names carry a
// Compose-assigned numeric suffix.
const dockerPSCommand = `docker ps --format '{{.ID}}\t{{.Names}}\t{{.Label "com.docker.compose.service"}}\t{{.Image}}\t{{.Status}}'`

// psFields is the number of tab-separated fields dockerPSCommand emits.
const psFields = 5

// parsePS turns the output of dockerPSCommand into containers, skipping lines
// that do not have the expected field count.
func parsePS(out string) []Container {
	var cs []Container
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != psFields {
			continue
		}
		cs = append(cs, Container{
			ID:      f[0],
			Name:    f[1],
			Service: f[2],
			Image:   f[3],
			Status:  f[4],
		})
	}
	return cs
}

// DisplayContainer returns the Compose service name, falling back to the
// container name.
func DisplayContainer(c Container) string {
	if c.Service != "" {
		return c.Service
	}
	return c.Name
}

// DockerExecLine builds the shell line that runs command inside container.
// sudo is required because the SSM session runs as ssm-user, which is not in
// the docker group and cannot reach /var/run/docker.sock. This is the single
// source of truth for both the on-screen preview and the executed command.
func DockerExecLine(container, command string) string {
	return "sudo docker exec -it " + container + " " + command
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/aws/ -run 'ParsePS|DisplayContainer|DockerExec|DockerPS' -v`
Expected: PASS (9 tests)

- [ ] **Step 6: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add internal/aws/types.go internal/aws/docker.go internal/aws/docker_test.go
git commit -m "feat: add Container type and pure docker command builders

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: AWS-StartInteractiveCommand invocation

**Files:**
- Modify: `internal/aws/exec.go` (add `interactiveCommandArgs` next to `portForwardArgs` around line 36; add `InteractiveCommand` next to `PortForwardCommand` around line 96)
- Test: `internal/aws/exec_test.go` (append)

**Interfaces:**
- Consumes: nothing from Task 1 (independent).
- Produces:
  - unexported `interactiveCommandArgs(profile, region, instanceID, command string) []string`
  - `func (c CLI) InteractiveCommand(instanceID, command string) *exec.Cmd`

- [ ] **Step 1: Write the failing test**

Append to `internal/aws/exec_test.go`:

```go
func TestInteractiveCommandArgs(t *testing.T) {
	got := interactiveCommandArgs("prod", "sa-east-1", "i-1", "sudo docker exec -it web rails c")
	want := []string{
		"ssm", "start-session", "--profile", "prod", "--region", "sa-east-1",
		"--target", "i-1",
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", `{"command":["sudo docker exec -it web rails c"]}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInteractiveCommandArgs_NoRegion(t *testing.T) {
	got := interactiveCommandArgs("prod", "", "i-1", "bash")
	want := []string{
		"ssm", "start-session", "--profile", "prod",
		"--target", "i-1",
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", `{"command":["bash"]}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A comma in the command would be read as a list separator by the AWS CLI's
// key=value shorthand, silently splitting it into two parameters. JSON encoding
// is what keeps the command intact.
func TestInteractiveCommandArgs_CommandWithCommaStaysOneParameter(t *testing.T) {
	got := interactiveCommandArgs("prod", "", "i-1", `rails runner "puts [1,2]"`)
	want := `{"command":["rails runner \"puts [1,2]\""]}`
	if got[len(got)-1] != want {
		t.Fatalf("parameters = %s, want %s", got[len(got)-1], want)
	}
}

func TestInteractiveCommand_WithConfigFile(t *testing.T) {
	c := CLI{Profile: "_awsx", ConfigFile: "/tmp/awsx/config"}
	cmd := c.InteractiveCommand("i-1", "bash")

	if !slices.Contains(cmd.Args, "AWS-StartInteractiveCommand") {
		t.Fatalf("args missing document name: %v", cmd.Args)
	}
	if !slices.Contains(cmd.Env, "AWS_CONFIG_FILE=/tmp/awsx/config") {
		t.Fatalf("AWS_CONFIG_FILE not set in env: %v", cmd.Env)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/aws/ -run InteractiveCommand -v`
Expected: FAIL to compile — `undefined: interactiveCommandArgs`, `c.InteractiveCommand undefined`.

- [ ] **Step 3: Write the implementation**

In `internal/aws/exec.go`, add `"encoding/json"` to the import block, then add after `portForwardArgs`:

```go
// interactiveCommandArgs builds the start-session invocation that runs a single
// command with a TTY attached. Parameters are JSON-encoded rather than using
// the AWS CLI key=value shorthand: the shorthand splits values on commas, which
// would silently break any command containing one.
func interactiveCommandArgs(profile, region, instanceID, command string) []string {
	args := []string{"ssm", "start-session", "--profile", profile}
	if region != "" {
		args = append(args, "--region", region)
	}
	params, err := json.Marshal(map[string][]string{"command": {command}})
	if err != nil {
		// Marshalling a map of strings cannot fail; fall back to the raw
		// command rather than dropping the parameter entirely.
		params = []byte(`{"command":[""]}`)
	}
	return append(args,
		"--target", instanceID,
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", string(params),
	)
}
```

and after `PortForwardCommand`:

```go
// InteractiveCommand builds an SSM start-session command that runs command on
// instanceID with a TTY, with the terminal wired to the current process for use
// with tea.ExecProcess.
func (c CLI) InteractiveCommand(instanceID, command string) *exec.Cmd {
	cmd := exec.Command("aws", interactiveCommandArgs(c.Profile, c.Region, instanceID, command)...)
	cmd.Env = c.env()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/aws/ -run InteractiveCommand -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/aws/exec.go internal/aws/exec_test.go
git commit -m "feat: add AWS-StartInteractiveCommand invocation

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Remote container listing via SendCommand

**Files:**
- Create: `internal/aws/containers.go`
- Test: `internal/aws/containers_test.go`

**Interfaces:**
- Consumes: `Container`, `parsePS`, `dockerPSCommand` (Task 1).
- Produces:
  - `func (c *Clients) Containers(ctx context.Context, instanceID string) ([]Container, error)` — satisfies `ContainerLister`
  - unexported `ssmRunner` interface, `runDockerPS(ctx, ssmRunner, instanceID) ([]Container, error)`, `var psPollInterval time.Duration`
  - `var ErrCommandFailed error`, `var ErrCommandTimeout error`

- [ ] **Step 1: Write the failing test**

Create `internal/aws/containers_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/aws/ -run RunDockerPS -v`
Expected: FAIL to compile — `undefined: runDockerPS`, `undefined: psPollInterval`, `undefined: ErrCommandFailed`, `undefined: ErrCommandTimeout`, and `*Clients does not implement ContainerLister`.

- [ ] **Step 3: Write the implementation**

Create `internal/aws/containers.go`:

```go
package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Errors returned by the remote docker ps run.
var (
	// ErrCommandFailed means the command ran but exited non-zero.
	ErrCommandFailed = errors.New("remote command failed")
	// ErrCommandTimeout means the invocation never reached a terminal state.
	ErrCommandTimeout = errors.New("remote command timed out")
)

// psPollInterval is how often the invocation result is polled. It is a variable
// so tests can shorten it.
var psPollInterval = 500 * time.Millisecond

// psTimeout bounds the whole send-and-poll cycle.
const psTimeout = 20 * time.Second

// ssmRunner is the subset of the SSM API used to run a shell command on an
// instance. *ssm.Client satisfies it.
type ssmRunner interface {
	SendCommand(ctx context.Context, in *ssm.SendCommandInput, opts ...func(*ssm.Options)) (*ssm.SendCommandOutput, error)
	GetCommandInvocation(ctx context.Context, in *ssm.GetCommandInvocationInput, opts ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error)
}

// Containers implements ContainerLister. It runs docker ps on the instance
// through SSM and parses the result. SendCommand runs as root, so no sudo is
// needed here — unlike the interactive docker exec, which runs as ssm-user.
func (c *Clients) Containers(ctx context.Context, instanceID string) ([]Container, error) {
	ctx, cancel := context.WithTimeout(ctx, psTimeout)
	defer cancel()
	return runDockerPS(ctx, c.ssm, instanceID)
}

// runDockerPS sends the docker ps command and polls until the invocation
// reaches a terminal state or ctx expires.
func runDockerPS(ctx context.Context, r ssmRunner, instanceID string) ([]Container, error) {
	out, err := r.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: ptr("AWS-RunShellScript"),
		InstanceIds:  []string{instanceID},
		Parameters:   map[string][]string{"commands": {dockerPSCommand}},
	})
	if err != nil {
		return nil, err
	}
	if out.Command == nil || out.Command.CommandId == nil {
		return nil, fmt.Errorf("%w: no command id returned", ErrCommandFailed)
	}
	commandID := *out.Command.CommandId

	for {
		inv, err := r.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandId:  &commandID,
			InstanceId: &instanceID,
		})
		switch {
		case err == nil:
			switch inv.Status {
			case ssmtypes.CommandInvocationStatusPending, ssmtypes.CommandInvocationStatusInProgress,
				ssmtypes.CommandInvocationStatusDelayed:
				// keep polling
			case ssmtypes.CommandInvocationStatusSuccess:
				return parsePS(deref(inv.StandardOutputContent)), nil
			default:
				detail := deref(inv.StandardErrorContent)
				if detail == "" {
					detail = string(inv.Status)
				}
				return nil, fmt.Errorf("%w: %s", ErrCommandFailed, detail)
			}
		case isInvocationMissing(err):
			// The invocation is not registered yet; retry.
		default:
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ErrCommandTimeout
		case <-time.After(psPollInterval):
		}
	}
}

// isInvocationMissing reports the short window after SendCommand during which
// the invocation is not yet queryable.
func isInvocationMissing(err error) bool {
	var missing *ssmtypes.InvocationDoesNotExist
	return errors.As(err, &missing)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/aws/ -run RunDockerPS -v`
Expected: PASS (5 tests)

- [ ] **Step 5: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/aws/containers.go internal/aws/containers_test.go
git commit -m "feat: list remote docker containers via ssm:SendCommand

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Command history in persisted state

**Files:**
- Modify: `internal/state/state.go`
- Test: `internal/state/state_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `state.ContainerKey(service string) string`
  - `func (s *State) CommandHistory(key string) []string`
  - `func (s *State) PushCommand(key, command string)`
  - `State.Commands map[string][]string` JSON field `commands`
  - const `maxCommandHistory = 5`

- [ ] **Step 1: Write the failing test**

Append to `internal/state/state_test.go`:

```go
import "slices" // add to the existing import block alongside "testing"

func TestCommandHistory_MostRecentFirst(t *testing.T) {
	s := Load()
	k := ContainerKey("web")

	s.PushCommand(k, "rails c")
	s.PushCommand(k, "bash")

	want := []string{"bash", "rails c"}
	if got := s.CommandHistory(k); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCommandHistory_DedupesAndPromotes(t *testing.T) {
	s := Load()
	k := ContainerKey("web")

	s.PushCommand(k, "rails c")
	s.PushCommand(k, "bash")
	s.PushCommand(k, "rails c")

	want := []string{"rails c", "bash"}
	if got := s.CommandHistory(k); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCommandHistory_CapsAtFive(t *testing.T) {
	s := Load()
	k := ContainerKey("web")
	for _, c := range []string{"c1", "c2", "c3", "c4", "c5", "c6"} {
		s.PushCommand(k, c)
	}

	want := []string{"c6", "c5", "c4", "c3", "c2"}
	if got := s.CommandHistory(k); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCommandHistory_IgnoresBlank(t *testing.T) {
	s := Load()
	k := ContainerKey("web")
	s.PushCommand(k, "   ")
	s.PushCommand(k, "")
	if got := s.CommandHistory(k); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestCommandHistory_SurvivesSaveLoad(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s := Load()
	s.PushCommand(ContainerKey("web"), "rails c")
	s.SetRegion(ProfileKey("prod"), "sa-east-1")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded := Load()
	if got := reloaded.CommandHistory(ContainerKey("web")); !slices.Equal(got, []string{"rails c"}) {
		t.Fatalf("history = %v, want [rails c]", got)
	}
	if got := reloaded.Region(ProfileKey("prod")); got != "sa-east-1" {
		t.Fatalf("region = %q, want sa-east-1", got)
	}
}

func TestContainerKeyDoesNotCollideWithRegionKeys(t *testing.T) {
	if ContainerKey("x") == ProfileKey("x") {
		t.Fatal("container and profile keys must not collide")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -v`
Expected: FAIL to compile — `undefined: ContainerKey`, `s.PushCommand undefined`, `s.CommandHistory undefined`.

- [ ] **Step 3: Write the implementation**

In `internal/state/state.go`, add `"strings"` to the imports, extend the struct and `Load`, and append the new methods:

```go
// State is the persisted preference set.
type State struct {
	Regions  map[string]string   `json:"regions"`
	Commands map[string][]string `json:"commands"`
}
```

In `Load`, initialize and repair both maps:

```go
func Load() *State {
	s := &State{Regions: map[string]string{}, Commands: map[string][]string{}}
	data, err := os.ReadFile(Path())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, s)
	if s.Regions == nil {
		s.Regions = map[string]string{}
	}
	if s.Commands == nil {
		s.Commands = map[string][]string{}
	}
	return s
}
```

Append at the end of the file:

```go
// maxCommandHistory caps how many past commands are remembered per container.
const maxCommandHistory = 5

// ContainerKey is the state key for a container's command history. It is keyed
// by the Compose service (or container name when there is none) rather than the
// container name, because Compose appends an instance suffix that changes when
// the container is recreated.
func ContainerKey(service string) string { return "container:" + service }

// CommandHistory returns the remembered commands for key, most recent first.
func (s *State) CommandHistory(key string) []string { return s.Commands[key] }

// PushCommand records command as the most recent one for key, removing any
// earlier occurrence and keeping at most maxCommandHistory entries. Blank
// commands are ignored.
func (s *State) PushCommand(key, command string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}
	if s.Commands == nil {
		s.Commands = map[string][]string{}
	}
	history := []string{command}
	for _, c := range s.Commands[key] {
		if c == command {
			continue
		}
		history = append(history, c)
		if len(history) == maxCommandHistory {
			break
		}
	}
	s.Commands[key] = history
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/state/ -v`
Expected: PASS (all, including the two pre-existing region tests)

- [ ] **Step 5: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "feat: remember the last commands run per container

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Replace the tunneling boolean with a flow enum

**Files:**
- Modify: `internal/tui/root.go` (field declaration ~line 63; `targetsMsg` case ~line 141; `sessionEndedMsg` case ~line 200; `screenMenu` routing ~line 300; `screenTunnelInstance` routing)
- Modify: `internal/tui/root_test.go` (`TestRoot_TunnelFlow_MenuToRDSToInstance`, `TestRoot_NoRDSShowsGuidance`)

**Interfaces:**
- Consumes: nothing.
- Produces: `flow` type with `flowSession`, `flowTunnel`, `flowExec` constants; `rootModel.flow` field replacing `rootModel.tunneling`.

**Note:** pure refactor. No behavior changes, no new screens. `flowExec` is declared here but not yet reachable — Task 6 wires it.

- [ ] **Step 1: Update the two tests that read the boolean**

In `internal/tui/root_test.go`, `TestRoot_TunnelFlow_MenuToRDSToInstance`, replace:

```go
	if !m.tunneling {
		t.Fatal("expected tunneling true")
	}
```

with:

```go
	if m.flow != flowTunnel {
		t.Fatalf("flow = %v, want flowTunnel", m.flow)
	}
```

In `TestRoot_NoRDSShowsGuidance`, replace `m.tunneling = true` with `m.flow = flowTunnel`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TunnelFlow|NoRDS' -v`
Expected: FAIL to compile — `undefined: flowTunnel`, `m.flow undefined`.

- [ ] **Step 3: Introduce the enum and swap the field**

In `internal/tui/root.go`, add above `type rootModel struct`:

```go
// flow is which main-menu action is in progress. It decides how the shared
// instance-picker screen is labelled and where the flow goes next.
type flow int

const (
	flowSession flow = iota // Acessar EC2
	flowTunnel              // Acessar banco/serviço (túnel)
	flowExec                // Rodar comando
)
```

Replace the `tunneling bool` field with `flow flow` (keep it in the same block, renaming the comment to `// Flow state.`):

```go
	// Flow state.
	flow           flow
	tunnelInstance awsx.Target
	tunnelDB       awsx.RDSInstance
```

- [ ] **Step 4: Update every read and write**

In the `targetsMsg` case:

```go
		if m.flow == flowTunnel {
			m.instancesScreen.list.Title = "Escolha a instância que fará o túnel (bastion SSM)"
			m.current = screenTunnelInstance
		} else {
			m.current = screenInstances
		}
```

In the `sessionEndedMsg` case:

```go
		back := errorAction{label: "Voltar para as instâncias", next: screenInstances}
		if m.flow == flowTunnel {
			back = errorAction{label: "Voltar para os bancos", next: screenRDS}
		}
```

and the two `if m.tunneling` title overrides in the same case become `if m.flow == flowTunnel`.

In the `screenMenu` routing case, replace `m.tunneling = false` with `m.flow = flowSession` and `m.tunneling = true` with `m.flow = flowTunnel`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/ -v`
Expected: PASS (all existing tests)

- [ ] **Step 6: Confirm the old field is gone**

Run: `grep -rn "tunneling" internal/`
Expected: no output

- [ ] **Step 7: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 8: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "refactor: replace tunneling bool with a flow enum

A third menu action makes a boolean unable to express the state that
decides the instance-picker title, the session-failure title, and the
back destination.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Fourth menu entry and new screen states

**Files:**
- Modify: `internal/tui/screen_error.go` (append to the `const` block, line 26)
- Modify: `internal/tui/screen_menu.go` (`menuActions`, `Update`)
- Modify: `internal/tui/screen_menu_test.go`
- Modify: `internal/tui/root_test.go` (`TestRoot_SSOFlow_SessionToMenu` — the quit navigation)

**Interfaces:**
- Consumes: nothing.
- Produces: `screenExecInstance`, `screenContainers`, `screenCommand` constants; `menuActions` with four entries; menu cursor 2 returns `screenExecInstance`, cursor 3 returns `screenQuit`.

**Note:** two existing tests walk the menu with `KeyDown` to reach "Sair". A fourth entry means one more `KeyDown` in each. Fixing them is part of this task.

- [ ] **Step 1: Write the failing test**

Replace `TestMenuScreen_SelectEC2AndQuit` in `internal/tui/screen_menu_test.go` with:

```go
func TestMenuScreen_SelectsEachAction(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"})

	_, next := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cursor 0 = Acessar EC2
	if next != screenInstances {
		t.Fatalf("next = %v, want screenInstances", next)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 1 = túnel
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenRDS {
		t.Fatalf("next = %v, want screenRDS", next)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 2 = Rodar comando
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenExecInstance {
		t.Fatalf("next = %v, want screenExecInstance", next)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor 3 = Sair
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenQuit {
		t.Fatalf("next = %v, want screenQuit", next)
	}
}

func TestMenuScreen_ViewListsRunCommand(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"})
	if !strings.Contains(m.View(), "Rodar comando") {
		t.Fatalf("menu does not list the run-command action: %q", m.View())
	}
}

func TestMenuScreen_CursorStopsAtLastAction(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"})
	for range 10 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, next := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenQuit {
		t.Fatalf("next = %v, want screenQuit", next)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run MenuScreen -v`
Expected: FAIL to compile — `undefined: screenExecInstance`.

- [ ] **Step 3: Add the screen constants**

In `internal/tui/screen_error.go`, append inside the existing `const` block, after `screenTunnelInstance`:

```go
	screenExecInstance
	screenContainers
	screenCommand
```

- [ ] **Step 4: Add the menu entry**

In `internal/tui/screen_menu.go`:

```go
var menuActions = []string{"Acessar EC2", "Acessar banco/serviço (túnel)", "Rodar comando", "Sair"}
```

and in `Update`, the `tea.KeyEnter` case:

```go
	case tea.KeyEnter:
		switch m.cursor {
		case 0:
			return m, screenInstances
		case 1:
			return m, screenRDS
		case 2:
			return m, screenExecInstance
		default:
			return m, screenQuit
		}
```

- [ ] **Step 5: Run the menu tests to verify they pass**

Run: `go test ./internal/tui/ -run MenuScreen -v`
Expected: PASS (3 tests)

- [ ] **Step 6: Fix the SSO flow test's quit navigation**

Run: `go test ./internal/tui/ -run SSOFlow_SessionToMenu -v`
Expected: FAIL — cleanup was not called on quit (two `KeyDown` now land on "Rodar comando", not "Sair").

In `internal/tui/root_test.go`, `TestRoot_SSOFlow_SessionToMenu`, replace the final navigation block:

```go
	// quit from menu -> cleanup runs (menu has 4 items; Sair is the fourth)
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !cleaned {
		t.Fatal("cleanup was not called on quit")
	}
```

- [ ] **Step 7: Run all TUI tests**

Run: `go test ./internal/tui/ -v`
Expected: PASS

- [ ] **Step 8: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 9: Commit**

```bash
git add internal/tui/screen_error.go internal/tui/screen_menu.go internal/tui/screen_menu_test.go internal/tui/root_test.go
git commit -m "feat: add 'Rodar comando' to the main menu

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Container picker screen

**Files:**
- Modify: `internal/tui/screen_pick.go` (append a `// --- containers ---` section, following the accounts/roles/rds/regions shape)
- Test: `internal/tui/screen_pick_test.go` (append)

**Interfaces:**
- Consumes: `awsx.Container`, `awsx.DisplayContainer` (Task 1).
- Produces:
  - `containersScreen` struct with field `list list.Model`
  - `newContainersScreen(cs []awsx.Container) containersScreen`
  - `func (s containersScreen) Update(msg tea.Msg) (containersScreen, *awsx.Container, tea.Cmd)`
  - `func (s containersScreen) View() string`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/screen_pick_test.go`:

```go
func TestContainersScreen_SelectReturnsContainer(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
		{ID: "def", Name: "myapp-sidekiq-1", Service: "sidekiq", Image: "ruby:3.2", Status: "Up 3 days"},
	})
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil {
		t.Fatal("expected a selection")
	}
	if sel.Name != "myapp-web-1" {
		t.Fatalf("selected %q, want myapp-web-1", sel.Name)
	}
}

func TestContainersScreen_TitleUsesComposeServiceThenName(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{Name: "myapp-web-1", Service: "web"},
		{Name: "standalone"},
	})
	items := s.list.Items()

	if got := items[0].(containerItem).Title(); got != "web" {
		t.Fatalf("first title = %q, want web", got)
	}
	if got := items[1].(containerItem).Title(); got != "standalone" {
		t.Fatalf("second title = %q, want standalone", got)
	}
}

func TestContainersScreen_DescriptionShowsNameImageStatus(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
	})
	got := s.list.Items()[0].(containerItem).Description()
	for _, want := range []string{"myapp-web-1", "ruby:3.2", "Up 3 days"} {
		if !strings.Contains(got, want) {
			t.Fatalf("description %q missing %q", got, want)
		}
	}
}

func TestContainersScreen_FilterMatchesServiceNameAndImage(t *testing.T) {
	s := newContainersScreen([]awsx.Container{
		{Name: "myapp-web-1", Service: "web", Image: "ruby:3.2"},
	})
	got := s.list.Items()[0].(containerItem).FilterValue()
	for _, want := range []string{"web", "myapp-web-1", "ruby:3.2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("filter value %q missing %q", got, want)
		}
	}
}
```

If `internal/tui/screen_pick_test.go` does not already import `strings`, add it to the import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run ContainersScreen -v`
Expected: FAIL to compile — `undefined: newContainersScreen`, `undefined: containerItem`.

- [ ] **Step 3: Write the implementation**

Append to `internal/tui/screen_pick.go`:

```go
// --- containers ---

type containerItem struct{ c awsx.Container }

func (i containerItem) Title() string { return awsx.DisplayContainer(i.c) }
func (i containerItem) Description() string {
	return fmt.Sprintf("%s   %s   %s", i.c.Name, i.c.Image, i.c.Status)
}
func (i containerItem) FilterValue() string {
	return i.c.Service + " " + i.c.Name + " " + i.c.Image
}

type containersScreen struct{ list list.Model }

func newContainersScreen(cs []awsx.Container) containersScreen {
	items := make([]list.Item, len(cs))
	for i, c := range cs {
		items[i] = containerItem{c: c}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione um container"
	return containersScreen{list: l}
}

func (s containersScreen) Update(msg tea.Msg) (containersScreen, *awsx.Container, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(containerItem); ok {
			c := it.c
			return s, &c, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s containersScreen) View() string { return s.list.View() }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run ContainersScreen -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen_pick.go internal/tui/screen_pick_test.go
git commit -m "feat: add the container picker screen

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Command input screen

**Files:**
- Create: `internal/tui/screen_command.go`
- Test: `internal/tui/screen_command_test.go`

**Interfaces:**
- Consumes: `awsx.Container`, `awsx.DockerExecLine`, `awsx.DisplayContainer` (Task 1).
- Produces:
  - `commandScreen` struct
  - `newCommandScreen(c awsx.Container, history []string) commandScreen`
  - `newManualCommandScreen(instanceName string) commandScreen`
  - `func (s commandScreen) Update(msg tea.Msg) (commandScreen, *commandSubmit, tea.Cmd)`
  - `func (s commandScreen) View() string`
  - `func (s commandScreen) line() string` — the full command line currently shown
  - `type commandSubmit struct{ Line, Inner string }` — `Line` is what executes, `Inner` is what goes into history (empty in full-line mode)
  - `func (s commandScreen) fullLineMode() bool`

**Behavior:**
- Two modes. In **inner mode** the input holds the in-container command and the full line is derived via `DockerExecLine`. In **full-line mode** the input holds the whole line and is used verbatim.
- `tab` toggles. Entering full-line mode seeds the input with the current derived line; returning to inner mode restores the previous inner value.
- `↑`/`↓` walk history in inner mode only.
- `enter` on a blank input does nothing.
- `esc` is handled by the caller (`root.go`), not here.
- **Manual mode** (`newManualCommandScreen`) is the fallback used when the container list could not be fetched: it starts in full-line mode with no container and no history, and `tab` is inert — there is no in-container mode to switch to, because no container was ever chosen.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/screen_command_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

var webContainer = awsx.Container{
	ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days",
}

// typeText feeds each rune of s to the screen as a key message.
func typeText(s commandScreen, text string) commandScreen {
	for _, r := range text {
		s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return s
}

func TestCommandScreen_PrefillsMostRecentHistoryEntry(t *testing.T) {
	s := newCommandScreen(webContainer, []string{"rails c", "bash"})
	if got := s.line(); got != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q", got)
	}
}

func TestCommandScreen_EmptyHistoryStartsBlank(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub != nil {
		t.Fatalf("expected no submit on a blank input, got %+v", sub)
	}
}

func TestCommandScreen_SubmitsDerivedLineAndInnerCommand(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "rails c")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Line != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q", sub.Line)
	}
	if sub.Inner != "rails c" {
		t.Fatalf("inner = %q, want 'rails c'", sub.Inner)
	}
}

func TestCommandScreen_PreviewMatchesDockerExecLine(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "bash")
	if !strings.Contains(s.View(), awsx.DockerExecLine("myapp-web-1", "bash")) {
		t.Fatalf("view does not show the derived line: %q", s.View())
	}
}

func TestCommandScreen_ViewNamesServiceAndContainer(t *testing.T) {
	v := newCommandScreen(webContainer, nil).View()
	if !strings.Contains(v, "web") || !strings.Contains(v, "myapp-web-1") {
		t.Fatalf("view missing container identity: %q", v)
	}
}

func TestCommandScreen_TabSeedsFullLineMode(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "rails c")

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !s.fullLineMode() {
		t.Fatal("expected full-line mode after tab")
	}
	if got := s.line(); got != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("seeded line = %q", got)
	}
}

func TestCommandScreen_FullLineModeSubmitsVerbatimWithNoHistory(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = typeText(s, "docker logs -f web")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Line != "docker logs -f web" {
		t.Fatalf("line = %q, want verbatim", sub.Line)
	}
	if sub.Inner != "" {
		t.Fatalf("inner = %q, want empty in full-line mode", sub.Inner)
	}
}

func TestCommandScreen_TabBackRestoresInnerValue(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "rails c")

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab}) // to full line
	s = typeText(s, " --sandbox")
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab}) // back to inner

	if s.fullLineMode() {
		t.Fatal("expected inner mode after the second tab")
	}
	if got := s.line(); got != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q, want the original inner command", got)
	}
}

func TestCommandScreen_HistoryNavigation(t *testing.T) {
	s := newCommandScreen(webContainer, []string{"rails c", "bash", "sh"})

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown}) // -> bash
	if got := s.line(); got != awsx.DockerExecLine("myapp-web-1", "bash") {
		t.Fatalf("after down: %q", got)
	}

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown}) // -> sh
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown}) // clamped at the last entry
	if got := s.line(); got != awsx.DockerExecLine("myapp-web-1", "sh") {
		t.Fatalf("after clamping down: %q", got)
	}

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp}) // -> bash
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp}) // -> rails c
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp}) // clamped at the first entry
	if got := s.line(); got != awsx.DockerExecLine("myapp-web-1", "rails c") {
		t.Fatalf("after clamping up: %q", got)
	}
}

func TestCommandScreen_HistoryIgnoredInFullLineMode(t *testing.T) {
	s := newCommandScreen(webContainer, []string{"rails c", "bash"})
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})

	before := s.line()
	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	if s.line() != before {
		t.Fatalf("history moved in full-line mode: %q -> %q", before, s.line())
	}
}

func TestCommandScreen_TrimsWhitespaceOnSubmit(t *testing.T) {
	s := newCommandScreen(webContainer, nil)
	s = typeText(s, "  rails c  ")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Inner != "rails c" {
		t.Fatalf("inner = %q, want trimmed", sub.Inner)
	}
	if sub.Line != "sudo docker exec -it myapp-web-1 rails c" {
		t.Fatalf("line = %q", sub.Line)
	}
}

func TestCommandScreen_ViewShowsKeyHints(t *testing.T) {
	v := newCommandScreen(webContainer, nil).View()
	for _, want := range []string{"enter", "tab", "esc"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view missing %q hint: %q", want, v)
		}
	}
}

func TestManualCommandScreen_StartsInFullLineModeAndNamesInstance(t *testing.T) {
	s := newManualCommandScreen("api")
	if !s.fullLineMode() {
		t.Fatal("expected full-line mode")
	}
	if !strings.Contains(s.View(), "api") {
		t.Fatalf("view does not name the instance: %q", s.View())
	}
}

func TestManualCommandScreen_TabIsInert(t *testing.T) {
	s := newManualCommandScreen("api")
	s = typeText(s, "sudo docker exec -it web bash")

	s, _, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !s.fullLineMode() {
		t.Fatal("tab must not leave full-line mode when no container was chosen")
	}
	if got := s.line(); got != "sudo docker exec -it web bash" {
		t.Fatalf("line = %q, want unchanged", got)
	}
}

func TestManualCommandScreen_SubmitsVerbatimWithNoHistory(t *testing.T) {
	s := newManualCommandScreen("api")
	s = typeText(s, "sudo docker exec -it web rails c")

	_, sub, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sub == nil {
		t.Fatal("expected a submit")
	}
	if sub.Line != "sudo docker exec -it web rails c" {
		t.Fatalf("line = %q", sub.Line)
	}
	if sub.Inner != "" {
		t.Fatalf("inner = %q, want empty", sub.Inner)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run CommandScreen -v`
Expected: FAIL to compile — `undefined: newCommandScreen`, `undefined: commandScreen`.

- [ ] **Step 3: Write the implementation**

Create `internal/tui/screen_command.go`:

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// commandSubmit is a confirmed command. Line is what actually runs; Inner is
// the in-container command to remember, empty when the user edited the whole
// line by hand (there is nothing container-scoped to record).
type commandSubmit struct {
	Line  string
	Inner string
}

// commandScreen asks for the command to run inside a container. By default the
// input holds only the in-container part and the full docker exec line is
// derived from it; tab switches to editing that line verbatim, for instances
// whose setup does not match the sudo docker exec assumption.
type commandScreen struct {
	container awsx.Container
	input     textinput.Model
	history   []string
	// cursor indexes history; len(history) means "not browsing".
	cursor int
	// fullLine is true while the whole command line is being edited.
	fullLine bool
	// savedInner keeps the in-container value while in full-line mode.
	savedInner string
	// manual means no container was chosen (the listing failed), so there is
	// no in-container mode to switch back to and no history to record.
	manual bool
	// target labels the header in manual mode, where there is no container.
	target string
}

func newCommandScreen(c awsx.Container, history []string) commandScreen {
	ti := newCommandInput("rails c")
	cursor := len(history)
	if len(history) > 0 {
		ti.SetValue(history[0])
		cursor = 0
	}
	return commandScreen{container: c, input: ti, history: history, cursor: cursor}
}

// newManualCommandScreen is the fallback when the container list could not be
// fetched: the whole command line is typed by hand.
func newManualCommandScreen(instanceName string) commandScreen {
	return commandScreen{
		input:    newCommandInput("sudo docker exec -it web rails c"),
		fullLine: true,
		manual:   true,
		target:   instanceName,
	}
}

func newCommandInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = "> "
	ti.Width = 60
	ti.Focus()
	return ti
}

func (s commandScreen) fullLineMode() bool { return s.fullLine }

// line is the full command that would run right now.
func (s commandScreen) line() string {
	v := strings.TrimSpace(s.input.Value())
	if s.fullLine {
		return v
	}
	if v == "" {
		return ""
	}
	return awsx.DockerExecLine(s.container.Name, v)
}

func (s commandScreen) Update(msg tea.Msg) (commandScreen, *commandSubmit, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, nil, cmd
	}

	switch km.Type {
	case tea.KeyEnter:
		line := s.line()
		if line == "" {
			return s, nil, nil
		}
		sub := commandSubmit{Line: line}
		if !s.fullLine {
			sub.Inner = strings.TrimSpace(s.input.Value())
		}
		return s, &sub, nil

	case tea.KeyTab:
		return s.toggleMode(), nil, nil

	case tea.KeyUp:
		if s.fullLine {
			return s, nil, nil
		}
		return s.moveHistory(-1), nil, nil

	case tea.KeyDown:
		if s.fullLine {
			return s, nil, nil
		}
		return s.moveHistory(1), nil, nil
	}

	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return s, nil, cmd
}

// toggleMode switches between editing the in-container command and the whole
// line, seeding the new value from the current one. It is a no-op in manual
// mode, where there is no container to build an in-container command against.
func (s commandScreen) toggleMode() commandScreen {
	if s.manual {
		return s
	}
	if s.fullLine {
		s.fullLine = false
		s.input.SetValue(s.savedInner)
		s.input.CursorEnd()
		return s
	}
	s.savedInner = strings.TrimSpace(s.input.Value())
	s.fullLine = true
	s.input.SetValue(awsx.DockerExecLine(s.container.Name, s.savedInner))
	s.input.CursorEnd()
	return s
}

// moveHistory steps the history cursor by delta, clamped to the list.
func (s commandScreen) moveHistory(delta int) commandScreen {
	if len(s.history) == 0 {
		return s
	}
	next := s.cursor + delta
	if next < 0 {
		next = 0
	}
	if next > len(s.history)-1 {
		next = len(s.history) - 1
	}
	s.cursor = next
	s.input.SetValue(s.history[next])
	s.input.CursorEnd()
	return s
}

func (s commandScreen) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", styleTitle.Render(s.header()))
	b.WriteString(s.input.View() + "\n\n")

	if s.manual {
		b.WriteString(styleFaint.Render("digite a linha completa que será executada na instância") + "\n\n")
		b.WriteString(styleFaint.Render("enter executa · tab indisponível · esc volta") + "\n")
		return b.String()
	}

	if s.fullLine {
		b.WriteString(styleFaint.Render("editando a linha completa") + "\n\n")
		b.WriteString(styleFaint.Render("enter executa · tab volta ao comando · esc volta") + "\n")
		return b.String()
	}

	if line := s.line(); line != "" {
		b.WriteString(styleFaint.Render(line) + "\n\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString(styleFaint.Render("enter executa · tab edita a linha toda · ↑↓ histórico · esc volta") + "\n")
	return b.String()
}

// header names what the command will run against: the container, or the
// instance when no container could be listed.
func (s commandScreen) header() string {
	if s.manual {
		return "Rodar comando em " + s.target
	}
	return fmt.Sprintf("Rodar comando em %s (%s)",
		awsx.DisplayContainer(s.container), s.container.Name)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run CommandScreen -v`
Expected: PASS (15 tests)

- [ ] **Step 5: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen_command.go internal/tui/screen_command_test.go
git commit -m "feat: add the command input screen with preview and history

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Container loading message and interactive exec command

**Files:**
- Modify: `internal/tui/messages.go` (add `containersMsg` next to `rdsMsg` ~line 34; add `loadContainersCmd` next to `loadRDSCmd`)
- Modify: `internal/tui/session.go` (add `interactiveCommander` and `interactiveExec` next to `portForwarder`/`portForwardExec`)
- Test: `internal/tui/session_test.go` (create)

**Interfaces:**
- Consumes: `awsx.ContainerLister`, `awsx.Container` (Tasks 1 and 3); `awsx.Sessioner`.
- Produces:
  - `type containersMsg struct{ containers []awsx.Container }`
  - `loadContainersCmd(cl awsx.ContainerLister, instanceID string) tea.Cmd`
  - `interactiveExec(s awsx.Sessioner, instanceID, command string) *exec.Cmd`
  - unexported `interactiveCommander` interface

- [ ] **Step 1: Write the failing test**

Create `internal/tui/session_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'InteractiveExec|LoadContainersCmd' -v`
Expected: FAIL to compile — `undefined: interactiveExec`, `undefined: loadContainersCmd`, `undefined: containersMsg`.

- [ ] **Step 3: Add the message and command**

In `internal/tui/messages.go`, add next to `rdsMsg`:

```go
type containersMsg struct{ containers []awsx.Container }
```

and next to `loadRDSCmd`:

```go
// loadContainersCmd lists the Docker containers running on instanceID. It uses
// a longer timeout than the other reads because the underlying SSM command has
// to be sent, executed on the instance, and polled back.
func loadContainersCmd(cl awsx.ContainerLister, instanceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), containerTimeout)
		defer cancel()
		cs, err := cl.Containers(ctx, instanceID)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ssm:SendCommand")}
		}
		return containersMsg{containers: cs}
	}
}
```

and next to the existing `awsTimeout` const:

```go
// containerTimeout covers the SendCommand + poll cycle behind a container list.
const containerTimeout = 30 * time.Second
```

- [ ] **Step 4: Add the exec plumbing**

Append to `internal/tui/session.go`:

```go
// interactiveCommander lets a Sessioner expose an SSM interactive-command
// *exec.Cmd.
type interactiveCommander interface {
	InteractiveCommand(instanceID, command string) *exec.Cmd
}

// interactiveExec returns the *exec.Cmd that runs command on the instance with
// a TTY attached, for tea.ExecProcess. Falls back to a direct aws invocation if
// the Sessioner does not expose InteractiveCommand.
func interactiveExec(s awsx.Sessioner, instanceID, command string) *exec.Cmd {
	if ic, ok := s.(interactiveCommander); ok {
		return ic.InteractiveCommand(instanceID, command)
	}
	params, err := json.Marshal(map[string][]string{"command": {command}})
	if err != nil {
		params = []byte(`{"command":[""]}`)
	}
	cmd := exec.Command("aws", "ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", string(params),
	)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}
```

Add `"encoding/json"` to the imports of `internal/tui/session.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run 'InteractiveExec|LoadContainersCmd' -v`
Expected: PASS (4 tests)

- [ ] **Step 6: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add internal/tui/messages.go internal/tui/session.go internal/tui/session_test.go
git commit -m "feat: add container loading and interactive exec plumbing

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Wire the flow into the root model

**Files:**
- Modify: `internal/tui/root.go` (`Deps.NewClients` and `Deps.NewEphemeral` signatures; `rootModel` fields; `targetsMsg` case; new `containersMsg` case; `sessionEndedMsg` case; `screenMenu` / `screenExecInstance` / `screenContainers` / `screenCommand` routing; `applyErrorNext`; `View`; `startChecking`; `startEphemeral`)
- Modify: `internal/cli/root.go` (the two factory closures)
- Modify: `internal/tui/root_test.go` (fake factories gain a return value; add flow tests)

**Interfaces:**
- Consumes: everything from Tasks 1-9.
- Produces:
  - `Deps.NewClients` returns `(IdentityProvider, EC2Lister, SSMLister, RDSLister, ContainerLister, string, error)`
  - `Deps.NewEphemeral` returns `(IdentityProvider, EC2Lister, SSMLister, RDSLister, ContainerLister, Sessioner, string, func(), error)`
  - `rootModel` fields `containers awsx.ContainerLister`, `execInstance awsx.Target`, `execContainer awsx.Container`, `containersScreen containersScreen`, `commandScreen commandScreen`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/root_test.go`:

```go
type fakeContainers struct{}

func (fakeContainers) Containers(context.Context, string) ([]awsx.Container, error) {
	return []awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
	}, nil
}

// menuToRunCommand drives a fresh model to the instance picker of the
// "Rodar comando" flow.
func menuToRunCommand(t *testing.T, d Deps) rootModel {
	t.Helper()
	m := NewRoot(d)
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select profile
	m = drive(m, identityMsg{id: awsx.Identity{Account: "1"}, region: "us-east-1"})

	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}) // túnel
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}) // Rodar comando
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.flow != flowExec {
		t.Fatalf("flow = %v, want flowExec", m.flow)
	}
	return m
}

func TestRoot_ExecFlow_InstanceToContainerToCommand(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())

	// instances arrive -> exec instance picker
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", State: "running", VpcID: "vpc-1"}, SSMOnline: true},
	}})
	if m.current != screenExecInstance {
		t.Fatalf("current = %v, want screenExecInstance", m.current)
	}

	// pick instance -> loading containers
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenChecking || m.execInstance.ID != "i-1" {
		t.Fatalf("current=%v instance=%q", m.current, m.execInstance.ID)
	}

	// containers arrive -> container picker
	m = drive(m, containersMsg{containers: []awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
	}})
	if m.current != screenContainers {
		t.Fatalf("current = %v, want screenContainers", m.current)
	}

	// pick container -> command screen
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenCommand {
		t.Fatalf("current = %v, want screenCommand", m.current)
	}
	if m.execContainer.Name != "myapp-web-1" {
		t.Fatalf("container = %q", m.execContainer.Name)
	}

	// type a command and submit -> an exec command is returned
	for _, r := range "bash" {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an exec command")
	}
}

func TestRoot_ExecFlow_RemembersCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := state.Load()
	d := fakeDeps()
	d.State = st

	m := menuToRunCommand(t, d)
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", VpcID: "vpc-1"}, SSMOnline: true},
	}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(m, containersMsg{containers: []awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web"},
	}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // pick container
	for _, r := range "rails c" {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	got := st.CommandHistory(state.ContainerKey("web"))
	if len(got) != 1 || got[0] != "rails c" {
		t.Fatalf("history = %v, want [rails c]", got)
	}
}

func TestRoot_ExecFlow_NoContainersShowsGuidance(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execInstance = awsx.Target{Instance: awsx.Instance{ID: "i-1", Name: "api"}}

	m = drive(m, containersMsg{containers: nil})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	if !strings.Contains(m.errorScreen.View(), "Nenhum container") {
		t.Fatalf("view missing empty message: %q", m.errorScreen.View())
	}
}

func TestRoot_ExecFlow_EscFromCommandGoesBackToContainers(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", VpcID: "vpc-1"}, SSMOnline: true},
	}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(m, containersMsg{containers: []awsx.Container{{ID: "abc", Name: "myapp-web-1", Service: "web"}}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.current != screenContainers {
		t.Fatalf("current = %v, want screenContainers", m.current)
	}
}

func TestRoot_ExecFlow_SessionEndReturnsToCommandScreen(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execContainer = awsx.Container{Name: "myapp-web-1", Service: "web"}

	m = drive(m, sessionEndedMsg{})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	v := m.errorScreen.View()
	if !strings.Contains(v, "Voltar para o comando") {
		t.Fatalf("missing back-to-command action: %q", v)
	}
}

func TestRoot_ExecFlow_ListingDeniedOffersManualCommand(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execInstance = awsx.Target{Instance: awsx.Instance{ID: "i-1", Name: "api"}}

	m = drive(m, errMsg{err: errors.New("denied"), action: "ssm:SendCommand"})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	v := m.errorScreen.View()
	if !strings.Contains(v, "ssm:SendCommand") {
		t.Fatalf("error does not name the permission: %q", v)
	}
	if !strings.Contains(v, "Digitar o comando à mão") {
		t.Fatalf("error does not offer the manual fallback: %q", v)
	}
}

func TestRoot_ExecFlow_ManualFallbackOpensFullLineCommandScreen(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execInstance = awsx.Target{Instance: awsx.Instance{ID: "i-1", Name: "api"}}
	m = drive(m, errMsg{err: errors.New("denied"), action: "ssm:SendCommand"})

	// The manual fallback is the first action on the error screen.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenCommand {
		t.Fatalf("current = %v, want screenCommand", m.current)
	}
	if !m.commandScreen.fullLineMode() {
		t.Fatal("expected the manual screen to start in full-line mode")
	}
	if !strings.Contains(m.commandScreen.View(), "api") {
		t.Fatalf("manual screen does not name the instance: %q", m.commandScreen.View())
	}
}
```

Add `"errors"` to the import block of `internal/tui/root_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run ExecFlow -v`
Expected: FAIL to compile — `m.execInstance undefined`, `m.execContainer undefined`, `m.commandScreen undefined` (the `rootModel` fields do not exist yet).

- [ ] **Step 3: Extend the injection seam**

In `internal/tui/root.go`, update the two factory signatures in `Deps`:

```go
	NewClients func(ctx context.Context, profile, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error)
```

```go
	NewEphemeral func(session profiles.SSOSession, accountID, roleName, region string) (idp awsx.IdentityProvider, ec2 awsx.EC2Lister, ssm awsx.SSMLister, rds awsx.RDSLister, containers awsx.ContainerLister, sess awsx.Sessioner, resolvedRegion string, cleanup func(), err error)
```

Add to `rootModel`, next to the other screens and clients:

```go
	containersScreen containersScreen
	commandScreen    commandScreen
```

```go
	containers awsx.ContainerLister
```

and next to the tunnel state:

```go
	// Exec (run-command) flow state.
	execInstance  awsx.Target
	execContainer awsx.Container
```

Update `startChecking`:

```go
	idp, ec2c, ssmc, rdsc, contc, resolved, err := m.deps.NewClients(context.Background(), p.Name, region)
```

and the assignment below it:

```go
	m.idp, m.ec2, m.ssm, m.rds, m.containers, m.region = idp, ec2c, ssmc, rdsc, contc, resolved
```

Update `startEphemeral`:

```go
	idp, ec2c, ssmc, rdsc, contc, sess, region2, cleanup, err := m.deps.NewEphemeral(m.ssoSession, m.accountID, m.roleName, region)
```

and its assignments:

```go
	m.rds, m.containers = rdsc, contc
	m.idp, m.ec2, m.ssm, m.session = idp, ec2c, ssmc, sess
```

- [ ] **Step 4: Route the exec flow**

In the `targetsMsg` case, replace the tunnel/else branch with a three-way switch:

```go
		switch m.flow {
		case flowTunnel:
			m.instancesScreen.list.Title = "Escolha a instância que fará o túnel (bastion SSM)"
			m.current = screenTunnelInstance
		case flowExec:
			m.instancesScreen.list.Title = "Escolha a instância que roda o container"
			m.current = screenExecInstance
		default:
			m.current = screenInstances
		}
```

Add a `containersMsg` case next to `rdsMsg`:

```go
	case containersMsg:
		if len(msg.containers) == 0 {
			return m.toError(
				"Nenhum container em execução nessa instância.",
				fmt.Sprintf("`docker ps` não retornou nada em %s (%s). A instância pode não rodar containers, ou o Docker pode estar parado.",
					awsx.DisplayName(m.execInstance.Instance), m.execInstance.ID),
				[]errorAction{
					{label: "Escolher outra instância", next: screenExecInstance},
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		m.containersScreen = newContainersScreen(msg.containers)
		m.containersScreen, _, _ = m.containersScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.containersScreen.list.Title = fmt.Sprintf("Containers em %s", awsx.DisplayName(m.execInstance.Instance))
		m.current = screenContainers
		return m, nil
```

In the `sessionEndedMsg` case, extend the back action and titles:

```go
		back := errorAction{label: "Voltar para as instâncias", next: screenInstances}
		switch m.flow {
		case flowTunnel:
			back = errorAction{label: "Voltar para os bancos", next: screenRDS}
		case flowExec:
			back = errorAction{label: "Voltar para o comando", next: screenCommand}
		}
```

and both title overrides become switches:

```go
			title := "Não foi possível abrir a sessão SSM."
			switch m.flow {
			case flowTunnel:
				title = "Não foi possível abrir o túnel."
			case flowExec:
				title = "Não foi possível rodar o comando."
			}
```

```go
		title := "Sessão encerrada."
		switch m.flow {
		case flowTunnel:
			title = "Túnel encerrado."
		case flowExec:
			title = "Comando encerrado."
		}
```

Add a container-listing failure branch at the **top** of the `errMsg` case, before the existing `m.current == screenChecking && m.login != nil` check. Order matters: container loading happens while `current` is `screenChecking`, so the existing check would otherwise send the user to the SSO login screen instead of showing the permission error.

```go
	case errMsg:
		// A failure while listing containers is recoverable: the command can
		// still be typed by hand. Prepare that screen so the error action can
		// switch to it.
		if m.flow == flowExec && m.current == screenChecking && m.execContainer.Name == "" {
			m.deps.Log.Error("listing containers on %s failed: %v", m.execInstance.ID, msg.err)
			detail := ""
			if msg.action != "" {
				detail = "Permissão necessária: " + msg.action + "\n\n"
			}
			if msg.err != nil {
				detail += msg.err.Error()
			}
			detail += "\n\nDetalhes no log: " + logging.Path()
			m.commandScreen = newManualCommandScreen(awsx.DisplayName(m.execInstance.Instance))
			return m.toError("Não foi possível listar os containers.", detail, []errorAction{
				{label: "Digitar o comando à mão", next: screenCommand},
				{label: "Escolher outra instância", next: screenExecInstance},
				{label: "Voltar para o menu principal", next: screenMenu},
				{label: "Sair", next: screenQuit},
			}), nil
		}
```

**Note (pre-existing, out of scope):** the `m.current == screenChecking && m.login != nil` check below routes *any* read failure during a loading phase to the SSO login screen — including a genuine `ec2:DescribeInstances` or `rds:DescribeDBInstances` denial. This plan works around it for the exec flow only; fixing it for the other flows is a separate change.

In the `screenMenu` routing case, add the new branch:

```go
		case screenExecInstance:
			m.flow = flowExec
			m.execContainer = awsx.Container{}
			m.current = screenChecking
			m.loading = "Carregando instâncias..."
			return m, loadTargetsCmd(m.ec2, m.ssm)
```

Add three routing cases after `screenRDS`:

```go
	case screenExecInstance:
		var sel *awsx.Target
		var cmd tea.Cmd
		m.instancesScreen, sel, cmd = m.instancesScreen.Update(msg)
		if sel != nil {
			m.execInstance = *sel
			// Clear any container from a previous round so a listing failure
			// is recognised as such.
			m.execContainer = awsx.Container{}
			m.current = screenChecking
			m.loading = "Carregando containers..."
			return m, loadContainersCmd(m.containers, sel.ID)
		}
		return m, cmd

	case screenContainers:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc && !m.containersScreen.list.SettingFilter() {
			m.current = screenExecInstance
			return m, nil
		}
		var sel *awsx.Container
		var cmd tea.Cmd
		m.containersScreen, sel, cmd = m.containersScreen.Update(msg)
		if sel != nil {
			m.execContainer = *sel
			m.commandScreen = newCommandScreen(*sel, m.commandHistory(*sel))
			m.current = screenCommand
		}
		return m, cmd

	case screenCommand:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc {
			m.current = screenContainers
			return m, nil
		}
		var sub *commandSubmit
		var cmd tea.Cmd
		m.commandScreen, sub, cmd = m.commandScreen.Update(msg)
		if sub != nil {
			return m.startExec(*sub)
		}
		return m, cmd
```

Add the two helpers next to `startTunnel`:

```go
// commandHistory returns the remembered commands for a container.
func (m rootModel) commandHistory(c awsx.Container) []string {
	if m.deps.State == nil {
		return nil
	}
	return m.deps.State.CommandHistory(state.ContainerKey(awsx.DisplayContainer(c)))
}

// startExec runs the confirmed command inside the chosen container, recording
// it in the per-container history first.
func (m rootModel) startExec(sub commandSubmit) (tea.Model, tea.Cmd) {
	if sub.Inner != "" && m.deps.State != nil {
		m.deps.State.PushCommand(state.ContainerKey(awsx.DisplayContainer(m.execContainer)), sub.Inner)
		if err := m.deps.State.Save(); err != nil {
			m.deps.Log.Error("could not save state: %v", err)
		}
	}
	id := m.execInstance.ID
	m.connectingName = fmt.Sprintf("%s em %s", awsx.DisplayContainer(m.execContainer), awsx.DisplayName(m.execInstance.Instance))
	m.connectingID = id
	m.deps.Log.Debug("running command: instance=%s container=%s region=%s line=%q",
		id, m.execContainer.Name, m.region, sub.Line)

	return m, execWithCapture(interactiveExec(m.session, id, sub.Line))
}
```

In `applyErrorNext`, add the three destinations:

```go
	case screenExecInstance:
		m.current = screenExecInstance
		return m, loadTargetsCmd(m.ec2, m.ssm)
	case screenContainers:
		m.current = screenContainers
		return m, nil
	case screenCommand:
		m.current = screenCommand
		return m, nil
```

In `View`, add the two new screens (and extend the shared instance-picker case):

```go
	case screenInstances, screenTunnelInstance, screenExecInstance:
		return m.instancesScreen.View()
	case screenContainers:
		return m.containersScreen.View()
	case screenCommand:
		return m.commandScreen.View()
```

- [ ] **Step 5: Update the test fakes**

In `internal/tui/root_test.go`, update `fakeDeps` and `ssoDeps` to return the container lister:

```go
		NewClients: func(context.Context, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, "us-east-1", nil
		},
```

```go
		NewEphemeral: func(profiles.SSOSession, string, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, awsx.Sessioner, string, func(), error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, nil, "sa-east-1", cleanup, nil
		},
```

The three other tests that override `NewClients` (`TestRoot_IdentityFailureWithLoginGoesToLogin`, `TestRoot_ProfileWithoutRegionShowsRegionPicker`, `TestRoot_ProfileRegionIsRemembered`) each need the same extra return value:

```go
	d.NewClients = func(context.Context, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
		return failIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, "us-east-1", nil
	}
```

```go
	d.NewClients = func(_ context.Context, _, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
		if region == "" {
			return nil, nil, nil, nil, nil, "", config.ErrNoRegion
		}
		return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, region, nil
	}
```

- [ ] **Step 6: Update the production wiring**

In `internal/cli/root.go`:

```go
				NewClients: func(ctx context.Context, profile, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
					c, resolved, err := awsx.NewClients(ctx, profile, region)
					if err != nil {
						return nil, nil, nil, nil, nil, "", err
					}
					return c, c, c, c, c, resolved, nil
				},
```

```go
				NewEphemeral: func(session profiles.SSOSession, accountID, roleName, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, awsx.Sessioner, string, func(), error) {
					eph, err := awsx.WriteEphemeralProfile(session, accountID, roleName, region)
					if err != nil {
						return nil, nil, nil, nil, nil, nil, "", nil, err
					}
					clients, resolved, err := eph.Clients(context.Background())
					if err != nil {
						_ = eph.Close()
						return nil, nil, nil, nil, nil, nil, "", nil, err
					}
					sess := awsx.CLI{Profile: eph.Profile, Region: resolved, ConfigFile: eph.ConfigPath}
					cleanup := func() { _ = eph.Close() }
					return clients, clients, clients, clients, clients, sess, resolved, cleanup, nil
				},
```

- [ ] **Step 7: Run the exec flow tests**

Run: `go test ./internal/tui/ -run ExecFlow -v`
Expected: PASS (7 tests)

- [ ] **Step 8: Run the whole suite**

Run: `go test ./... -v`
Expected: PASS, including all pre-existing tests

- [ ] **Step 9: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 10: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go internal/cli/root.go
git commit -m "feat: wire the run-command flow into the root model

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Document the feature and its permissions

**Files:**
- Modify: `README.md` ("What it does" list, "Required IAM permissions" block, new section after "EC2 access via SSM")

**Interfaces:**
- Consumes: the shipped behavior from Tasks 1-10.
- Produces: documentation only.

- [ ] **Step 1: Extend the IAM permission list**

In `README.md`, replace the permissions block with:

```
ec2:DescribeInstances
ssm:DescribeInstanceInformation
ssm:StartSession
ssm:TerminateSession
ssm:DescribeSessions
ssm:GetConnectionStatus
ssm:SendCommand
ssm:GetCommandInvocation
rds:DescribeDBInstances
```

and extend the sentence below it:

```markdown
Starting a session may also require access to the
`arn:aws:ssm:*:*:document/SSM-SessionManagerRunShell`,
`AWS-StartInteractiveCommand`, and `AWS-RunShellScript` documents.
```

- [ ] **Step 2: Add the feature to the "What it does" list**

Extend item 4 of the numbered list:

```markdown
4. Shows a menu: open an interactive session on an EC2 instance, tunnel to a
   database, or run a command inside a container on the instance.
```

- [ ] **Step 3: Document the flow**

Add after the "EC2 access via SSM" section:

```markdown
## Running commands in containers

The **Rodar comando** menu entry runs any command inside a Docker container on
an EC2 instance — a Rails console, a shell, a one-off script:

1. Pick the instance.
2. AWSX runs `docker ps` on it through `ssm:SendCommand` and lists the
   containers. Containers started by Docker Compose are shown by their service
   name, with the container name, image, and status below it.
3. Type the command. The last five commands per container are remembered and
   reachable with the arrow keys, so `rails c` is one keystroke the second time.
4. AWSX opens an interactive session running
   `sudo docker exec -it <container> <command>` — a real TTY, so consoles and
   long-running output work.

`sudo` is needed because the SSM session runs as `ssm-user`, which is not in the
`docker` group. `docker exec` is used rather than `docker compose exec` so the
command does not depend on the working directory or on which Compose version the
instance has.

Press `tab` on the command screen to edit the whole `docker exec` line, for
instances whose setup differs.
```

- [ ] **Step 4: Verify the docs match reality**

Run: `grep -n "Rodar comando" README.md internal/tui/screen_menu.go`
Expected: the menu label in the README matches `menuActions` exactly.

Run: `grep -n "ssm:SendCommand" README.md internal/tui/messages.go`
Expected: the permission named in the README matches the one in `deniedAction`.

- [ ] **Step 5: Full verification**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "docs: document running commands in containers

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Manual verification

After Task 11, run the binary against a real account before opening a PR:

```sh
go build -o /tmp/awsx ./cmd/awsx && AWSX_DEBUG=true /tmp/awsx
```

- [ ] Pick a profile, then "Rodar comando". The instance list is titled "Escolha a instância que roda o container".
- [ ] Pick an instance that runs containers. The container list appears within a few seconds, showing Compose service names.
- [ ] Type `bash`, confirm the grey preview reads `sudo docker exec -it <container> bash`, press Enter, land in a shell inside the container. `exit` returns to AWSX.
- [ ] Choose the same container again — the input is prefilled with `bash`.
- [ ] Run `rails c` on an application container and confirm the console is interactive (arrow keys, history, Ctrl+C).
- [ ] Press `tab`, confirm the input becomes the full line and is editable.
- [ ] Pick an instance with no containers and confirm the "Nenhum container em execução" screen offers a way back.
