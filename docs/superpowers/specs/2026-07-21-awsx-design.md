# AWSX — Design Doc

Date: 2026-07-21
Owner: davidsgoncalves
Status: Approved (brainstorming), pending implementation plan

## 1. What it is

AWSX is an interactive terminal CLI (Go) that collapses the manual "pick profile →
check session → SSO login → find instance id → check SSM → start-session" flow into a
single command: `awsx`. It targets developers/DevOps/SRE using AWS IAM Identity Center
and EC2 with SSM Session Manager. Public GitHub repo, MIT, macOS + Linux.

The full product requirements live in the PRD (source of truth for the *what*). This
document records the *how* — the architectural decisions made during brainstorming.

## 2. Design principles

- **Portable by construction.** No assumptions about the machine it runs on. OS/arch,
  package managers, PATH, region and profiles are all discovered at runtime from the
  user's environment. No hardcoded paths (e.g. `/opt/homebrew/...`) or region defaults.
- **Never own credentials.** AWSX reads the official AWS CLI SSO cache (via the SDK) and
  delegates login/session to the CLI. It never stores, prints, or logs credentials/tokens.
- **Don't mutate the user's AWS config.** `~/.aws/config` is never modified by AWSX. The
  only writes are those the AWS CLI itself makes to its cache during `sso login`.

## 3. Key decision — hybrid AWS integration

Confirmed during brainstorming. Three-way split:

| Operation | Mechanism | Why |
|---|---|---|
| `sts get-caller-identity` | **SDK v2** | Typed identity/error; detects expired session cleanly |
| `ec2:DescribeInstances` | **SDK v2** | Typed `[]types.Instance`; no JSON string parsing |
| `ssm:DescribeInstanceInformation` | **SDK v2** | Typed online-instance set for the EC2×SSM join |
| `aws sso login --profile X` | **exec (shell-out)** | Reuses official OIDC device flow + browser + cache; owning SSO is explicitly out of PRD scope |
| `aws ssm start-session` | **exec (shell-out)** | session-manager-plugin must take over the TTY; not expressible via SDK |

Rationale: shell-out only where inevitable; SDK where typed data and typed errors matter
(the EC2×SSM join and the "permission denied on `ec2:DescribeInstances`" error handling
in PRD §9). The SDK reads the SSO cache automatically via
`config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))` — it consumes the
token the CLI wrote; it does not reimplement SSO.

Rejected: pure shell-out (fragile JSON parsing + string-matched errors, hard to test);
pure SDK (impossible — start-session needs the TTY handoff, sso login is out of scope).

## 4. Region resolution (V1)

From the selected profile's `region` in `~/.aws/config` (honoring `AWS_REGION` if set, per
normal SDK resolution). If no region resolves, show a clear error asking the user to
configure one. **No fixed fallback** (would silently query the wrong region). Interactive
region selection is V1.1, out of V1 scope.

## 5. Architecture & package layout

```
cmd/awsx/main.go        entrypoint; wires real deps and runs the TUI

internal/
  aws/                  AWS boundary (the key layer)
    identity.go         sts get-caller-identity (SDK)      -> Identity
    ec2.go              describe-instances (SDK)           -> []Instance
    ssm.go              describe-instance-information (SDK) -> set of online IDs
    inventory.go        joins EC2 x SSM                     -> []Target (SSM-capable only)
    login.go            exec: aws sso login --profile X
    session.go          exec: aws ssm start-session (TTY handoff)
  profiles/             parse ~/.aws/config (list profiles, flag/prioritize SSO)
  deps/                 detect aws CLI + session-manager-plugin; guided install
  tui/                  Bubble Tea: root model + per-screen sub-models
  config/               resolve region/profile, AWSX_DEBUG
```

- **Cobra, thin root, now.** Root with no subcommand runs the TUI (V1 behavior). ~15 lines,
  keeps V1.2 subcommands (`ec2`, `whoami`, `doctor`) pluggable without refactor. Only the
  root ships in V1.
- **Interfaces at the boundary:** `Identity`, `EC2Lister`, `SSMLister`, `Login`, `Sessioner`.
  The TUI depends on interfaces, not on the SDK/exec directly → mockable without touching
  AWS or spawning processes.
- **Profile parsing** with `gopkg.in/ini.v1`: reads `[profile x]` sections, flags SSO
  profiles (`sso_session`/`sso_start_url`) to sort them to the top of the menu.

## 6. TUI flow & screen modeling

A `rootModel` holds shared state (deps status, selected profile, identity, region) and
delegates to a **sub-model per screen**. Uses `bubbles/list` (native filtering = the search
the PRD wants) for profiles and instances, and `bubbles/spinner` for loading states.

Screens (mirror PRD §5 flow):

