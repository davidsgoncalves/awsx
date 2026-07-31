# AWSX — Run a Command in a Container ("Rodar comando") (Design)

Date: 2026-07-31
Status: Approved (brainstorming), pending implementation

## Goal

Let a user run an arbitrary command inside a Docker container on an EC2
instance, without leaving AWSX and without remembering instance IDs, container
names, or `aws ssm start-session` invocations. The driving case is `rails c` on
an application container, but nothing in the design is Rails-specific: any
command, any container.

## Scope

- **In:** EC2 instances reachable via SSM that run containers under Docker /
  Docker Compose. Pick instance → pick container → type command → get an
  interactive TTY inside the container.
- **Out:** ECS (EC2 or Fargate), EKS, `docker compose exec` / `run`, a dedicated
  logs screen, command presets, running one command across several instances.

## Runtime assumptions

Containers run under Docker Compose directly on the EC2 instance. Compose
creates ordinary Docker containers, so `docker ps` lists them and
`docker exec -it <name> <cmd>` reaches them from any working directory. The
design therefore uses `docker exec`, not `docker compose exec`:

- no dependency on the working directory (no need to locate a compose file),
- no dependency on the Compose version (`docker-compose` v1 vs `docker compose`
  v2 plugin),
- the target name comes from `docker ps` itself, so it cannot be wrong.

`sudo` prefixes the `docker` call because the SSM session runs as `ssm-user`,
which is not in the `docker` group and cannot reach `/var/run/docker.sock`. The
SSM Agent grants `ssm-user` `NOPASSWD:ALL` by default, so this does not block on
a password prompt.

**Rejected: `sudo su - ubuntu -c "docker exec ..."`.** Switching users buys
nothing here — `docker exec` addresses the container by name, and the process
environment comes from the container's own `ENTRYPOINT`/`WORKDIR`, not from the
caller. It costs two things: a second shell level (so every quote in a
user-typed command needs double escaping, breaking `rails runner "puts
User.count"`), and a hardcoded username that does not exist on Amazon Linux
(`ec2-user`) or on custom AMIs.

## User flow

A fourth entry in the main menu, after the tunnel:

```
Acessar EC2
Acessar banco/serviço (túnel)
Rodar comando
Sair
```

```
menu → [instances]   reuses the existing instance list screen
     → [containers]  AWSX runs `docker ps` remotely and lists the result
     → [command]     free-text input, prefilled with the last command used
     → interactive `docker exec` (TTY handed to the terminal)
     → session ends → back to the command screen
```

Returning to the **command** screen (not the menu) after a session ends means
swapping `rails c` for `bash` costs one keystroke, not a full re-selection.

### Container list

`docker ps` is asked for the Compose service label so the list can lead with a
readable name:

```
web
  myapp-web-1 · ruby:3.2 · Up 3 days
```

Title is the Compose service (`web`) when the label is present, otherwise the
container name. Description is always `name · image · status`. Filter matches
service, name, and image.

### Command screen

```
Rodar comando em web (myapp-web-1)

  > rails c

  sudo docker exec -it myapp-web-1 rails c

  enter executa · tab edita a linha toda · esc volta
```

- The input holds the **in-container** command; the grey line below is the full
  command that will run.
- `tab` toggles to editing that full line, for instances whose setup does not
  match the `sudo docker exec -it` assumption. Toggling back restores the
  in-container field.
- `↑`/`↓` walk the command history for this container.
- The preview and the executed command are produced by the same function, so
  the preview cannot drift from reality.

### Command history

`internal/state` gains `Commands map[string][]string`, keyed by
`container:<service>` — the Compose service when present, the container name
otherwise. The service is the stable key: the container name carries a
Compose-assigned suffix (`-1`) that changes when the container is recreated.

Five entries per key, most recent first, deduplicated. The most recent one
prefills the input. Persistence is best-effort, matching the existing region
memory: a write failure is logged, never fatal.

## Architecture

### `internal/aws`

```go
// types.go
type Container struct {
    ID      string
    Name    string // myapp-web-1
    Service string // compose label com.docker.compose.service; may be empty
    Image   string
    Status  string
}

type ContainerLister interface {
    Containers(ctx context.Context, instanceID string) ([]Container, error)
}
```

**`containers.go` (new)** implements `Containers` on the existing `*Clients`,
reusing its `ssm` client. It issues `ssm:SendCommand` with the
`AWS-RunShellScript` document running

```sh
docker ps --format '{{.ID}}\t{{.Names}}\t{{.Label "com.docker.compose.service"}}\t{{.Image}}\t{{.Status}}'
```

then polls `ssm:GetCommandInvocation` every 500ms until the invocation leaves
`Pending`/`InProgress`, with a 20s ceiling. A non-`Success` status surfaces the
invocation's `StandardErrorContent`.

`SendCommand` runs as root, so `docker ps` needs no `sudo` here — only the
interactive `docker exec`, which runs as `ssm-user`, does.

