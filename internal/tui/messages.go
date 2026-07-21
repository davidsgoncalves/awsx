package tui

import (
	"context"
	"errors"
	"time"

	"github.com/aws/smithy-go"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
)

const awsTimeout = 15 * time.Second

type identityMsg struct {
	id     awsx.Identity
	region string
}
type targetsMsg struct{ targets []awsx.Target }
type loginDoneMsg struct{ err error }
type sessionEndedMsg struct{ err error }

// errMsg carries a failed operation. action, when set, is the denied IAM action
// (e.g. "ec2:DescribeInstances") extracted from the SDK error.
type errMsg struct {
	err    error
	action string
}

// deniedAction returns the IAM action from an access-denied SDK error, or "".
func deniedAction(err error, fallback string) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation":
			return fallback
		}
	}
	return ""
}

func loadIdentityCmd(p awsx.IdentityProvider, region string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		id, err := p.WhoAmI(ctx)
		if err != nil {
			return errMsg{err: err}
		}
		return identityMsg{id: id, region: region}
	}
}

func loadTargetsCmd(ec2 awsx.EC2Lister, ssm awsx.SSMLister) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		instances, err := ec2.RunningInstances(ctx)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ec2:DescribeInstances")}
		}
		online, err := ssm.OnlineInstanceIDs(ctx)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ssm:DescribeInstanceInformation")}
		}
		return targetsMsg{targets: awsx.Join(instances, online)}
	}
}

func loginCmd(l awsx.Login) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return loginDoneMsg{err: l.SSOLogin(ctx)}
	}
}
