package tui

import (
	"fmt"
	"os"
	"os/exec"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

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
