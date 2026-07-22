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

// SSO account/role discovery messages.
type needLoginMsg struct{}
type discovererReadyMsg struct{ d awsx.SSODiscoverer }
type accountsMsg struct{ accounts []awsx.Account }
type rolesMsg struct{ roles []awsx.Role }
type rdsMsg struct{ dbs []awsx.RDSInstance }

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

// initDiscovererCmd builds the SSO discoverer. A missing/expired token yields
// needLoginMsg so the caller can trigger sso-session login and retry.
func initDiscovererCmd(newDisc func(ctx context.Context) (awsx.SSODiscoverer, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		d, err := newDisc(ctx)
		if err != nil {
			if errors.Is(err, awsx.ErrTokenExpiredOrMissing) {
				return needLoginMsg{}
			}
			return errMsg{err: err}
		}
		return discovererReadyMsg{d: d}
	}
}

func loadAccountsCmd(d awsx.SSODiscoverer) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		accts, err := d.Accounts(ctx)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "sso:ListAccounts")}
		}
		return accountsMsg{accounts: accts}
	}
}

func loadRDSCmd(r awsx.RDSLister) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		dbs, err := r.RDSInstances(ctx)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "rds:DescribeDBInstances")}
		}
		return rdsMsg{dbs: dbs}
	}
}

func loadRolesCmd(d awsx.SSODiscoverer, accountID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		roles, err := d.Roles(ctx, accountID)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "sso:ListAccountRoles")}
		}
		return rolesMsg{roles: roles}
	}
}
