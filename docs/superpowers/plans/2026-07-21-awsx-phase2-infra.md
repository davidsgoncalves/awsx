# AWSX Phase 2 (Infra & Release) Implementation Plan

> **For agentic workers:** Follow task-by-task. Infra tasks are declarative; each verifies with a real command (`goreleaser check`, `shellcheck`, dry-run) before moving on. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make AWSX publicly installable — CI on every push/PR, a tagged GoReleaser pipeline producing cross-platform binaries + checksums + a GitHub Release + a Homebrew tap formula, a `curl | sh` installer, and the community files a public repo needs. Then publish `github.com/davidsgoncalves/awsx` and cut `v0.1.0`.

**Architecture:** GoReleaser is the release engine, driven by a tag via GitHub Actions. CI is a separate lightweight workflow. `install.sh` resolves the latest release from the GitHub API, downloads the matching archive, verifies its checksum, and installs the binary. The Homebrew tap lives in a second repo (`davidsgoncalves/homebrew-tap`) that GoReleaser writes to using a PAT-equivalent token.

**Tech Stack:** GoReleaser v2, GitHub Actions, golangci-lint, shellcheck (validation), POSIX sh (installer).

## Global Constraints

- Repo: `github.com/davidsgoncalves/awsx` (public). Tap repo: `github.com/davidsgoncalves/homebrew-tap` (public).
- Artifact names (PRD §15): `awsx_Darwin_x86_64.tar.gz`, `awsx_Darwin_arm64.tar.gz`, `awsx_Linux_x86_64.tar.gz`, `awsx_Linux_arm64.tar.gz`, `checksums.txt`.
- Build targets: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64.
- LICENSE: MIT, "Copyright (c) 2026 David Gonçalves".
- Install dirs: `/usr/local/bin/awsx`, or `~/.local/bin/awsx` without root.
- All docs/comments/commit messages in English.
- Publishing (repo creation, push, tag) is authorized by the user for this run.

---

### Task 0: Local validation tooling

- [ ] **Step 1:** Install tooling for local verification.
  Run: `brew install goreleaser golangci-lint shellcheck`
  Expected: all three resolve; `goreleaser --version`, `golangci-lint --version`, `shellcheck --version` work.

---

### Task 1: Community & documentation files

**Files:** Create `LICENSE`, `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`.

- [ ] **Step 1:** Write `LICENSE` (MIT, 2026 David Gonçalves).
- [ ] **Step 2:** Write `README.md` covering PRD §18: what it is, install (curl + brew), dependencies, configuring AWS SSO, usage, required IAM permissions, EC2-via-SSM, contributing, cutting a release.
- [ ] **Step 3:** Write `CONTRIBUTING.md` (build, test, lint, PR flow).
- [ ] **Step 4:** Write `CODE_OF_CONDUCT.md` (Contributor Covenant).
- [ ] **Step 5:** Write `SECURITY.md` (no-credential design, how to report).
- [ ] **Step 6:** Commit.
  Run: `git add LICENSE README.md CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md && git commit -m "docs: add license and community files"`

---

### Task 2: Lint config + CI workflow

**Files:** Create `.golangci.yml`, `.github/workflows/ci.yml`.

- [ ] **Step 1:** Write `.golangci.yml` (enable govet, staticcheck, errcheck, ineffassign, unused; conservative).
- [ ] **Step 2:** Verify lint passes locally.
  Run: `golangci-lint run ./...`
  Expected: no issues (fix any that surface).
- [ ] **Step 3:** Write `.github/workflows/ci.yml`: on push/PR — setup Go 1.23, `go test ./...`, `golangci-lint`, `go build ./...`.
- [ ] **Step 4:** Commit.
  Run: `git add .golangci.yml .github/workflows/ci.yml && git commit -m "ci: add lint config and ci workflow"`

---

### Task 3: GoReleaser config

**Files:** Create `.goreleaser.yaml`.