**`docker.go` (new)** holds the pure, AWS-free logic:

- `parsePS(out string) []Container` — tab-separated parsing, tolerant of empty
  fields and malformed lines.
- `DockerExecLine(container, cmd string) string` → `sudo docker exec -it <container> <cmd>`.
  Single source of truth for both the preview and the executed command.

**`exec.go`** gains `InteractiveCommand(instanceID, command string) *exec.Cmd`,
a sibling of the existing `PortForwardCommand`:

```
aws ssm start-session --profile P [--region R] --target i-abc \
  --document-name AWS-StartInteractiveCommand \
  --parameters '{"command":["sudo docker exec -it myapp-web-1 rails c"]}'
```

`--parameters` is passed as **JSON**, not as the `key=value` shorthand: the
shorthand splits on commas, so `rails runner "puts [1,2]"` would silently become
two parameters. `AWS-StartInteractiveCommand` (rather than `ssm:SendCommand`) is
what gives a real TTY, which `rails c` requires.

### `internal/tui`

Three states appended after `screenTunnelInstance`, preserving the numeric
values existing tests assert on: `screenExecInstance`, `screenContainers`,
`screenCommand`.

- **`screen_pick.go`** — container list added alongside accounts/roles/rds/
  regions, following the same `list.Model` shape.
- **`screen_command.go` (new)** — the only screen with a `textinput` and its own
  mode state (in-container vs full line) and history cursor.
- **`messages.go`** — `containersMsg`, `loadContainersCmd(ContainerLister, instanceID)`,
  with `deniedAction(err, "ssm:SendCommand")` for the access-denied case.
- **`session.go`** — `interactiveExec(Sessioner, instanceID, command)`, following
  the existing optional-interface pattern (`commandWriter`, `portForwarder`) plus
  the direct-`aws` fallback. Runs through `execWithCapture` so stderr survives
  the TUI redraw.
- **`root.go`** — new routing, the client wiring below, and the flow refactor
  below.

### Client wiring

`*Clients` already holds the SSM client, so it implements `ContainerLister` for
free. What changes is the injection seam: `Deps.NewClients` and
`Deps.NewEphemeral` each return one more value, the `ContainerLister`, and
`rootModel` stores it next to `ec2`/`ssm`/`rds`.

That pushes `NewClients` to seven return values and `NewEphemeral` to nine —
both already long. The alternative, collapsing them into a struct, touches every
factory and every fake in `root_test.go` for no behavioral gain, so it stays out
of this change. Worth revisiting if a fifth client ever shows up.

### Flow refactor (`m.tunneling` → `m.flow`)

`rootModel.tunneling bool` currently decides three things: the instance list's
title and target screen, the error title when a session fails, and where Esc
returns. A third consumer makes a boolean unable to express the state. It
becomes:

```go
type flow int
const (
    flowSession flow = iota // Acessar EC2
    flowTunnel              // Acessar banco/serviço
    flowExec                // Rodar comando
)
```

Roughly six call sites in `root.go`. No behavior change to the existing two
flows — this is a rename plus a third case.

## Error handling

Every failure lands on the existing error screen with a way out, never a dead
end:

| Situation | Message and actions |
|---|---|
| `ssm:SendCommand` denied | Names the missing permission; offers "digitar o comando à mão", which opens the command screen in full-line mode |
| `docker ps` returns nothing | "Nenhum container em execução nessa instância"; back to instances / menu |
| Docker missing or `docker ps` fails | The instance's real stderr, as the port-forward flow already does |
| Poll exceeds 20s | "A instância não respondeu"; points at the log path |
| Interactive session fails | Captured stderr, target, region, log path — same shape as the SSM session error |
| Session ends normally | "Sessão encerrada"; back to command / containers / menu |

## IAM

Two permissions beyond today's set:

```
ssm:SendCommand
ssm:GetCommandInvocation
```

plus access to the `AWS-RunShellScript` and `AWS-StartInteractiveCommand`
documents. README's permission list and a new "Running commands in containers"
section are updated accordingly.

## Testing

Following the project's existing shape — one `*_test.go` per file, fakes
injected through `tui.Deps`:

- `docker_test.go` — `parsePS` with and without the Compose label, empty output,
  empty fields, malformed lines; `DockerExecLine` output.
- `exec_test.go` — `AWS-StartInteractiveCommand` args, including JSON encoding of
  commands containing commas and quotes.
- `containers_test.go` — poll loop against a fake SSM: success, non-`Success`
  status, timeout.
- `state_test.go` — history push, dedupe, five-entry cap, ordering.
- `screen_command_test.go` — preview matches `DockerExecLine`, `tab` toggle both
  ways, `↑`/`↓` history navigation, Enter emits the full line.
- `screen_pick_test.go` — container selection, service-vs-name title fallback.
- `root_test.go` — menu → instance → container → command → exec end to end, plus
  the denied-permission fallback path.

## Open questions

None.
