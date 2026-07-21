# AWSX Phase 1 (Application) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A working `awsx` binary that, run locally, checks dependencies, lists AWS profiles, logs in via SSO when needed, lists SSM-capable running EC2 instances, and hands the terminal to an interactive SSM session — no instance IDs or commands typed by the user.

**Architecture:** Hybrid AWS integration. The `internal/aws` package is a boundary of small interfaces: SDK v2 for typed reads (`sts get-caller-identity`, `ec2 describe-instances`, `ssm describe-instance-information`) and shell-out for the two operations that require it (`aws sso login`, `aws ssm start-session`). A Bubble Tea `rootModel` holds shared state and delegates to per-screen sub-models; AWS work runs off the render loop as `tea.Cmd`s returning typed messages. A thin Cobra root runs the TUI.

**Tech Stack:** Go 1.23+, Cobra (root), Bubble Tea + Bubbles + Lip Gloss (TUI), aws-sdk-go-v2 (`config`, `service/sts`, `service/ec2`, `service/ssm`), `gopkg.in/ini.v1` (profile parsing).

## Global Constraints

- Go module path: `github.com/davidsgoncalves/awsx`.
- Go version floor: `go 1.23`.
- Supported platforms: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64. No Windows.
- **Never** write or print credentials/tokens; only *read* the official AWS CLI cache via the SDK. `~/.aws/config` is never modified by AWSX.
- No hardcoded machine paths (e.g. `/opt/homebrew`), no hardcoded region default.
- Region resolves from the selected profile; if none resolves, show a clear error (no fallback).
- All AWS SDK calls use a `context.Context` with a timeout.
- All code comments and commit messages in English.
- `AWSX_DEBUG=true` enables verbose logging through a filter that never emits `AWS_*`, tokens, or `~/.aws/sso/cache` contents.

**Local prerequisite:** Go is not installed on the dev machine. Before Task 1, confirm with the user and run `brew install go`. AWS CLI v2 and session-manager-plugin are already present.

---

## File Structure

```
cmd/awsx/main.go            entrypoint: builds root command, executes it
internal/
  cli/root.go               Cobra root; no subcommand -> runs TUI
  config/
    config.go               Resolve struct + AWSX_DEBUG; region/profile resolution
    config_test.go
  profiles/
    profiles.go             parse ~/.aws/config -> []Profile, SSO-first ordering
    profiles_test.go
  deps/
    deps.go                 OS/arch + PATH detection; install suggestions
    deps_test.go
  aws/
    types.go                Identity, Instance, Target (domain types) + interfaces
    inventory.go            Join([]Instance, onlineIDs) -> []Target (pure)
    inventory_test.go
    sdk.go                  real SDK-backed Identity/EC2Lister/SSMLister
    exec.go                 real exec-backed Login/Sessioner + argv builders
    exec_test.go            tests argv builders only
  tui/
    styles.go               shared lipgloss styles
    messages.go             typed tea.Msg definitions + tea.Cmd constructors
    root.go                 rootModel: shared state + screen delegation
    root_test.go
    screen_deps.go          depsScreen
    screen_profiles.go      profileScreen (bubbles/list, filterable)
    screen_checking.go      checkingScreen (spinner while sts runs)
    screen_login.go         loginScreen (runs sso login)
    screen_menu.go          menuScreen (header + actions)
    screen_instances.go     instancesScreen (bubbles/list, filterable)
    screen_error.go         errorScreen (message + action list, reusable)
    screen_*_test.go        per-screen Update()/View() tests
```

---

### Task 1: Bootstrap module + thin Cobra root

**Files:**
- Create: `go.mod` (via `go mod init`)
- Create: `cmd/awsx/main.go`
- Create: `internal/cli/root.go`
- Test: `internal/cli/root_test.go`

**Interfaces:**
- Produces: `cli.NewRootCmd() *cobra.Command` — root command; `RunE` launches the TUI. For this task `RunE` prints a placeholder so the binary is runnable; Task 15 rewires it to the TUI.

- [ ] **Step 1: Confirm Go install with user, then install**

Run (after user confirmation): `brew install go && go version`
Expected: prints `go version go1.2x ...`

- [ ] **Step 2: Initialize the module**

Run: `cd /Users/davidgoncalves/projects/awsx && go mod init github.com/davidsgoncalves/awsx`
Expected: creates `go.mod` with `module github.com/davidsgoncalves/awsx` and `go 1.2x`. Ensure the `go` line is `go 1.23` or higher (edit if the toolchain wrote a lower value).

- [ ] **Step 3: Write the failing test**

`internal/cli/root_test.go`:
```go
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
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestNewRootCmd_Use -v`
Expected: FAIL — build error, `undefined: NewRootCmd`.

- [ ] **Step 5: Add Cobra and write the root**

Run: `go get github.com/spf13/cobra@latest`

`internal/cli/root.go`:
```go
// Package cli builds the awsx command tree. The root command runs the
// interactive TUI; subcommands are reserved for future versions.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCmd returns the awsx root command. With no subcommand it runs the TUI.
func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "awsx",
		Short:         "Interactive AWS login and EC2 access via SSM Session Manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Rewired to the TUI in Task 15.
			fmt.Fprintln(cmd.OutOrStdout(), "awsx: TUI not wired yet")
			return nil
		},
	}
}
```

- [ ] **Step 6: Write main.go**

`cmd/awsx/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/davidsgoncalves/awsx/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 7: Run test + build to verify**

Run: `go mod tidy && go test ./internal/cli/ -v && go build -o /dev/null ./cmd/awsx`
Expected: PASS; build succeeds.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum cmd internal/cli
git commit -m "feat: bootstrap module and thin cobra root"
```

---

### Task 2: Profiles package — parse ~/.aws/config, SSO-first

**Files:**
- Create: `internal/profiles/profiles.go`
- Test: `internal/profiles/profiles_test.go`

**Interfaces:**
- Produces:
  - `type Profile struct { Name string; Region string; IsSSO bool }`
  - `func Parse(configPath string) ([]Profile, error)` — reads an AWS config file, returns profiles. SSO profiles (section has `sso_session` or `sso_start_url`) sorted first, then alphabetical within each group. Returns an empty slice (not error) if the file does not exist.

- [ ] **Step 1: Write the failing test**

`internal/profiles/profiles_test.go`:
```go
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

func TestParse_MissingFileReturnsEmpty(t *testing.T) {
	got, err := Parse(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/profiles/ -v`
Expected: FAIL — `undefined: Parse`.

- [ ] **Step 3: Implement**

Run: `go get gopkg.in/ini.v1@latest`

`internal/profiles/profiles.go`:
```go
// Package profiles reads AWS shared config profiles for selection in the TUI.
package profiles

import (
	"errors"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
)

// Profile is a selectable AWS profile from the shared config file.
type Profile struct {
	Name   string
	Region string
	IsSSO  bool
}

// Parse reads an AWS config file and returns its profiles. SSO profiles are
// listed first, alphabetical within each group. A missing file yields an empty
// slice and no error.
func Parse(configPath string) ([]Profile, error) {
	f, err := ini.Load(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var out []Profile
	for _, s := range f.Sections() {
		name, ok := profileName(s.Name())
		if !ok {
			continue
		}
		out = append(out, Profile{
			Name:   name,
			Region: s.Key("region").String(),
			IsSSO:  s.HasKey("sso_session") || s.HasKey("sso_start_url"),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsSSO != out[j].IsSSO {
			return out[i].IsSSO // SSO first
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// profileName maps a config section name to a profile name. In AWS config,
// the default profile is "[default]" and others are "[profile NAME]".
func profileName(section string) (string, bool) {
	if section == "default" {
		return "default", true
	}
	if rest, ok := strings.CutPrefix(section, "profile "); ok {
		return strings.TrimSpace(rest), true
	}
	return "", false // DEFAULT ini section, etc.
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/profiles/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/profiles go.mod go.sum
git commit -m "feat: parse aws config profiles with sso-first ordering"
```

---