```
depsScreen      "Verificando ambiente..."; missing aws/plugin -> install action
profileScreen   filterable list; SSO profiles on top
checkingScreen  sts get-caller-identity (spinner)
loginScreen     runs aws sso login; waits; "Abrindo autenticação..."
menuScreen      header (profile/account/role/region) + [Acessar EC2 / Sair]
instancesScreen filterable list of targets (Name tag as label, id as fallback)
errorScreen     message + contextual action list (retry/back/quit) — reusable
```

Three decisions that determine quality:

1. **AWS calls are async via `tea.Cmd`.** Each query runs off the render loop and returns a
   typed message (`identityMsg`, `targetsMsg`, `errMsg`). UI never blocks; spinner runs
   while pending. Timeouts via `context.WithTimeout` (PRD §20).
2. **SSM session uses `tea.ExecProcess`.** Bubble Tea suspends the TUI, hands the TTY to
   `aws ssm start-session` (plugin takes over), and resumes when the session ends, emitting a
   "session ended" msg → the return menu (back to instances / main menu / quit). Required for
   an interactive session to work inside a TUI.
3. **`errorScreen` is generic** — receives `(message, []action)`. All PRD §9 errors (CLI
   missing, permission denied showing the exact action like `ec2:DescribeInstances`, no
   instances, session-open failure) route through it with contextual actions.

Global nav: arrows + Enter (list), type = filter, Esc = back one screen, `q`/`Ctrl+C` = quit.

## 7. Dependency detection & install (`deps/`)

Runtime detection, no machine assumptions:
- Detect OS (`darwin`/`linux`) and arch (`amd64`/`arm64`).
- Look for `aws` and `session-manager-plugin` on the user's PATH.
- Suggest `brew install` **only if** brew exists on that machine; otherwise fall back to the
  official AWS CLI v2 installer (Linux) / Session Manager Plugin instructions.
- Any `sudo` command is shown explicitly and requires confirmation. If automatic install
  isn't possible, print clear instructions and exit.

## 8. Error mapping & security

- **Error mapping (SDK → screen):** capture `smithy.APIError`; on access-denied codes,
  extract the action (`ec2:DescribeInstances`, `ssm:DescribeInstanceInformation`) and route
  to `errorScreen` with the exact permission — typed, no string matching. `sts` failing with
  an invalid/expired token routes to `loginScreen` instead of an error.
- **Security:** never write/print credentials or tokens; only *read* the official cache via
  the SDK. `AWSX_DEBUG=true` enables verbose logging through a filter that never emits `AWS_*`,
  tokens, or `~/.aws/sso/cache` contents — metadata only (profile, region, instance, error
  code). `~/.aws/config` is never modified.

## 9. Testing strategy (TDD)

- `inventory.go` EC2×SSM join: pure table tests, no AWS.
- Interfaces enable fakes: inject a lister returning fixed instances or an `AccessDenied`
  error and assert `errorScreen` shows the right permission.
- Bubble Tea sub-models tested via `teatest`/`catwalk` (send `tea.KeyMsg`, assert `View()`),
  or plain `Update()` assertions where sufficient.
- Not automatable (documented manual test): the real `start-session` TTY handoff (needs live
  AWS + plugin) and the `sso login` browser flow.

## 10. Build, release, distribution

- **GoReleaser** builds `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64` +
  `checksums.txt`, with PRD §15 artifact names (`awsx_Darwin_arm64.tar.gz`, etc.).
- **`.github/workflows/ci.yml`** (PR/push): `go test ./...`, `golangci-lint`, `go build`.
- **`.github/workflows/release.yml`** (tag `vX.Y.Z`): test, lint, GoReleaser (binaries +
  GitHub Release + Homebrew tap update).
- **`scripts/install.sh`**: detect OS+arch, resolve latest release via GitHub API, download the
  right binary, **validate checksum**, install to `/usr/local/bin` (or `~/.local/bin` without
  root), report result.
- Repo files (PRD §17): `README.md`, `LICENSE` (MIT), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `SECURITY.md`, `.goreleaser.yaml`, `go.mod`.
- **Owner:** `davidsgoncalves` → `github.com/davidsgoncalves/awsx`, tap
  `davidsgoncalves/homebrew-tap`.

## 11. Phasing

- **Phase 1 — application:** deps detection, profile listing, SSO login, session check, main
  menu, EC2 listing + SSM filtering, SSM session handoff, return menu. Working locally.
- **Phase 2 — infra:** CI, GoReleaser, install.sh, Homebrew tap, community files, README.

## 12. Out of scope (V1)

Per PRD §21: SSH/PEM, ECS Exec, EKS, RDS/S3/Lambda/CloudWatch/Secrets Manager, port
forwarding, file transfer, Windows, multi-region simultaneous, favorites, connection history,
remote logout, IAM management, auto profile creation, own SSO/OIDC implementation.

## 13. Local build prerequisite

Go is not currently installed on the dev machine. `brew install go` is needed before
building/testing (to be confirmed with the user at implementation time). AWS CLI v2 and
session-manager-plugin are present.