- [ ] **Step 1:** Write `.goreleaser.yaml`: version 2; builds for the 4 targets (CGO off); archives named per PRD (`{{.ProjectName}}_{{title .Os}}_{{.Arch}}` with amd64→x86_64 replacement); `checksums.txt`; `brews` block targeting `davidsgoncalves/homebrew-tap`; changelog from git.
- [ ] **Step 2:** Validate the config.
  Run: `goreleaser check`
  Expected: "config is valid".
- [ ] **Step 3:** Dry-run a full build (no publish).
  Run: `goreleaser release --snapshot --clean --skip=publish`
  Expected: `dist/` contains the 4 archives with the correct names + `checksums.txt`.
- [ ] **Step 4:** Commit.
  Run: `git add .goreleaser.yaml && git commit -m "build: add goreleaser config"`

---

### Task 4: Release workflow

**Files:** Create `.github/workflows/release.yml`.

- [ ] **Step 1:** Write `.github/workflows/release.yml`: on tag `v*` — checkout (full history), setup Go 1.23, run tests, run GoReleaser with `GITHUB_TOKEN` and `HOMEBREW_TAP_GITHUB_TOKEN`.
- [ ] **Step 2:** Commit.
  Run: `git add .github/workflows/release.yml && git commit -m "ci: add goreleaser release workflow"`

---

### Task 5: Install script

**Files:** Create `scripts/install.sh`.

- [ ] **Step 1:** Write `scripts/install.sh`: detect OS (Darwin/Linux) + arch (x86_64/arm64), resolve latest release tag via GitHub API, download the matching archive + `checksums.txt`, verify the checksum, extract, install to `/usr/local/bin` (or `~/.local/bin` without write access), print result. POSIX sh, `set -e`.
- [ ] **Step 2:** Lint it.
  Run: `shellcheck scripts/install.sh`
  Expected: no warnings (fix any).
- [ ] **Step 3:** Make executable + commit.
  Run: `chmod +x scripts/install.sh && git add scripts/install.sh && git commit -m "feat: add curl-based install script"`

---

### Task 6: Publish

- [ ] **Step 1:** Create the public repos.
  Run: `gh repo create davidsgoncalves/awsx --public --source=. --remote=origin --push` and `gh repo create davidsgoncalves/homebrew-tap --public --description "Homebrew tap for awsx"`
  Expected: both repos exist; `main` pushed to `awsx`.
- [ ] **Step 2:** Set the tap token secret (reuse the gh token, which has `repo` scope).
  Run: `gh secret set HOMEBREW_TAP_GITHUB_TOKEN --repo davidsgoncalves/awsx --body "$(gh auth token)"`
  Expected: secret set.
- [ ] **Step 3:** Tag and push to trigger the release.
  Run: `git tag v0.1.0 && git push origin v0.1.0`
- [ ] **Step 4:** Watch the release run.
  Run: `gh run watch --repo davidsgoncalves/awsx $(gh run list --repo davidsgoncalves/awsx --workflow release.yml --limit 1 --json databaseId -q '.[0].databaseId')`
  Expected: success; GitHub Release with 4 archives + checksums; formula pushed to the tap.
- [ ] **Step 5:** Verify install paths.
  Run: `curl -fsSL https://raw.githubusercontent.com/davidsgoncalves/awsx/main/scripts/install.sh | sh` and `brew install davidsgoncalves/tap/awsx`
  Expected: `awsx` on PATH from both.

---

## Self-Review Notes
- Covers PRD §14 (install), §15 (builds/artifacts), §16 (releases), §17 (repo files), §18 (README).
- Token: the gh CLI token has `repo` scope, so it can push the formula to the tap repo; used as `HOMEBREW_TAP_GITHUB_TOKEN`.
- Risk: the first release is irreversible (public). Mitigated by `goreleaser check` + `--snapshot` dry-run before tagging.
