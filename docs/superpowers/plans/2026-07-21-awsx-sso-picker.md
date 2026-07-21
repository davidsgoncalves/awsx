# AWSX SSO Account/Role/Region Picker — Implementation Plan

> Follow task-by-task, TDD. Each task ends green + committed. Steps use `- [ ]`.

**Goal:** Add an interactive account → role → region flow starting from an
`[sso-session]`, using an ephemeral temp profile so AWSX never handles raw
credentials. Additive; the existing profile flow is untouched.

**Tech:** aws-sdk-go-v2 `service/sso`, `credentials/ssocreds`; existing TUI/SDK.

## Global Constraints (inherit Phase 1 constraints)
- Interactive path targets `[sso-session]` blocks only.
- AWSX reads the SSO token only to enumerate accounts/roles; never handles raw creds.
- Ephemeral profile written to a temp dir (never `~/.aws/config`); removed on exit.
- New screens reuse `bubbles/list` (filterable), matching existing screens.

---

### Task 1: Parse `[sso-session]` blocks
**Files:** `internal/profiles/profiles.go`, `internal/profiles/profiles_test.go`
**Produces:** `type SSOSession struct { Name, StartURL, Region string }`;
`func ParseSSOSessions(configPath string) ([]SSOSession, error)` (sorted by name;
empty on missing file).
- [ ] Test: config with `[sso-session vakinha]` (sso_start_url, sso_region) →
  one SSOSession with those fields.
- [ ] Run (fail), implement (iterate sections, `CutPrefix(name, "sso-session ")`,
  read `sso_start_url`/`sso_region`), run (pass), commit.

---

### Task 2: Static region list
**Files:** `internal/aws/regions.go`, `internal/aws/regions_test.go`
**Produces:** `func Regions() []string` — curated public AWS regions.
- [ ] Test: list is non-empty, contains "us-east-1" and "sa-east-1", is sorted,
  has no duplicates.
- [ ] Implement, run, commit.

---

### Task 3: SSO discovery (token + ListAccounts + ListAccountRoles)
**Files:** `internal/aws/sso.go`, `internal/aws/sso_test.go`
**Produces:**
- `type Account struct { ID, Name string }`, `type Role struct { Name string }`
- `type SSODiscoverer interface { Accounts(ctx) ([]Account, error); Roles(ctx, accountID string) ([]Role, error) }`
- `func readSSOToken(cacheDir, sessionName string, now time.Time) (token string, err error)` —
  reads `<sha1hex(sessionName)>.json` (use `ssocreds.StandardCachedTokenFilepath`);
  error `ErrTokenExpiredOrMissing` if absent or `expiresAt` <= now.
- `var ErrTokenExpiredOrMissing = errors.New(...)`
- `type SSOClient struct{...}` implementing `SSODiscoverer` (SDK `sso` client + token).
- `func NewSSOClient(ctx, session SSOSession, now time.Time) (*SSOClient, error)`.
- [ ] Test `readSSOToken`: write a temp cache file with future `expiresAt` → returns
  token; past `expiresAt` → `ErrTokenExpiredOrMissing`; missing file → same error.
  (Compute the filename via `ssocreds.StandardCachedTokenFilepath(sessionName)` but
  point it at a temp HOME via `t.Setenv("HOME", dir)`.)
- [ ] Run (fail), implement, run (pass), commit. (Accounts/Roles themselves are
  thin SDK wrappers — verified live in the smoke task, not unit-tested.)

---

### Task 4: Ephemeral temp profile
**Files:** `internal/aws/ephemeral.go`, `internal/aws/ephemeral_test.go`
**Produces:**
- `type Ephemeral struct { ConfigPath, Profile string; cleanup func() }`
- `func WriteEphemeralProfile(session SSOSession, accountID, roleName, region string) (*Ephemeral, error)` —
  `os.MkdirTemp`, write a config file with `[sso-session NAME]` + `[profile _awsx]`
  (sso_account_id, sso_role_name, region), return paths + cleanup.
- `func (e *Ephemeral) Close() error` — remove the temp dir.
- `func (e *Ephemeral) Clients(ctx) (*Clients, string, error)` — `LoadDefaultConfig`
  with `WithSharedConfigFiles([ConfigPath])` + `WithSharedConfigProfile(Profile)`,
  returns clients + region (reuses the Clients builder from sdk.go).
- [ ] Test `WriteEphemeralProfile`: file exists, contains `[profile _awsx]`,
  `sso_account_id = <id>`, `sso_role_name = <role>`, `region = <region>`, and the
  `[sso-session NAME]` block; `Close()` removes the dir.
- [ ] Run (fail), implement, run (pass), commit.

**Refactor note:** extract the client construction in `sdk.go` into
`newClientsFromConfig(cfg aws.Config) *Clients` so both `NewClients` and
`Ephemeral.Clients` share it.

---

### Task 5: Exec — sso-session login + ephemeral session env
**Files:** `internal/aws/exec.go`, `internal/aws/exec_test.go`
**Produces:**
- Extend `CLI` with `ConfigFile string`; when set, `SessionCommand`/`StartSession`
  run with `AWS_CONFIG_FILE=<ConfigFile>` appended to `os.Environ()`.
