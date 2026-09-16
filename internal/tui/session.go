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

// execWithCapture runs cmd via tea.ExecProcess, capturing its stderr (alongside
// the terminal) so a failure message survives the TUI redraw and reaches the
// error screen and log.
func execWithCapture(cmd *exec.Cmd) tea.Cmd {
	buf := &strings.Builder{}
	if cmd.Stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, buf)
	} else {
		cmd.Stderr = buf
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return sessionEndedMsg{err: err, stderr: strings.TrimSpace(buf.String())}
	})
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
