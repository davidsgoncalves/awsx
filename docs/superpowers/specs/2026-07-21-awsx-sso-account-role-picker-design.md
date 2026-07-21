# AWSX — Interactive SSO Account/Role/Region Picker (Design)

Date: 2026-07-21
Status: Approved (brainstorming), pending implementation

## Goal

Let a user start from an IAM Identity Center **SSO session** (`[sso-session]` in
`~/.aws/config`) and pick **account → role → region** interactively, without
needing a pre-configured `[profile]` for each account/role/region combination.
Authentication still delegates to `aws sso login`; AWSX does not implement SSO.

## Scope

- **In:** modern `[sso-session]` blocks. From one, enumerate accounts and roles
  via the SSO portal API, pick a region from a built-in list, then list EC2/SSM
  and open a session exactly as today.
- **Out (unchanged, existing path):** fully-configured `[profile]` entries and
  legacy inline-SSO profiles (`sso_start_url` directly in a profile) keep going
  straight to the current session-check flow.
- The profile-selection screen lists both: SSO sessions (which branch into the
  new pickers) and regular profiles (which behave as before).

## Key decisions

1. **Credentials — ephemeral temp profile (never touch raw creds).** For the
   chosen account/role/region, AWSX writes an ephemeral profile to a temporary
   `AWS_CONFIG_FILE` in a temp dir (NEVER `~/.aws/config`). Both the SDK (via
   `WithSharedConfigFiles`) and the `aws ssm start-session` child (via the
   `AWS_CONFIG_FILE` env var) resolve SSO credentials from the cached token
   themselves. AWSX only *reads* the SSO token to enumerate accounts/roles —
   which the PRD explicitly permits (`~/.aws/sso/cache`) — and never handles the
   raw access key / secret / session token. The temp dir is removed on exit.

2. **Region — built-in static list.** A curated list of public AWS regions,
   presented in a filterable list. No extra IAM permission; may include regions
   not enabled for the account (acceptable tradeoff vs. `ec2:DescribeRegions`).

3. **Account/role enumeration via SSO portal API.** With the cached access token
   AWSX calls `sso:ListAccounts` and `sso:ListAccountRoles`. If the token is
   missing or expired, it runs `aws sso login --sso-session <name>` first, then
   re-reads the token.

## Flow

```
deps -> selection screen
  |
  +-- regular / legacy-sso profile --> (current flow: checking -> [login] -> menu)
  |
  +-- [sso-session] selected
        -> ensure token (aws sso login --sso-session NAME if missing/expired)
        -> sso:ListAccounts        -> account picker (filterable)
        -> sso:ListAccountRoles(a) -> role picker (filterable)
        -> region picker (static list, filterable)
        -> write ephemeral temp profile (temp AWS_CONFIG_FILE)
        -> build SDK config from it -> menu (identity via sts) -> EC2 -> SSM session
```

## New components

- `internal/profiles`: parse `[sso-session]` blocks →
  `type SSOSession struct { Name, StartURL, Region string }`; a combined
  selection list of profiles + sso-sessions.
- `internal/aws/sso.go`: token read + `ListAccounts` / `ListAccountRoles` behind
  a `SSODiscoverer` interface. Types `Account{ID, Name}`, `Role{Name}`.
- `internal/aws/ephemeral.go`: write the temp profile config, return
  `(configPath, profileName, cleanup func())`; helper to build an
  `aws.Config`/clients from it, and to set `AWS_CONFIG_FILE` for the SSM child.
- `internal/aws/regions.go`: static region list.
- `internal/tui`: `accountScreen`, `roleScreen`, `regionScreen`; root wiring to
  branch on session-vs-profile selection and thread account/role/region.

## Security notes

- AWSX still never persists or prints credentials/tokens. It reads the SSO token
  only to list accounts/roles. Raw credentials are resolved by the CLI/SDK from
  the token, not by AWSX.
- The ephemeral config lives in a `os.MkdirTemp` dir and is removed on exit; it
  contains no secrets (only account id, role name, region, and a reference to
  the sso-session).
- `~/.aws/config` is never modified.

## Out of scope

- `ec2:DescribeRegions`-based region discovery (chose static list).
- Caching the last account/role/region (future: V1.1 favorites/history).
- Legacy inline-SSO profile interactive enumeration.