- `type SSOSessionLogin struct { Session string }` implementing `Login`:
  `aws sso login --sso-session <Session>`.
- `func ssoSessionLoginArgs(session string) []string` → `["sso","login","--sso-session",session]`.
- [ ] Test: `ssoSessionLoginArgs("vakinha")` equals expected; a `CLI{Profile:"_awsx",
  ConfigFile:"/tmp/x"}` `SessionCommand("i-1")` has `AWS_CONFIG_FILE=/tmp/x` in `.Env`
  and argv `ssm start-session --profile _awsx --target i-1`.
- [ ] Run (fail), implement, run (pass), commit.

---

### Task 6: TUI account/role/region screens
**Files:** `internal/tui/screen_pick.go`, `internal/tui/screen_pick_test.go`
**Produces:** three filterable list screens sharing one generic item type:
- `type accountScreen`, `roleScreen`, `regionScreen` each with
  `newXScreen(items)`, `Update(msg) (X, *selection, tea.Cmd)`, `View()`.
  (Selection returns `*aws.Account` / `*aws.Role` / `*string` respectively.)
- [ ] Test each: after `WindowSizeMsg` + `Enter`, returns the first item.
- [ ] Run (fail), implement (mirror `screen_profiles.go` pattern), run (pass), commit.

---

### Task 7: Selection screen lists sso-sessions too
**Files:** `internal/tui/screen_profiles.go`, `internal/tui/screen_profiles_test.go`
**Produces:** `newSelectionScreen(profiles, ssoSessions)` returning items of a sum
type; `Update` returns `(*profiles.Profile, *profiles.SSOSession)` — exactly one
non-nil on Enter. SSO sessions labeled e.g. `vakinha  (SSO session)` and sorted
first. Keep `newProfileScreen` behavior via the combined screen.
- [ ] Test: with one session + one profile, selecting the session returns the
  session (profile nil); selecting the profile returns the profile.
- [ ] Run (fail), implement, run (pass), commit.

---

### Task 8: Root wiring for the SSO flow
**Files:** `internal/tui/root.go`, `internal/tui/messages.go`, `internal/tui/root_test.go`
**Produces:**
- Extend `Deps` with:
  - `SSOSessions []profiles.SSOSession`
  - `NewDiscoverer func(ctx, session profiles.SSOSession) (aws.SSODiscoverer, error)`
  - `NewEphemeral func(session profiles.SSOSession, accountID, roleName, region string) (idp aws.IdentityProvider, ec2 aws.EC2Lister, ssm aws.SSMLister, login aws.Login, sess aws.Sessioner, region2 string, cleanup func(), err error)`
  - `NewSSOLogin func(session profiles.SSOSession) aws.Login`
- New screens/states: `screenAccounts`, `screenRoles`, `screenRegion`.
- New messages: `accountsMsg{[]aws.Account}`, `rolesMsg{[]aws.Role}`, and cmds
  `loadAccountsCmd`, `loadRolesCmd`.
- Flow: session selected → ensure token via discoverer (if it errors with
  `ErrTokenExpiredOrMissing`, run sso-session login then retry) → accounts →
  roles → region → build ephemeral clients → `screenChecking` → identity → menu.
- Store `cleanup` on the model; call it on quit (`tea.Quit` paths).
- [ ] Tests (fakes): selecting a session with a fake discoverer returning 1
  account → `screenAccounts`; picking account → `screenRoles`; picking role →
  `screenRegion`; picking region builds ephemeral (fake) → `screenChecking`;
  identity → `screenMenu`. Assert cleanup is invoked on quit.
- [ ] Run (fail), implement, run (pass), commit.

---

### Task 9: Wire real deps in cli/root.go
**Files:** `internal/cli/root.go`
**Produces:** populate the new `Deps` fields with real constructors
(`profiles.ParseSSOSessions`, `aws.NewSSOClient`, `aws.WriteEphemeralProfile` +
`Ephemeral.Clients`, `aws.SSOSessionLogin`, `CLI{Profile:"_awsx", ConfigFile:...}`).
- [ ] Build; existing cli test passes; commit.

---

### Task 10: Gate + live smoke
**Files:** extend `internal/aws/smoke_test.go`
- [ ] Add `TestSmoke_SSODiscovery` (guarded by `AWSX_SMOKE_SSO_SESSION`): build
  `NewSSOClient`, list accounts, list roles for the first account; log counts.
- [ ] `go test ./...`, `go vet ./...`, `golangci-lint run ./...`, cross-build 4 targets.
- [ ] Live: `AWSX_SMOKE_SSO_SESSION=vakinha go test ./internal/aws/ -run TestSmoke_SSODiscovery -v`.
- [ ] Commit.

---

## Self-Review Notes
- Credentials: AWSX reads the token (allowed) to enumerate; raw creds resolved by
  SDK/CLI from the ephemeral profile — never handled by AWSX.
- Additive: existing profile path and its tests unchanged.
- Manual TUI run still required for full end-to-end (account/role/region nav +
  real SSM handoff).