### Task 3: Config package — paths, region resolution, debug flag

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `func ConfigPath() string` — `$AWS_CONFIG_FILE` if set, else `~/.aws/config`.
  - `func DebugEnabled() bool` — true when `AWSX_DEBUG=true`.
  - `func ResolveRegion(profileRegion string) (string, error)` — returns `AWS_REGION`/`AWS_DEFAULT_REGION` if set, else `profileRegion`; error `ErrNoRegion` if both empty.
  - `var ErrNoRegion = errors.New(...)`

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: ResolveRegion`.

- [ ] **Step 3: Implement**

`internal/config/config.go`:
```go
// Package config resolves runtime settings from the environment: config file
// path, region, and the debug flag. It never resolves credentials.
package config

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoRegion means no region could be resolved from env or the profile.
var ErrNoRegion = errors.New("no region configured for the selected profile; set one in ~/.aws/config or AWS_REGION")

// ConfigPath returns the AWS shared config path, honoring AWS_CONFIG_FILE.
func ConfigPath() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".aws", "config")
	}
	return filepath.Join(home, ".aws", "config")
}

// DebugEnabled reports whether verbose logging is on.
func DebugEnabled() bool {
	return os.Getenv("AWSX_DEBUG") == "true"
}

// ResolveRegion returns the effective region: env override first, then the
// profile's configured region. ErrNoRegion if neither is set.
func ResolveRegion(profileRegion string) (string, error) {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r, nil
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r, nil
	}
	if profileRegion != "" {
		return profileRegion, nil
	}
	return "", ErrNoRegion
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat: resolve config path, region, and debug flag"
```

---

### Task 4: Deps package — detect aws CLI + session-manager-plugin

**Files:**
- Create: `internal/deps/deps.go`
- Test: `internal/deps/deps_test.go`

**Interfaces:**
- Produces:
  - `type Dependency struct { Name string; Binary string; Found bool; Path string }`
  - `func Check() []Dependency` — checks `aws` and `session-manager-plugin` on PATH via `exec.LookPath`.
  - `func InstallHint(binary string, goos string, hasBrew bool) string` — returns a human instruction string for installing the given binary on the given OS. Pure (params injected) so it is testable without touching the real machine.
  - `func BrewAvailable() bool` — `exec.LookPath("brew")` succeeded.

- [ ] **Step 1: Write the failing test**

`internal/deps/deps_test.go`:
```go
package deps

import (
	"strings"
	"testing"
)

func TestInstallHint_MacBrew(t *testing.T) {
	got := InstallHint("aws", "darwin", true)
	if !strings.Contains(got, "brew install awscli") {
		t.Fatalf("hint should suggest brew, got: %q", got)
	}
}

