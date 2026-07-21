package aws

import (
	"context"
	"os"
	"os/exec"
)

// CLI shells out to the AWS CLI for the two operations the SDK cannot do:
// interactive SSO login and the terminal-owning SSM session.
type CLI struct {
	Profile string
}

func loginArgs(profile string) []string {
	return []string{"sso", "login", "--profile", profile}
}

func sessionArgs(profile, instanceID string) []string {
	return []string{"ssm", "start-session", "--profile", profile, "--target", instanceID}
}

// SSOLogin runs `aws sso login`, streaming its output so the user sees the
// browser/device-code prompts. Honors ctx cancellation.
func (c CLI) SSOLogin(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "aws", loginArgs(c.Profile)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// StartSession runs `aws ssm start-session`, handing over stdin/stdout/stderr
// so the session-manager-plugin takes control of the terminal.
func (c CLI) StartSession(instanceID string) error {
	cmd := exec.Command("aws", sessionArgs(c.Profile, instanceID)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
