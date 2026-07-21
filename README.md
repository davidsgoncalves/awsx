# AWSX

AWSX is an interactive terminal CLI for authenticating to AWS and accessing EC2
instances through AWS Systems Manager (SSM) Session Manager. Run a single
command and pick a profile, then an instance — no instance IDs, roles, or
`aws ssm start-session` invocations to remember.

```
awsx
```

## What it does

1. Checks that the AWS CLI and the Session Manager Plugin are installed.
2. Lists your local AWS profiles (IAM Identity Center / SSO profiles first).
3. Validates the session and runs `aws sso login` when it has expired.
4. Shows a menu, lists running EC2 instances that are reachable via SSM, and
   opens an interactive session in the one you pick.

## Install

### macOS / Linux (script)

```sh
curl -fsSL https://raw.githubusercontent.com/davidsgoncalves/awsx/main/scripts/install.sh | sh
```

Installs to `/usr/local/bin/awsx`, or `~/.local/bin/awsx` when you can't write
to `/usr/local/bin` (make sure that directory is on your `PATH`).

### Homebrew

```sh
brew install davidsgoncalves/tap/awsx
```

### From source

```sh
go install github.com/davidsgoncalves/awsx/cmd/awsx@latest
```

## Dependencies

AWSX relies on two external tools at runtime:

- **AWS CLI v2** — used for `aws sso login` and to open the SSM session.
- **Session Manager Plugin** — required for interactive SSM sessions.

If either is missing, AWSX detects it and shows how to install it for your
platform.

## Configure AWS SSO

AWSX reads your existing AWS config; it never creates or stores credentials. If
you don't have a profile yet:

```sh
aws configure sso
```

A profile needs a `region` (AWSX uses the profile's region, or `AWS_REGION` if
set, and errors clearly when neither is configured).

## Usage

```sh
awsx
```

- Arrow keys to move, `Enter` to select, type to filter, `Esc` to go back,
  `q` / `Ctrl+C` to quit.
- When an SSM session ends you return to the menu.
- Debug logging (never includes credentials or tokens):

  ```sh
  AWSX_DEBUG=true awsx
  ```

## Required IAM permissions

```
ec2:DescribeInstances
ssm:DescribeInstanceInformation
ssm:StartSession
ssm:TerminateSession
ssm:DescribeSessions
ssm:GetConnectionStatus
```

Starting a session may also require access to the
`arn:aws:ssm:*:*:document/SSM-SessionManagerRunShell` document.

The target instance must have an IAM role that allows the SSM Agent to
register, and the agent must be running and online.

## EC2 access via SSM

AWSX lists only instances that are `running` **and** online in SSM. The
instance name comes from the `Name` tag, falling back to the instance ID. Under
the hood it cross-references `ec2:DescribeInstances` with
`ssm:DescribeInstanceInformation` and opens the session with
`aws ssm start-session`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports and pull requests are
welcome.

## Releasing

Releases are cut by GoReleaser via GitHub Actions on a version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The pipeline runs tests, builds the four platform binaries, generates
`checksums.txt`, creates the GitHub Release, and updates the Homebrew tap.

## License

[MIT](LICENSE) © 2026 David Gonçalves
