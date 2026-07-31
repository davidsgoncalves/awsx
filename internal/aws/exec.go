package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// CLI shells out to the AWS CLI for the two operations the SDK cannot do:
// interactive SSO login and the terminal-owning SSM session. When ConfigFile is
// set, commands run with AWS_CONFIG_FILE pointing at it, so the CLI resolves SSO
// credentials from an ephemeral profile without touching ~/.aws/config. Region,
// when set, is passed to start-session so it works for profiles that have no
// region configured.
type CLI struct {
	Profile    string
	Region     string
	ConfigFile string
}

func loginArgs(profile string) []string {
	return []string{"sso", "login", "--profile", profile}
}

func sessionArgs(profile, region, instanceID string) []string {
	args := []string{"ssm", "start-session", "--profile", profile}
	if region != "" {
		args = append(args, "--region", region)
	}
	return append(args, "--target", instanceID)
}

func ssoSessionLoginArgs(session string) []string {
	return []string{"sso", "login", "--sso-session", session}
}

func portForwardArgs(profile, region, instanceID, host string, remotePort, localPort int) []string {
	args := []string{"ssm", "start-session", "--profile", profile}
	if region != "" {
		args = append(args, "--region", region)
	}
	return append(args,
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", fmt.Sprintf("host=%s,portNumber=%d,localPortNumber=%d", host, remotePort, localPort),
	)
}

// interactiveCommandArgs builds the start-session invocation that runs a single
// command with a TTY attached. Parameters are JSON-encoded rather than using
// the AWS CLI key=value shorthand: the shorthand splits values on commas, which
// would silently break any command containing one.
func interactiveCommandArgs(profile, region, instanceID, command string) []string {
	args := []string{"ssm", "start-session", "--profile", profile}
	if region != "" {
		args = append(args, "--region", region)
	}
	params, err := json.Marshal(map[string][]string{"command": {command}})
	if err != nil {
		// Marshalling a map of strings cannot fail; fall back to the raw
		// command rather than dropping the parameter entirely.
		params = []byte(`{"command":[""]}`)
	}
	return append(args,
		"--target", instanceID,
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", string(params),
	)
}

// env returns the environment for a child command: the inherited environment
// plus AWS_CONFIG_FILE when ConfigFile is set, or nil to inherit unchanged.
func (c CLI) env() []string {
	if c.ConfigFile == "" {
		return nil
	}
	return append(os.Environ(), "AWS_CONFIG_FILE="+c.ConfigFile)
}

// SSOLogin runs `aws sso login`, streaming its output so the user sees the
// browser/device-code prompts. Honors ctx cancellation.
func (c CLI) SSOLogin(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "aws", loginArgs(c.Profile)...)
	cmd.Env = c.env()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// StartSession runs `aws ssm start-session`, handing over stdin/stdout/stderr
// so the session-manager-plugin takes control of the terminal.
func (c CLI) StartSession(instanceID string) error {
	return c.SessionCommand(instanceID).Run()
}

// SessionCommand builds the start-session command with the TTY wired to the
// current process, for use with tea.ExecProcess.
func (c CLI) SessionCommand(instanceID string) *exec.Cmd {
	cmd := exec.Command("aws", sessionArgs(c.Profile, c.Region, instanceID)...)
	cmd.Env = c.env()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}

// PortForwardCommand builds an SSM port-forward-to-remote-host command with the
// TTY wired to the current process, for use with tea.ExecProcess. It tunnels
// localPort on the loopback interface to host:remotePort through instanceID.
func (c CLI) PortForwardCommand(instanceID, host string, remotePort, localPort int) *exec.Cmd {
	cmd := exec.Command("aws", portForwardArgs(c.Profile, c.Region, instanceID, host, remotePort, localPort)...)
	cmd.Env = c.env()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}

// InteractiveCommand builds an SSM start-session command that runs command on
// instanceID with a TTY, with the terminal wired to the current process for use
// with tea.ExecProcess.
func (c CLI) InteractiveCommand(instanceID, command string) *exec.Cmd {
	cmd := exec.Command("aws", interactiveCommandArgs(c.Profile, c.Region, instanceID, command)...)
	cmd.Env = c.env()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}

// SSOSessionLogin runs `aws sso login --sso-session NAME` for the interactive
// SSO-session flow, where no single profile is chosen up front.
type SSOSessionLogin struct {
	Session string
}

// SSOLogin implements Login.
func (l SSOSessionLogin) SSOLogin(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "aws", ssoSessionLoginArgs(l.Session)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
