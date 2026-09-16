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
4. Shows a menu: open an interactive session on an EC2 instance, tunnel to a
   database, run a command inside a container on the instance, reach an ECS
   container, or update itself.

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
ssm:SendCommand
ssm:GetCommandInvocation
rds:DescribeDBInstances
ecs:ListClusters
ecs:ListTasks
ecs:DescribeTasks
ecs:DescribeContainerInstances
ecs:ExecuteCommand
```

Starting a session may also require access to the
`arn:aws:ssm:*:*:document/SSM-SessionManagerRunShell`,
`AWS-StartInteractiveCommand`, and `AWS-RunShellScript` documents.

The target instance must have an IAM role that allows the SSM Agent to
register, and the agent must be running and online. For the ECS flow, the task
role needs `ssmmessages:CreateControlChannel`, `CreateDataChannel`,
`OpenControlChannel`, and `OpenDataChannel`.

## EC2 access via SSM

AWSX lists only instances that are `running` **and** online in SSM. The
instance name comes from the `Name` tag, falling back to the instance ID. Under
the hood it cross-references `ec2:DescribeInstances` with
`ssm:DescribeInstanceInformation` and opens the session with
`aws ssm start-session`.

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

## ECS

The **ECS** menu entry walks from the cluster down to the container, without
choosing a node:

1. AWSX lists the ECS clusters in the region and skips the picker when there is
   only one.
2. It lists the running tasks with `ecs:ListTasks` and `ecs:DescribeTasks`,
   showing each container by its service name with the EC2 instance it landed
   on. On a cluster whose nodes share one name, that is what tells them apart.
3. The chosen task offers three ways in:

| Action | What it opens |
| --- | --- |
| **Rodar comando** | The command screen, with the same per-service history as the Docker flow, then `aws ecs execute-command --interactive` |
| **Shell no container** | `bash` inside the container, falling back to `sh` for images that do not ship it |
| **Acessar host (SSM)** | `aws ssm start-session` on the instance the task landed on |

`Acessar host (SSM)` is left out for Fargate tasks, which run on no instance of
yours. The first two actions need ECS Exec on the service; the host session
works without it.

ECS Exec execs the command directly, with no shell and a minimal PATH, so AWSX
wraps what you type in `/bin/sh -c '...'` and prepends the working directory's
`bin` to PATH, which is where a Rails image keeps its binstubs. `rails c` and
`rake db:migrate:status` work as typed. The wrapped line is shown on the command
screen before it runs.

The command screen opens with `rails c` already filled in, so the whole flow is
menu, service, enter. Once something else has been run against that service, the
last command comes back instead.

A task whose service has `enableExecuteCommand` turned off is refused with that
name in the message: the setting is applied at deploy time, so it has to be
changed on the service and rolled out before the container accepts a session.
The host session stays available meanwhile.

## Updating

The **Atualizar AWSX** menu entry compares the running version against the
latest GitHub release. The check happens when you open the entry, and AWSX
makes no network call for it before that.

What `Enter` does depends on how this copy was installed:

- **Release archive** (`scripts/install.sh` or a manual download): AWSX
  downloads the archive for your platform, verifies it against `checksums.txt`,
  and swaps its own binary through a temporary file in the same directory.
  Without write permission there, the screen shows the installer command to run
  under `sudo`.
- **Homebrew**: runs `brew upgrade --cask davidsgoncalves/tap/awsx`, so the tap
  stays consistent with what is on disk.
- **`go install`**: shows `go install github.com/davidsgoncalves/awsx/cmd/awsx@latest`,
  which is how the toolchain replaces builds it owns.

`awsx --version` prints the same version. Release builds carry the tag; a local
`go build` reports `dev`.

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
`checksums.txt`, creates the GitHub Release, and updates the Homebrew tap. The
tag is stamped into the binary at link time and is what `awsx --version` and the
update check report.

## License

[MIT](LICENSE) © 2026 David Gonçalves
