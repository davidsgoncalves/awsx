package tui

import (
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