func TestInstallHint_MacNoBrew(t *testing.T) {
	got := InstallHint("aws", "darwin", false)
	if strings.Contains(got, "brew") {
		t.Fatalf("no-brew hint should not mention brew, got: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "awscli.amazonaws.com") &&
		!strings.Contains(strings.ToLower(got), "official") {
		t.Fatalf("hint should point to official installer, got: %q", got)
	}
}

func TestInstallHint_LinuxPlugin(t *testing.T) {
	got := InstallHint("session-manager-plugin", "linux", false)
	if !strings.Contains(strings.ToLower(got), "session-manager-plugin") {
		t.Fatalf("hint should mention the plugin, got: %q", got)
	}
}

func TestCheck_ReturnsBothDeps(t *testing.T) {
	got := Check()
	if len(got) != 2 {
		t.Fatalf("want 2 deps, got %d", len(got))
	}
	names := map[string]bool{got[0].Binary: true, got[1].Binary: true}
	if !names["aws"] || !names["session-manager-plugin"] {
		t.Fatalf("unexpected deps: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/deps/ -v`
Expected: FAIL — `undefined: InstallHint`.

- [ ] **Step 3: Implement**

`internal/deps/deps.go`:
```go
// Package deps detects the external runtime dependencies AWSX needs and
// produces platform-appropriate install hints. It makes no assumptions about
// the host beyond querying PATH and the provided GOOS.
package deps

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Dependency is an external binary AWSX relies on.
type Dependency struct {
	Name   string
	Binary string
	Found  bool
	Path   string
}

var required = []struct{ name, binary string }{
	{"AWS CLI", "aws"},
	{"Session Manager Plugin", "session-manager-plugin"},
}

// Check looks up each required dependency on PATH.
func Check() []Dependency {
	out := make([]Dependency, 0, len(required))
	for _, r := range required {
		path, err := exec.LookPath(r.binary)
		out = append(out, Dependency{
			Name:   r.name,
			Binary: r.binary,
			Found:  err == nil,
			Path:   path,
		})
	}
	return out
}

// BrewAvailable reports whether Homebrew is on PATH.
func BrewAvailable() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// GOOS returns the current operating system. Wrapper kept so callers pass it
// into InstallHint, keeping that function pure and testable.
func GOOS() string { return runtime.GOOS }

// InstallHint returns a human-readable install instruction for binary on goos.
func InstallHint(binary, goos string, hasBrew bool) string {
	switch binary {
	case "aws":
		switch goos {
		case "darwin":
			if hasBrew {
				return "Install with: brew install awscli"
			}
			return "Download the official AWS CLI v2 installer: https://awscli.amazonaws.com/AWSCLIV2.pkg"
		default: // linux
			return "Install AWS CLI v2 (official): curl 'https://awscli.amazonaws.com/awscli-exe-linux-$(uname -m).zip' -o awscliv2.zip && unzip awscliv2.zip && sudo ./aws/install"
		}
	case "session-manager-plugin":
		switch goos {
		case "darwin":
			if hasBrew {
				return "Install with: brew install --cask session-manager-plugin"
			}
			return "Install the session-manager-plugin (official docs): https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html"
		default: // linux
			return "Install the session-manager-plugin (official docs): https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html"
		}
	}
	return fmt.Sprintf("Install %q and ensure it is on your PATH.", binary)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/deps/ -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add internal/deps
git commit -m "feat: detect aws cli and session-manager-plugin with install hints"
```

---

### Task 5: AWS domain types + inventory join (pure core)

**Files:**
- Create: `internal/aws/types.go`
- Create: `internal/aws/inventory.go`
- Test: `internal/aws/inventory_test.go`

**Interfaces:**
- Produces:
  - `type Identity struct { Account string; Arn string; UserID string }`
  - `type Instance struct { ID string; Name string; State string; Type string; PrivateIP string }`
  - `type Target struct { Instance; SSMOnline bool }`
  - `func Join(instances []Instance, onlineIDs map[string]bool) []Target` — returns targets for running instances that are online in SSM, sorted by display Name. This is the EC2×SSM cross the PRD (§6.9) describes.
  - Interfaces consumed later by the TUI:
    - `type IdentityProvider interface { WhoAmI(ctx context.Context) (Identity, error) }`
    - `type EC2Lister interface { RunningInstances(ctx context.Context) ([]Instance, error) }`
    - `type SSMLister interface { OnlineInstanceIDs(ctx context.Context) (map[string]bool, error) }`
    - `type Login interface { SSOLogin(ctx context.Context) error }`
    - `type Sessioner interface { StartSession(instanceID string) error }`
  - `func DisplayName(i Instance) string` — `i.Name` if non-empty, else `i.ID`.

- [ ] **Step 1: Write the failing test**

`internal/aws/inventory_test.go`:
```go
package aws

import "testing"

func TestJoin_OnlyRunningAndOnline_SortedByName(t *testing.T) {
	instances := []Instance{
		{ID: "i-3", Name: "worker", State: "running", Type: "t3.medium", PrivateIP: "10.0.1.22"},
		{ID: "i-1", Name: "api", State: "running", Type: "t3.large", PrivateIP: "10.0.1.15"},
		{ID: "i-2", Name: "stopped-box", State: "stopped", Type: "t3.small", PrivateIP: "10.0.1.9"},
		{ID: "i-4", Name: "no-ssm", State: "running", Type: "t3.small", PrivateIP: "10.0.1.30"},
	}
	online := map[string]bool{"i-1": true, "i-3": true, "i-4": false}

	got := Join(instances, online)

	if len(got) != 2 {
		t.Fatalf("want 2 targets, got %d: %+v", len(got), got)
	}
	if got[0].Name != "api" || got[1].Name != "worker" {
		t.Fatalf("unexpected order: %q, %q", got[0].Name, got[1].Name)
	}
	if !got[0].SSMOnline {
		t.Fatal("api should be SSMOnline")
	}
}

func TestDisplayName_FallsBackToID(t *testing.T) {
	if got := DisplayName(Instance{ID: "i-9"}); got != "i-9" {
		t.Fatalf("got %q, want i-9", got)
	}
	if got := DisplayName(Instance{ID: "i-9", Name: "api"}); got != "api" {
		t.Fatalf("got %q, want api", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/aws/ -run 'TestJoin|TestDisplayName' -v`
Expected: FAIL — `undefined: Join`.

- [ ] **Step 3: Implement types**

`internal/aws/types.go`:
```go
// Package aws is the boundary between AWSX and AWS. Typed reads go through the
// SDK; the two operations that require it (sso login, start-session) shell out
// to the AWS CLI. All behavior is expressed as small interfaces so the TUI can
// be tested with fakes.
package aws

import "context"

// Identity is the caller identity from sts:GetCallerIdentity.
type Identity struct {
	Account string
	Arn     string
	UserID  string
}

// Instance is a minimal EC2 instance view.
type Instance struct {
	ID        string
	Name      string
	State     string
	Type      string
	PrivateIP string
}

// Target is an instance that can receive an SSM session.
type Target struct {
	Instance
	SSMOnline bool
}

// IdentityProvider resolves the caller identity for a profile/region.
type IdentityProvider interface {
	WhoAmI(ctx context.Context) (Identity, error)
}

// EC2Lister lists running EC2 instances in the resolved region.
type EC2Lister interface {
	RunningInstances(ctx context.Context) ([]Instance, error)
}

// SSMLister returns the set of instance IDs currently online in SSM.
type SSMLister interface {
	OnlineInstanceIDs(ctx context.Context) (map[string]bool, error)
}

// Login performs the interactive SSO login (shell-out to the AWS CLI).
type Login interface {
	SSOLogin(ctx context.Context) error
}

// Sessioner opens an interactive SSM session, handing over the terminal.
type Sessioner interface {
	StartSession(instanceID string) error
}

// DisplayName returns the instance Name tag or the instance ID as fallback.
func DisplayName(i Instance) string {
	if i.Name != "" {
		return i.Name
	}
	return i.ID
}
```

- [ ] **Step 4: Implement the join**

`internal/aws/inventory.go`:
```go
package aws

import "sort"

// Join returns SSM-capable targets: running instances that are online in SSM,
// sorted by display name. This is the EC2 x SSM cross described in PRD 6.9.
func Join(instances []Instance, onlineIDs map[string]bool) []Target {
	var out []Target
	for _, inst := range instances {
		if inst.State != "running" {
			continue
		}
		if !onlineIDs[inst.ID] {
			continue
		}
		out = append(out, Target{Instance: inst, SSMOnline: true})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return DisplayName(out[i].Instance) < DisplayName(out[j].Instance)
	})
	return out
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/aws/ -run 'TestJoin|TestDisplayName' -v`
Expected: PASS (both).

- [ ] **Step 6: Commit**

```bash
git add internal/aws/types.go internal/aws/inventory.go internal/aws/inventory_test.go
git commit -m "feat: aws domain types, boundary interfaces, and ec2xssm join"
```

---

### Task 6: SDK-backed clients (identity, EC2, SSM)

**Files:**
- Create: `internal/aws/sdk.go`
- Test: none automated (real SDK calls need AWS; the join and TUI are tested via fakes). Verified by `go build` + manual smoke.

**Interfaces:**
- Consumes: `Identity`, `Instance` (Task 5); `config.ResolveRegion` (Task 3).
- Produces:
  - `type Clients struct { ... }` implementing `IdentityProvider`, `EC2Lister`, `SSMLister`.
  - `func NewClients(ctx context.Context, profile string) (*Clients, string, error)` — loads shared config for `profile`, resolves region (error if none), returns clients + the resolved region.

- [ ] **Step 1: Add SDK dependencies**

Run:
```bash
go get github.com/aws/aws-sdk-go-v2/config \
       github.com/aws/aws-sdk-go-v2/service/sts \
       github.com/aws/aws-sdk-go-v2/service/ec2 \
       github.com/aws/aws-sdk-go-v2/service/ssm \
       github.com/aws/smithy-go
```

- [ ] **Step 2: Implement the SDK clients**

`internal/aws/sdk.go`:
```go
package aws

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/davidsgoncalves/awsx/internal/config"
)

// Clients bundles the SDK-backed readers for a single profile/region.
type Clients struct {
	sts *sts.Client
	ec2 *ec2.Client
	ssm *ssm.Client
}

// NewClients loads shared config for profile, resolves the region (error if
// none), and returns typed clients plus the resolved region. It reads the
// official AWS CLI SSO cache; it never creates credentials.
func NewClients(ctx context.Context, profile string) (*Clients, string, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithSharedConfigProfile(profile))
	if err != nil {
		return nil, "", fmt.Errorf("load profile %q: %w", profile, err)
	}
	region, err := config.ResolveRegion(cfg.Region)
	if err != nil {
		return nil, "", err
	}
	cfg.Region = region
	return &Clients{
		sts: sts.NewFromConfig(cfg),
		ec2: ec2.NewFromConfig(cfg),
		ssm: ssm.NewFromConfig(cfg),
	}, region, nil
}

// WhoAmI implements IdentityProvider.
func (c *Clients) WhoAmI(ctx context.Context) (Identity, error) {
	out, err := c.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		Account: deref(out.Account),
		Arn:     deref(out.Arn),
		UserID:  deref(out.UserId),
	}, nil
}

// RunningInstances implements EC2Lister.
func (c *Clients) RunningInstances(ctx context.Context) ([]Instance, error) {
	var out []Instance
	p := ec2.NewDescribeInstancesPaginator(c.ec2, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{
			Name:   ptr("instance-state-name"),
			Values: []string{"running"},
		}},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, res := range page.Reservations {
			for _, inst := range res.Instances {
				out = append(out, Instance{
					ID:        deref(inst.InstanceId),
					Name:      nameTag(inst.Tags),
					State:     string(inst.State.Name),
					Type:      string(inst.InstanceType),
					PrivateIP: deref(inst.PrivateIpAddress),
				})
			}
		}
	}
	return out, nil
}

// OnlineInstanceIDs implements SSMLister.
func (c *Clients) OnlineInstanceIDs(ctx context.Context) (map[string]bool, error) {
	online := map[string]bool{}
	p := ssm.NewDescribeInstanceInformationPaginator(c.ssm, &ssm.DescribeInstanceInformationInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, info := range page.InstanceInformationList {
			if string(info.PingStatus) == "Online" {
				online[deref(info.InstanceId)] = true
			}
		}
	}
	return online, nil
}

func nameTag(tags []ec2types.Tag) string {
	for _, t := range tags {
		if deref(t.Key) == "Name" {
			return deref(t.Value)
		}
	}
	return ""
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```

- [ ] **Step 3: Build to verify**

Run: `go mod tidy && go build ./...`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add internal/aws/sdk.go go.mod go.sum
git commit -m "feat: sdk-backed identity, ec2, and ssm readers"
```

---

### Task 7: Exec-backed login + session (argv builders tested)

**Files:**
- Create: `internal/aws/exec.go`
- Test: `internal/aws/exec_test.go`

**Interfaces:**
- Consumes: `Login`, `Sessioner` (Task 5).
- Produces:
  - `type CLI struct { Profile string }` implementing `Login` and `Sessioner`.
  - `func loginArgs(profile string) []string` — `["sso","login","--profile",profile]`.
  - `func sessionArgs(profile, instanceID string) []string` — `["ssm","start-session","--profile",profile,"--target",instanceID]`.
  - `SSOLogin` runs `aws` with `loginArgs`, inheriting stdout/stderr, respecting ctx.
  - `StartSession` runs `aws` with `sessionArgs`, inheriting stdin/stdout/stderr (TTY handoff).

- [ ] **Step 1: Write the failing test**

`internal/aws/exec_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/aws/ -run 'Args' -v`
Expected: FAIL — `undefined: loginArgs`.

- [ ] **Step 3: Implement**

`internal/aws/exec.go`:
```go
package aws

import (
	"context"
	"os"
	"os/exec"
)

// CLI shells out to the AWS CLI for the two operations the SDK cannot do:
// interactive SSO login and the terminal-owning SSM session.
type CLI struct {
	Profile string
}

func loginArgs(profile string) []string {
	return []string{"sso", "login", "--profile", profile}
}

func sessionArgs(profile, instanceID string) []string {
	return []string{"ssm", "start-session", "--profile", profile, "--target", instanceID}
}

// SSOLogin runs `aws sso login`, streaming its output so the user sees the
// browser/device-code prompts. Honors ctx cancellation.
func (c CLI) SSOLogin(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "aws", loginArgs(c.Profile)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// StartSession runs `aws ssm start-session`, handing over stdin/stdout/stderr
// so the session-manager-plugin takes control of the terminal.
func (c CLI) StartSession(instanceID string) error {
	cmd := exec.Command("aws", sessionArgs(c.Profile, instanceID)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/aws/ -run 'Args' -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/aws/exec.go internal/aws/exec_test.go
git commit -m "feat: exec-backed sso login and ssm start-session"
```

---

### Task 8: TUI styles + typed messages

**Files:**
- Create: `internal/tui/styles.go`
- Create: `internal/tui/messages.go`

**Interfaces:**
- Consumes: `aws.Identity`, `aws.Target` (Task 5); the boundary interfaces.
- Produces:
  - Message types: `identityMsg{ id aws.Identity; region string }`, `targetsMsg{ targets []aws.Target }`, `loginDoneMsg{ err error }`, `sessionEndedMsg{ err error }`, `errMsg{ err error; action string }`.
  - `func loadIdentityCmd(p aws.IdentityProvider, region string) tea.Cmd`
  - `func loadTargetsCmd(ec2 aws.EC2Lister, ssm aws.SSMLister) tea.Cmd`
  - `func loginCmd(l aws.Login) tea.Cmd`
  - `var styleTitle, styleErr, styleFaint lipgloss.Style`

- [ ] **Step 1: Add Bubble Tea deps**

Run:
```bash
go get github.com/charmbracelet/bubbletea@latest \
       github.com/charmbracelet/bubbles@latest \
       github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Write styles**

`internal/tui/styles.go`:
```go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	styleTitle = lipgloss.NewStyle().Bold(true)
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleFaint = lipgloss.NewStyle().Faint(true)
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)
```

- [ ] **Step 3: Write messages + commands**

`internal/tui/messages.go`:
```go
package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/aws/smithy-go"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

const awsTimeout = 15 * time.Second

type identityMsg struct {
	id     awsx.Identity
	region string
}
type targetsMsg struct{ targets []awsx.Target }
type loginDoneMsg struct{ err error }
type sessionEndedMsg struct{ err error }

// errMsg carries a failed operation. action, when set, is the denied IAM action
// (e.g. "ec2:DescribeInstances") extracted from the SDK error.
type errMsg struct {
	err    error
	action string
}

// deniedAction returns the IAM action from an access-denied SDK error, or "".
func deniedAction(err error, fallback string) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation":
			return fallback
		}
	}
	return ""
}

func loadIdentityCmd(p awsx.IdentityProvider, region string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		id, err := p.WhoAmI(ctx)
		if err != nil {
			return errMsg{err: err}
		}
		return identityMsg{id: id, region: region}
	}
}

func loadTargetsCmd(ec2 awsx.EC2Lister, ssm awsx.SSMLister) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		instances, err := ec2.RunningInstances(ctx)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ec2:DescribeInstances")}
		}
		online, err := ssm.OnlineInstanceIDs(ctx)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ssm:DescribeInstanceInformation")}
		}
		return targetsMsg{targets: awsx.Join(instances, online)}
	}
}

func loginCmd(l awsx.Login) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return loginDoneMsg{err: l.SSOLogin(ctx)}
	}
}
```

- [ ] **Step 4: Build to verify**

Run: `go mod tidy && go build ./internal/tui/`
Expected: builds (no `View`/`Update` yet, but the file compiles).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/styles.go internal/tui/messages.go go.mod go.sum
git commit -m "feat: tui styles and typed aws command messages"
```

---

### Task 9: errorScreen (reusable)

**Files:**
- Create: `internal/tui/screen_error.go`
- Test: `internal/tui/screen_error_test.go`

**Interfaces:**
- Produces:
  - `type errorAction struct { label string; next screen }` (where `screen` is the enum defined here).
  - `type screen int` with consts `screenDeps, screenProfiles, screenChecking, screenLogin, screenMenu, screenInstances, screenError, screenQuit`.
  - `type errorScreen struct { title, detail string; actions []errorAction; cursor int }`
  - `func newErrorScreen(title, detail string, actions []errorAction) errorScreen`
  - `func (e errorScreen) Update(msg tea.Msg) (errorScreen, screen)` — arrow keys move cursor; Enter returns the selected action's `next`; otherwise returns `screenError`.
  - `func (e errorScreen) View() string`

- [ ] **Step 1: Write the failing test**

`internal/tui/screen_error_test.go`:
```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestErrorScreen_EnterReturnsSelectedNext(t *testing.T) {
	e := newErrorScreen("Boom", "it broke", []errorAction{
		{label: "Retry", next: screenChecking},
		{label: "Quit", next: screenQuit},
	})
	// move down to "Quit", press enter
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, next := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenQuit {
		t.Fatalf("next = %v, want screenQuit", next)
	}
}

func TestErrorScreen_ViewShowsDetail(t *testing.T) {
	e := newErrorScreen("Denied", "Permission needed: ec2:DescribeInstances", nil)
	if !strings.Contains(e.View(), "ec2:DescribeInstances") {
		t.Fatalf("view missing detail: %q", e.View())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestErrorScreen -v`
Expected: FAIL — `undefined: newErrorScreen`.

- [ ] **Step 3: Implement**

`internal/tui/screen_error.go`:
```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenDeps screen = iota
	screenProfiles
	screenChecking
	screenLogin
	screenMenu
	screenInstances
	screenError
	screenQuit
)

type errorAction struct {
	label string
	next  screen
}

type errorScreen struct {
	title   string
	detail  string
	actions []errorAction
	cursor  int
}

func newErrorScreen(title, detail string, actions []errorAction) errorScreen {
	if len(actions) == 0 {
		actions = []errorAction{{label: "Sair", next: screenQuit}}
	}
	return errorScreen{title: title, detail: detail, actions: actions}
}

func (e errorScreen) Update(msg tea.Msg) (errorScreen, screen) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return e, screenError
	}
	switch km.Type {
	case tea.KeyUp:
		if e.cursor > 0 {
			e.cursor--
		}
	case tea.KeyDown:
		if e.cursor < len(e.actions)-1 {
			e.cursor++
		}
	case tea.KeyEnter:
		return e, e.actions[e.cursor].next
	}
	return e, screenError
}

func (e errorScreen) View() string {
	var b strings.Builder
	b.WriteString(styleErr.Render(e.title) + "\n\n")
	if e.detail != "" {
		b.WriteString(e.detail + "\n\n")
	}
	for i, a := range e.actions {
		cursor := "  "
		if i == e.cursor {
			cursor = "> "
		}
		b.WriteString(cursor + a.label + "\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestErrorScreen -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen_error.go internal/tui/screen_error_test.go
git commit -m "feat: reusable tui error screen with action list"
```

---

### Task 10: depsScreen

**Files:**
- Create: `internal/tui/screen_deps.go`
- Test: `internal/tui/screen_deps_test.go`

**Interfaces:**
- Consumes: `deps.Dependency` (Task 4); `screen` enum (Task 9).
- Produces:
  - `type depsScreen struct { checks []deps.Dependency }`
  - `func newDepsScreen(checks []deps.Dependency) depsScreen`
  - `func (d depsScreen) allFound() bool`
  - `func (d depsScreen) missing() []deps.Dependency`
  - `func (d depsScreen) View() string` — one line per dep with ✓/✗.

- [ ] **Step 1: Write the failing test**

`internal/tui/screen_deps_test.go`:
```go
package tui

import (
	"strings"
	"testing"

	"github.com/davidsgoncalves/awsx/internal/deps"
)

func TestDepsScreen_AllFound(t *testing.T) {
	d := newDepsScreen([]deps.Dependency{
		{Name: "AWS CLI", Binary: "aws", Found: true},
		{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
	})
	if !d.allFound() {
		t.Fatal("want allFound true")
	}
	if len(d.missing()) != 0 {
		t.Fatalf("want no missing, got %v", d.missing())
	}
}

func TestDepsScreen_MissingListed(t *testing.T) {
	d := newDepsScreen([]deps.Dependency{
		{Name: "AWS CLI", Binary: "aws", Found: false},
		{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
	})
	if d.allFound() {
		t.Fatal("want allFound false")
	}
	m := d.missing()
	if len(m) != 1 || m[0].Binary != "aws" {
		t.Fatalf("want [aws] missing, got %v", m)
	}
	if !strings.Contains(d.View(), "AWS CLI") {
		t.Fatalf("view should mention AWS CLI: %q", d.View())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestDepsScreen -v`
Expected: FAIL — `undefined: newDepsScreen`.

- [ ] **Step 3: Implement**

`internal/tui/screen_deps.go`:
```go
package tui

import (
	"strings"

	"github.com/davidsgoncalves/awsx/internal/deps"
)

type depsScreen struct {
	checks []deps.Dependency
}

func newDepsScreen(checks []deps.Dependency) depsScreen {
	return depsScreen{checks: checks}
}

func (d depsScreen) allFound() bool {
	return len(d.missing()) == 0
}

func (d depsScreen) missing() []deps.Dependency {
	var out []deps.Dependency
	for _, c := range d.checks {
		if !c.Found {
			out = append(out, c)
		}
	}
	return out
}

func (d depsScreen) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Verificando ambiente...") + "\n\n")
	for _, c := range d.checks {
		mark := styleOK.Render("✓")
		if !c.Found {
			mark = styleErr.Render("✗")
		}
		b.WriteString(mark + " " + c.Name + "\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestDepsScreen -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen_deps.go internal/tui/screen_deps_test.go
git commit -m "feat: tui deps screen"
```

---

### Task 11: profileScreen (filterable list)

**Files:**
- Create: `internal/tui/screen_profiles.go`
- Test: `internal/tui/screen_profiles_test.go`

**Interfaces:**
- Consumes: `profiles.Profile` (Task 2); `bubbles/list`.
- Produces:
  - `type profileItem struct { p profiles.Profile }` implementing `list.Item` (`FilterValue() string` returns the profile name).
  - `type profileScreen struct { list list.Model }`
  - `func newProfileScreen(ps []profiles.Profile) profileScreen`
  - `func (s profileScreen) Update(msg tea.Msg) (profileScreen, *profiles.Profile, tea.Cmd)` — returns the selected profile (non-nil) when Enter is pressed on an item; otherwise nil.
  - `func (s profileScreen) View() string`

- [ ] **Step 1: Write the failing test**

`internal/tui/screen_profiles_test.go`:
```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

func TestProfileScreen_EnterSelects(t *testing.T) {
	s := newProfileScreen([]profiles.Profile{
		{Name: "alpha-sso", IsSSO: true, Region: "sa-east-1"},
		{Name: "beta"},
	})
	// list needs a size to render/select
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil {
		t.Fatal("want a selected profile, got nil")
	}
	if sel.Name != "alpha-sso" {
		t.Fatalf("selected %q, want alpha-sso", sel.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestProfileScreen -v`
Expected: FAIL — `undefined: newProfileScreen`.

- [ ] **Step 3: Implement**

`internal/tui/screen_profiles.go`:
```go
package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

type profileItem struct{ p profiles.Profile }

func (i profileItem) FilterValue() string { return i.p.Name }
func (i profileItem) Title() string {
	if i.p.IsSSO {
		return i.p.Name + "  (SSO)"
	}
	return i.p.Name
}
func (i profileItem) Description() string { return i.p.Region }

type profileScreen struct {
	list list.Model
}

func newProfileScreen(ps []profiles.Profile) profileScreen {
	items := make([]list.Item, len(ps))
	for i, p := range ps {
		items[i] = profileItem{p: p}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione um perfil AWS"
	return profileScreen{list: l}
}

func (s profileScreen) Update(msg tea.Msg) (profileScreen, *profiles.Profile, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(profileItem); ok {
			p := it.p
			return s, &p, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s profileScreen) View() string { return s.list.View() }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestProfileScreen -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen_profiles.go internal/tui/screen_profiles_test.go
git commit -m "feat: tui filterable profile selection screen"
```

---

### Task 12: menuScreen

**Files:**
- Create: `internal/tui/screen_menu.go`
- Test: `internal/tui/screen_menu_test.go`

**Interfaces:**
- Consumes: `aws.Identity` (Task 5); `screen` enum (Task 9).
- Produces:
  - `type menuScreen struct { profile, region string; id aws.Identity; cursor int }`
  - `func newMenuScreen(profile, region string, id aws.Identity) menuScreen`
  - `func (m menuScreen) Update(msg tea.Msg) (menuScreen, screen)` — Enter on "Acessar EC2" -> `screenInstances`; on "Sair" -> `screenQuit`; else `screenMenu`.
  - `func (m menuScreen) View() string` — header (profile/account/region) + two actions.
  - `func roleFromArn(arn string) string` — best-effort role display from an assumed-role ARN; returns "" if not parseable.

- [ ] **Step 1: Write the failing test**

`internal/tui/screen_menu_test.go`:
```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func TestMenuScreen_SelectEC2AndQuit(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "123"})
	_, next := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cursor 0 = Acessar EC2
	if next != screenInstances {
		t.Fatalf("next = %v, want screenInstances", next)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, next = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != screenQuit {
		t.Fatalf("next = %v, want screenQuit", next)
	}
}

func TestMenuScreen_ViewShowsHeader(t *testing.T) {
	m := newMenuScreen("prod", "us-east-1", awsx.Identity{Account: "947592431146"})
	v := m.View()
	if !strings.Contains(v, "prod") || !strings.Contains(v, "947592431146") || !strings.Contains(v, "us-east-1") {
		t.Fatalf("header missing fields: %q", v)
	}
}

func TestRoleFromArn(t *testing.T) {
	arn := "arn:aws:sts::123:assumed-role/AWSReservedSSO_SystemAdministrator_abc/david"
	if got := roleFromArn(arn); got != "AWSReservedSSO_SystemAdministrator_abc" {
		t.Fatalf("got %q", got)
	}
	if got := roleFromArn("garbage"); got != "" {
		t.Fatalf("want empty for unparseable, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestMenuScreen|TestRoleFromArn' -v`
Expected: FAIL — `undefined: newMenuScreen`.

- [ ] **Step 3: Implement**

`internal/tui/screen_menu.go`:
```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

var menuActions = []string{"Acessar EC2", "Sair"}

type menuScreen struct {
	profile string
	region  string
	id      awsx.Identity
	cursor  int
}

func newMenuScreen(profile, region string, id awsx.Identity) menuScreen {
	return menuScreen{profile: profile, region: region, id: id}
}

func (m menuScreen) Update(msg tea.Msg) (menuScreen, screen) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, screenMenu
	}
	switch km.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(menuActions)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		if m.cursor == 0 {
			return m, screenInstances
		}
		return m, screenQuit
	}
	return m, screenMenu
}

func (m menuScreen) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("AWSX") + "\n\n")
	b.WriteString(fmt.Sprintf("Perfil: %s\n", m.profile))
	b.WriteString(fmt.Sprintf("Conta: %s\n", m.id.Account))
	if role := roleFromArn(m.id.Arn); role != "" {
		b.WriteString(fmt.Sprintf("Role: %s\n", role))
	}
	b.WriteString(fmt.Sprintf("Região: %s\n\n", m.region))
	for i, a := range menuActions {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		b.WriteString(cursor + a + "\n")
	}
	return b.String()
}

// roleFromArn extracts the role name from an assumed-role ARN, or "".
func roleFromArn(arn string) string {
	const marker = ":assumed-role/"
	i := strings.Index(arn, marker)
	if i < 0 {
		return ""
	}
	rest := arn[i+len(marker):]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		return rest[:slash]
	}
	return rest
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestMenuScreen|TestRoleFromArn' -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen_menu.go internal/tui/screen_menu_test.go
git commit -m "feat: tui main menu screen"
```

---

### Task 13: instancesScreen (filterable list)

**Files:**
- Create: `internal/tui/screen_instances.go`
- Test: `internal/tui/screen_instances_test.go`

**Interfaces:**
- Consumes: `aws.Target`, `aws.DisplayName` (Task 5); `bubbles/list`.
- Produces:
  - `type targetItem struct { t aws.Target }` implementing `list.Item`; `FilterValue()` concatenates name, ID, private IP, and type (PRD §6.10).
  - `type instancesScreen struct { list list.Model }`
  - `func newInstancesScreen(targets []aws.Target) instancesScreen`
  - `func (s instancesScreen) Update(msg tea.Msg) (instancesScreen, *aws.Target, tea.Cmd)` — non-nil target on Enter.
  - `func (s instancesScreen) View() string`

- [ ] **Step 1: Write the failing test**

`internal/tui/screen_instances_test.go`:
```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

func targets() []awsx.Target {
	return []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api-production", Type: "t3.large", PrivateIP: "10.0.1.15", State: "running"}, SSMOnline: true},
		{Instance: awsx.Instance{ID: "i-2", Name: "worker-production", Type: "t3.medium", PrivateIP: "10.0.1.22", State: "running"}, SSMOnline: true},
	}
}

func TestInstancesScreen_EnterSelects(t *testing.T) {
	s := newInstancesScreen(targets())
	s, _, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s, sel, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel == nil || sel.ID != "i-1" {
		t.Fatalf("want i-1 selected, got %+v", sel)
	}
}

func TestTargetItem_FilterValueIncludesIPAndID(t *testing.T) {
	it := targetItem{t: targets()[0]}
	fv := it.FilterValue()
	for _, want := range []string{"api-production", "i-1", "10.0.1.15", "t3.large"} {
		if !strings.Contains(fv, want) {
			t.Fatalf("FilterValue %q missing %q", fv, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestInstancesScreen|TestTargetItem' -v`
Expected: FAIL — `undefined: newInstancesScreen`.

- [ ] **Step 3: Implement**

`internal/tui/screen_instances.go`:
```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

type targetItem struct{ t awsx.Target }

func (i targetItem) Title() string { return awsx.DisplayName(i.t.Instance) }
func (i targetItem) Description() string {
	return fmt.Sprintf("%s   %s   %s   %s", i.t.State, i.t.Type, i.t.PrivateIP, i.t.ID)
}
func (i targetItem) FilterValue() string {
	return strings.Join([]string{
		awsx.DisplayName(i.t.Instance), i.t.ID, i.t.PrivateIP, i.t.Type,
	}, " ")
}

type instancesScreen struct {
	list list.Model
}

func newInstancesScreen(targets []awsx.Target) instancesScreen {
	items := make([]list.Item, len(targets))
	for i, t := range targets {
		items[i] = targetItem{t: t}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Selecione uma instância"
	return instancesScreen{list: l}
}

func (s instancesScreen) Update(msg tea.Msg) (instancesScreen, *awsx.Target, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.list.SetSize(ws.Width, ws.Height-2)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && !s.list.SettingFilter() {
		if it, ok := s.list.SelectedItem().(targetItem); ok {
			t := it.t
			return s, &t, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, nil, cmd
}

func (s instancesScreen) View() string { return s.list.View() }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestInstancesScreen|TestTargetItem' -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen_instances.go internal/tui/screen_instances_test.go
git commit -m "feat: tui filterable instance selection screen"
```

---

### Task 14: rootModel — wiring, screen delegation, session handoff

**Files:**
- Create: `internal/tui/root.go`
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: every screen + message + the boundary interfaces.
- Produces:
  - `type Deps struct { Profiles []profiles.Profile; Checks []deps.Dependency; NewClients func(ctx context.Context, profile string) (idp aws.IdentityProvider, ec2 aws.EC2Lister, ssm aws.SSMLister, region string, err error); NewCLI func(profile string) (aws.Login, aws.Sessioner) }` — injection seam; real wiring in Task 15, fakes in tests.
  - `type rootModel struct { ... current screen; sub-models; Deps; profile string; err ... }`
  - `func NewRoot(d Deps) rootModel`
  - `func (m rootModel) Init() tea.Cmd`
  - `func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)`
  - `func (m rootModel) View() string`
  - `func Run(d Deps) error` — `tea.NewProgram(NewRoot(d)).Run()`.

- [ ] **Step 1: Write the failing test (fake deps drive the happy path)**

`internal/tui/root_test.go`:
```go
package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
)

type fakeIDP struct{}

func (fakeIDP) WhoAmI(context.Context) (awsx.Identity, error) {
	return awsx.Identity{Account: "123", Arn: "arn:aws:sts::123:assumed-role/Admin/x"}, nil
}

type fakeEC2 struct{}

func (fakeEC2) RunningInstances(context.Context) ([]awsx.Instance, error) {
	return []awsx.Instance{{ID: "i-1", Name: "api", State: "running", Type: "t3.large", PrivateIP: "10.0.1.15"}}, nil
}

type fakeSSM struct{}

func (fakeSSM) OnlineInstanceIDs(context.Context) (map[string]bool, error) {
	return map[string]bool{"i-1": true}, nil
}

func fakeDeps() Deps {
	return Deps{
		Profiles: []profiles.Profile{{Name: "prod", IsSSO: true, Region: "us-east-1"}},
		Checks: []deps.Dependency{
			{Name: "AWS CLI", Binary: "aws", Found: true},
			{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
		},
		NewClients: func(context.Context, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, "us-east-1", nil
		},
	}
}

// drive applies a message and returns the concrete rootModel.
func drive(m tea.Model, msg tea.Msg) rootModel {
	next, _ := m.Update(msg)
	return next.(rootModel)
}

func TestRoot_DepsOKStartsAtProfiles(t *testing.T) {
	m := NewRoot(fakeDeps())
	// Init runs the deps check synchronously in NewRoot; with all found the
	// initial screen is the profile selector.
	if m.current != screenProfiles {
		t.Fatalf("current = %v, want screenProfiles", m.current)
	}
}

func TestRoot_MissingDepGoesToError(t *testing.T) {
	d := fakeDeps()
	d.Checks[0].Found = false
	m := NewRoot(d)
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
}

func TestRoot_SelectProfileThenIdentityShowsMenu(t *testing.T) {
	m := NewRoot(fakeDeps())
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select "prod" -> checkingScreen
	if m.current != screenChecking {
		t.Fatalf("after select current = %v, want screenChecking", m.current)
	}
	// identity command resolves via fake
	m = drive(m, identityMsg{id: awsx.Identity{Account: "123"}, region: "us-east-1"})
	if m.current != screenMenu {
		t.Fatalf("after identity current = %v, want screenMenu", m.current)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestRoot -v`
Expected: FAIL — `undefined: NewRoot`.

- [ ] **Step 3: Implement the root model**

`internal/tui/root.go`:
```go
package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
)

// Deps is the injection seam for the TUI. Real wiring lives in Task 15; tests
// supply fakes.
type Deps struct {
	Profiles   []profiles.Profile
	Checks     []deps.Dependency
	NewClients func(ctx context.Context, profile string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error)
	NewCLI     func(profile string) (awsx.Login, awsx.Sessioner)
}

type rootModel struct {
	deps    Deps
	current screen

	depsScreen      depsScreen
	profileScreen   profileScreen
	menuScreen      menuScreen
	instancesScreen instancesScreen
	errorScreen     errorScreen

	profile string
	region  string
	idp     awsx.IdentityProvider
	ec2     awsx.EC2Lister
	ssm     awsx.SSMLister
	login   awsx.Login
	session awsx.Sessioner

	width, height int
	quitting      bool
}

// NewRoot builds the initial model. The dependency check is evaluated eagerly:
// missing deps route straight to an error screen, otherwise the profile
// selector is shown.
func NewRoot(d Deps) rootModel {
	m := rootModel{deps: d}
	m.depsScreen = newDepsScreen(d.Checks)
	if !m.depsScreen.allFound() {
		m.current = screenError
		miss := m.depsScreen.missing()
		detail := deps.InstallHint(miss[0].Binary, deps.GOOS(), deps.BrewAvailable())
		m.errorScreen = newErrorScreen(
			miss[0].Name+" não encontrado.",
			detail,
			[]errorAction{{label: "Sair", next: screenQuit}},
		)
		return m
	}
	m.current = screenProfiles
	m.profileScreen = newProfileScreen(d.Profiles)
	return m
}

func (m rootModel) Init() tea.Cmd { return nil }

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
	case identityMsg:
		m.region = msg.region
		m.menuScreen = newMenuScreen(m.profile, msg.region, msg.id)
		m.current = screenMenu
		return m, nil
	case targetsMsg:
		m.instancesScreen = newInstancesScreen(msg.targets)
		m.instancesScreen, _, _ = m.instancesScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.current = screenInstances
		return m, nil
	case loginDoneMsg:
		if msg.err != nil {
			return m.toError("O login foi cancelado ou não foi concluído.", "", []errorAction{
				{label: "Tentar novamente", next: screenChecking},
				{label: "Escolher outro perfil", next: screenProfiles},
				{label: "Sair", next: screenQuit},
			}), nil
		}
		m.current = screenChecking
		return m, loadIdentityCmd(m.idp, m.region)
	case sessionEndedMsg:
		return m.toError("Sessão encerrada.", "", []errorAction{
			{label: "Voltar para as instâncias", next: screenInstances},
			{label: "Voltar para o menu principal", next: screenMenu},
			{label: "Sair", next: screenQuit},
		}), nil
	case errMsg:
		detail := ""
		if msg.action != "" {
			detail = "Permissão necessária: " + msg.action
		} else if msg.err != nil {
			detail = msg.err.Error()
		}
		return m.toError("Ocorreu um erro.", detail, nil), nil
	}

	return m.routeToScreen(msg)
}

// routeToScreen dispatches a message to the active screen and applies the
// screen transition it requests.
func (m rootModel) routeToScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current {
	case screenProfiles:
		var sel *profiles.Profile
		var cmd tea.Cmd
		m.profileScreen, sel, cmd = m.profileScreen.Update(msg)
		if sel != nil {
			return m.startChecking(*sel)
		}
		return m, cmd

	case screenMenu:
		var next screen
		m.menuScreen, next = m.menuScreen.Update(msg)
		switch next {
		case screenInstances:
			m.current = screenInstances
			return m, loadTargetsCmd(m.ec2, m.ssm)
		case screenQuit:
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case screenInstances:
		var sel *awsx.Target
		var cmd tea.Cmd
		m.instancesScreen, sel, cmd = m.instancesScreen.Update(msg)
		if sel != nil {
			id := sel.ID
			name := awsx.DisplayName(sel.Instance)
			return m, tea.ExecProcess(sessionExec(m.session, id, name), func(err error) tea.Msg {
				return sessionEndedMsg{err: err}
			})
		}
		return m, cmd

	case screenError:
		var next screen
		m.errorScreen, next = m.errorScreen.Update(msg)
		return m.applyErrorNext(next)
	}
	return m, nil
}

func (m rootModel) startChecking(p profiles.Profile) (tea.Model, tea.Cmd) {
	m.profile = p.Name
	idp, ec2c, ssmc, region, err := m.deps.NewClients(context.Background(), p.Name)
	if err != nil {
		return m.toError("Não foi possível preparar o perfil.", err.Error(), []errorAction{
			{label: "Escolher outro perfil", next: screenProfiles},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	m.idp, m.ec2, m.ssm, m.region = idp, ec2c, ssmc, region
	if m.deps.NewCLI != nil {
		m.login, m.session = m.deps.NewCLI(p.Name)
	}
	m.current = screenChecking
	// Try identity first; on failure the error path can route to login.
	return m, loadIdentityCmd(m.idp, m.region)
}

func (m rootModel) applyErrorNext(next screen) (tea.Model, tea.Cmd) {
	switch next {
	case screenError:
		return m, nil
	case screenQuit:
		m.quitting = true
		return m, tea.Quit
	case screenChecking:
		if m.login != nil {
			m.current = screenLogin
			return m, loginCmd(m.login)
		}
		m.current = screenChecking
		return m, loadIdentityCmd(m.idp, m.region)
	case screenProfiles:
		m.current = screenProfiles
		return m, nil
	case screenInstances:
		m.current = screenInstances
		return m, loadTargetsCmd(m.ec2, m.ssm)
	case screenMenu:
		m.current = screenMenu
		return m, nil
	}
	return m, nil
}

func (m rootModel) toError(title, detail string, actions []errorAction) rootModel {
	m.errorScreen = newErrorScreen(title, detail, actions)
	m.current = screenError
	return m
}

func (m rootModel) View() string {
	if m.quitting {
		return ""
	}
	switch m.current {
	case screenDeps:
		return m.depsScreen.View()
	case screenProfiles:
		return m.profileScreen.View()
	case screenChecking:
		return styleFaint.Render("Verificando sessão...")
	case screenLogin:
		return styleFaint.Render("Abrindo autenticação AWS...")
	case screenMenu:
		return m.menuScreen.View()
	case screenInstances:
		return m.instancesScreen.View()
	case screenError:
		return m.errorScreen.View()
	}
	return ""
}

// Run starts the Bubble Tea program with the given dependencies.
func Run(d Deps) error {
	_, err := tea.NewProgram(NewRoot(d), tea.WithAltScreen()).Run()
	return err
}
```

- [ ] **Step 4: Add the session exec helper**

Append to `internal/tui/messages.go`:
```go
// sessionExec builds the *exec.Cmd for an SSM session via the Sessioner. It is
// defined here so root.go can hand it to tea.ExecProcess for the TTY handoff.
```
Then create `internal/tui/session.go`:
```go
package tui

import (
	"os"
	"os/exec"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

// sessionExec returns an *exec.Cmd that, when run by tea.ExecProcess, hands the
// terminal to the SSM session. The Sessioner interface is bypassed here because
// tea.ExecProcess needs the concrete *exec.Cmd; we reconstruct it via the CLI's
// StartSession semantics.
func sessionExec(s awsx.Sessioner, instanceID, _ string) *exec.Cmd {
	if cw, ok := s.(commandWriter); ok {
		return cw.SessionCommand(instanceID)
	}
	// Fallback: direct aws invocation (used when a real CLI is wired).
	cmd := exec.Command("aws", "ssm", "start-session", "--target", instanceID)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// commandWriter lets a Sessioner expose its *exec.Cmd for tea.ExecProcess.
type commandWriter interface {
	SessionCommand(instanceID string) *exec.Cmd
}
```

Then extend `internal/aws/exec.go` so `CLI` satisfies `commandWriter` cleanly (add method):
```go
// SessionCommand builds the start-session command with the TTY wired to the
// current process, for use with tea.ExecProcess.
func (c CLI) SessionCommand(instanceID string) *exec.Cmd {
	cmd := exec.Command("aws", sessionArgs(c.Profile, instanceID)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestRoot -v && go build ./...`
Expected: PASS (all three) and clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go internal/tui/session.go internal/tui/messages.go internal/aws/exec.go
git commit -m "feat: root tui model wiring screens and ssm session handoff"
```

---

### Task 15: Wire the TUI into the Cobra root

**Files:**
- Modify: `internal/cli/root.go`
- Test: manual smoke (the TUI needs a TTY; no automated test).

**Interfaces:**
- Consumes: `tui.Run`, `tui.Deps`; `profiles.Parse`, `config.ConfigPath`; `deps.Check`; `aws.NewClients`, `aws.CLI`.

- [ ] **Step 1: Rewire RunE to build real deps and run the TUI**

Replace the `RunE` in `internal/cli/root.go` with:
```go
		RunE: func(cmd *cobra.Command, args []string) error {
			ps, err := profiles.Parse(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("read profiles: %w", err)
			}
			if len(ps) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(),
					"Nenhum perfil AWS foi encontrado. Configure a AWS CLI (aws configure sso) antes de continuar.")
				return nil
			}
			return tui.Run(tui.Deps{
				Profiles: ps,
				Checks:   deps.Check(),
				NewClients: func(ctx context.Context, profile string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error) {
					c, region, err := awsx.NewClients(ctx, profile)
					if err != nil {
						return nil, nil, nil, "", err
					}
					return c, c, c, region, nil
				},
				NewCLI: func(profile string) (awsx.Login, awsx.Sessioner) {
					cli := awsx.CLI{Profile: profile}
					return cli, cli
				},
			})
		},
```

Update the imports block of `internal/cli/root.go`:
```go
import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/config"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
	"github.com/davidsgoncalves/awsx/internal/tui"
)
```

- [ ] **Step 2: Verify existing cli test still passes + build**

Run: `go test ./internal/cli/ -v && go build -o ./bin/awsx ./cmd/awsx`
Expected: PASS; binary at `./bin/awsx`.

- [ ] **Step 3: Manual smoke (requires a real AWS SSO profile)**

Run: `./bin/awsx`
Expected: deps show ✓✓; profile list appears; selecting a valid profile either shows the menu (valid session) or, when implemented end-to-end, the identity resolves. Verify `Ctrl+C` quits cleanly. (SSO login and the real SSM handoff are validated here manually — they cannot be unit-tested.)

- [ ] **Step 4: Commit**

```bash
git add internal/cli/root.go
git commit -m "feat: wire tui into the awsx root command"
```

---

### Task 16: Session-expired routing (identity failure -> login)

**Files:**
- Modify: `internal/tui/root.go` (identity error handling)
- Test: `internal/tui/root_test.go` (add case)

**Interfaces:**
- Consumes: `errMsg`, `loginCmd`, `screenLogin` (existing).
- Produces: on an identity `errMsg` while a `Login` is available, route to `screenLogin` and run `loginCmd`, instead of the generic error screen.

**Rationale:** PRD §6.6 — an invalid/expired session should trigger `aws sso login`, not an error. The current `errMsg` handler shows a generic error; this task makes identity failures during the checking phase route to login.

- [ ] **Step 1: Add the failing test**

Add to `internal/tui/root_test.go`:
```go
type failIDP struct{}

func (failIDP) WhoAmI(context.Context) (awsx.Identity, error) {
	return awsx.Identity{}, context.DeadlineExceeded // stand-in for expired/invalid
}

type fakeLogin struct{ called *bool }

func (f fakeLogin) SSOLogin(context.Context) error { *f.called = true; return nil }

func TestRoot_IdentityFailureWithLoginGoesToLogin(t *testing.T) {
	called := false
	d := fakeDeps()
	d.NewClients = func(context.Context, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error) {
		return failIDP{}, fakeEC2{}, fakeSSM{}, "us-east-1", nil
	}
	d.NewCLI = func(string) (awsx.Login, awsx.Sessioner) { return fakeLogin{called: &called}, nil }

	m := NewRoot(d)
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select profile -> checking
	// identity fails during checking:
	next, cmd := m.Update(errMsg{err: context.DeadlineExceeded})
	m = next.(rootModel)
	if m.current != screenLogin {
		t.Fatalf("current = %v, want screenLogin", m.current)
	}
	if cmd == nil {
		t.Fatal("expected a login command")
	}
	_ = cmd() // execute the login cmd; fake sets called=true
	if !called {
		t.Fatal("login was not invoked")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestRoot_IdentityFailureWithLogin -v`
Expected: FAIL — lands on `screenError`, not `screenLogin`.

- [ ] **Step 3: Implement the routing**

In `internal/tui/root.go`, replace the `case errMsg:` block in `Update` with:
```go
	case errMsg:
		// During the checking phase, an identity failure means the session is
		// expired/invalid: route to SSO login instead of a generic error.
		if m.current == screenChecking && m.login != nil {
			m.current = screenLogin
			return m, loginCmd(m.login)
		}
		detail := ""
		if msg.action != "" {
			detail = "Permissão necessária: " + msg.action
		} else if msg.err != nil {
			detail = msg.err.Error()
		}
		return m.toError("Ocorreu um erro.", detail, nil), nil
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/tui/ -run TestRoot -v`
Expected: PASS (all four root tests, including the new one).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat: route expired-session identity failure to sso login"
```

---

### Task 17: Empty-targets and no-profile guidance

**Files:**
- Modify: `internal/tui/root.go` (handle empty `targetsMsg`)
- Test: `internal/tui/root_test.go` (add case)

**Interfaces:**
- Consumes: `targetsMsg`, `errorScreen`.
- Produces: an empty `targetsMsg` routes to an error screen listing PRD §9 reasons, with actions back to the menu / quit.

- [ ] **Step 1: Add the failing test**

Add to `internal/tui/root_test.go`:
```go
func TestRoot_NoTargetsShowsGuidance(t *testing.T) {
	m := NewRoot(fakeDeps())
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, targetsMsg{targets: nil})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	if !strings.Contains(m.errorScreen.View(), "Nenhuma instância") {
		t.Fatalf("view missing empty message: %q", m.errorScreen.View())
	}
}
```
(Add `"strings"` to the test imports if not present.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestRoot_NoTargets -v`
Expected: FAIL — lands on `screenInstances` with an empty list.

- [ ] **Step 3: Implement**

In `internal/tui/root.go`, replace the `case targetsMsg:` block with:
```go
	case targetsMsg:
		if len(msg.targets) == 0 {
			return m.toError(
				"Nenhuma instância EC2 disponível via SSM foi encontrada.",
				"Possíveis motivos: nenhuma instância em execução; SSM Agent desconectado; instância sem IAM Role para SSM; perfil sem permissão; região sem instâncias.",
				[]errorAction{
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		m.instancesScreen = newInstancesScreen(msg.targets)
		m.instancesScreen, _, _ = m.instancesScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.current = screenInstances
		return m, nil
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/tui/ -run TestRoot -v`
Expected: PASS (all root tests).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat: guide the user when no ssm targets are available"
```

---

### Task 18: Full test + build gate

**Files:** none (verification task).

- [ ] **Step 1: Run the whole suite**

Run: `go test ./... -v`
Expected: all packages PASS.

- [ ] **Step 2: Vet + build all targets**

Run:
```bash
go vet ./...
for t in "darwin amd64" "darwin arm64" "linux amd64" "linux arm64"; do
  set -- $t
  GOOS=$1 GOARCH=$2 go build -o /dev/null ./cmd/awsx && echo "ok $1/$2"
done
```
Expected: `go vet` clean; four `ok` lines.

- [ ] **Step 3: Commit any fixups**

```bash
git add -A
git commit -m "chore: pass full test, vet, and cross-build gate" || echo "nothing to commit"
```

---

## Self-Review Notes

- **Spec coverage:** deps detection (T4/T10), profile listing + SSO-first (T2/T11), region resolution (T3), session check + SSO login (T6/T7/T14/T16), main menu (T12), EC2×SSM listing (T5/T6/T13/T17), SSM handoff + return menu (T7/T14), error mapping incl. denied IAM action (T8/T9/T14), no-profile guidance (T15), no-targets guidance (T17), security/no-cred (boundary reads only; no logging of secrets — enforced by never printing SDK inputs), timeouts (T8). Region-missing error surfaces via `NewClients` (T6) → `startChecking` error path (T14).
- **Not covered here (Phase 2):** CI, GoReleaser, install.sh, Homebrew tap, community files, README. Separate plan.
- **Deferred/manual:** real `sso login` browser flow and real `start-session` TTY handoff (T15 step 3) — cannot be unit-tested.
- **Type consistency:** `screen` enum defined once (T9) and consumed everywhere; boundary interfaces defined once (T5); `Deps.NewClients` returns `(IdentityProvider, EC2Lister, SSMLister, region, error)` and is called that way in T14/T15.
