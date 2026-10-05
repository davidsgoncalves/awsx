package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/update"
)

// execWithCapture runs cmd via tea.Exec, capturing its stderr (alongside
// the terminal) so a failure message survives the TUI redraw and reaches the
// error screen and log.
func execWithCapture(cmd *exec.Cmd) tea.Cmd {
	buf := &strings.Builder{}
	if cmd.Stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, buf)
	} else {
		cmd.Stderr = buf
	}
	return tea.Exec(pauseAfter(cmd), func(err error) tea.Msg {
		return sessionEndedMsg{err: err, stderr: strings.TrimSpace(buf.String())}
	})
}

// finishedPrompt is printed when the child process exits, before the TUI takes
// the terminal back and the alternate screen hides the output.
const finishedPrompt = "\nFinalizado. Aperte enter para fechar."

// pausedCmd runs a child process and then waits for enter, so its last lines
// stay readable until the user dismisses them.
type pausedCmd struct {
	cmd *exec.Cmd
	in  io.Reader
	out io.Writer
}

// pauseAfter wraps cmd for tea.Exec. Streams already set on cmd are kept.
func pauseAfter(cmd *exec.Cmd) *pausedCmd {
	return &pausedCmd{cmd: cmd, in: cmd.Stdin, out: cmd.Stdout}
}

func (p *pausedCmd) SetStdin(r io.Reader) {
	if p.cmd.Stdin == nil {
		p.cmd.Stdin = r
	}
	if p.in == nil {
		p.in = r
	}
}

func (p *pausedCmd) SetStdout(w io.Writer) {
	if p.cmd.Stdout == nil {
		p.cmd.Stdout = w
	}
	if p.out == nil {
		p.out = w
	}
}

func (p *pausedCmd) SetStderr(w io.Writer) {
	if p.cmd.Stderr == nil {
		p.cmd.Stderr = w
	}
}

// Run runs the child process, then blocks until enter. The child's error is
// returned whether or not it succeeded.
func (p *pausedCmd) Run() error {
	err := p.cmd.Run()
	if p.out != nil {
		_, _ = fmt.Fprintln(p.out, finishedPrompt)
	}
	if p.in != nil {
		waitForEnter(p.in)
	}
	return err
}

// waitForEnter reads until a line break or EOF. Both CR and LF count, since a
// child such as session-manager-plugin can leave the terminal in raw mode.
func waitForEnter(r io.Reader) {
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n == 1 && (b[0] == '\n' || b[0] == '\r') {
			return
		}
		if err != nil {
			return
		}
	}
}

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

// portForwarder lets a Sessioner expose an SSM port-forward *exec.Cmd.
type portForwarder interface {
	PortForwardCommand(instanceID, host string, remotePort, localPort int) *exec.Cmd
}

// portForwardExec returns the *exec.Cmd that opens the SSM tunnel, for
// tea.ExecProcess. Falls back to a direct aws invocation if the Sessioner does
// not expose PortForwardCommand.
func portForwardExec(s awsx.Sessioner, instanceID, host string, remotePort, localPort int) *exec.Cmd {
	if pf, ok := s.(portForwarder); ok {
		return pf.PortForwardCommand(instanceID, host, remotePort, localPort)
	}
	cmd := exec.Command("aws", "ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", fmt.Sprintf("host=%s,portNumber=%d,localPortNumber=%d", host, remotePort, localPort),
	)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// ecsExecCommander lets a Sessioner expose an `aws ecs execute-command`
// *exec.Cmd.
type ecsExecCommander interface {
	ECSExecCommand(cluster, task, container, command string) *exec.Cmd
}

// ecsExec returns the *exec.Cmd that opens an ECS Exec session inside the
// container, for tea.ExecProcess. Falls back to a direct aws invocation if the
// Sessioner does not expose ECSExecCommand.
func ecsExec(s awsx.Sessioner, cluster, task, container, command string) *exec.Cmd {
	if ec, ok := s.(ecsExecCommander); ok {
		return ec.ECSExecCommand(cluster, task, container, command)
	}
	cmd := exec.Command("aws", "ecs", "execute-command",
		"--cluster", cluster,
		"--task", task,
		"--container", container,
		"--interactive",
		"--command", command,
	)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// brewUpgradeExec returns the *exec.Cmd that upgrades the Homebrew cask, for
// tea.ExecProcess: brew writes its progress straight to the terminal.
func brewUpgradeExec() *exec.Cmd {
	cmd := exec.Command("brew", update.BrewUpgradeArgs()...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

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
