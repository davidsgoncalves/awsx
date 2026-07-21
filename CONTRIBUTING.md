# Contributing to AWSX

Thanks for your interest in improving AWSX. This guide covers the basics.

## Prerequisites

- Go 1.23 or newer
- AWS CLI v2 and the Session Manager Plugin (for manual end-to-end testing)

## Development

Clone and build:

```sh
git clone https://github.com/davidsgoncalves/awsx.git
cd awsx
go build ./cmd/awsx
```

Run the test suite, vet, and lint before opening a pull request:

```sh
go test ./...
go vet ./...
golangci-lint run ./...
```

There is an env-guarded live integration test that exercises the real AWS
boundary (read-only). It is skipped unless you opt in:

```sh
AWSX_SMOKE_PROFILE=<your-profile> go test ./internal/aws/ -run TestSmoke_Boundary -v
```

## Guidelines

- Follow the existing package structure and style; keep files focused.
- Write tests for new behavior. Business logic lives behind small interfaces so
  it can be tested without touching AWS.
- Never log or print credentials or tokens.
- Write code, comments, and commit messages in English.

## Pull requests

1. Branch off `main`.
2. Keep commits focused; make sure `go test`, `go vet`, and `golangci-lint`
   pass.
3. Describe the change and how you tested it.

## Reporting issues

Open a GitHub issue with your OS/arch, the AWSX version (`awsx version` once
available, or the release tag), and steps to reproduce. Do not include
credentials, tokens, or account identifiers.
