package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/smithy-go"
	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/update"
)

const awsTimeout = 15 * time.Second

// containerTimeout covers the SendCommand + poll cycle behind a container list.
const containerTimeout = 30 * time.Second

// downloadTimeout covers fetching and verifying a release archive.
const downloadTimeout = 2 * time.Minute

type identityMsg struct {
	id     awsx.Identity
	region string
}
type targetsMsg struct{ targets []awsx.Target }
type loginDoneMsg struct{ err error }
type sessionEndedMsg struct {
	err error
	// stderr holds the captured error output of the child process (used for
	// port-forward failures, where the message would otherwise be lost).
	stderr string
}

// SSO account/role discovery messages.
type needLoginMsg struct{}
type discovererReadyMsg struct{ d awsx.SSODiscoverer }
type accountsMsg struct{ accounts []awsx.Account }
type rolesMsg struct{ roles []awsx.Role }
type rdsMsg struct{ dbs []awsx.RDSInstance }
type containersMsg struct{ containers []awsx.Container }
type ecsClustersMsg struct{ clusters []string }
type ecsTasksMsg struct {
	cluster string
	tasks   []awsx.ECSTask
}

// updateCheckMsg carries the answer to a release check, together with how
// this copy of awsx can be replaced.
type updateCheckMsg struct {
	latest string
	method update.Method
	path   string
	err    error
}

// updateDoneMsg reports the outcome of an upgrade attempt.
type updateDoneMsg struct{ err error }

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

// loadContainersCmd lists the Docker containers running on instanceID. It uses
// a longer timeout than the other reads because the underlying SSM command has
// to be sent, executed on the instance, and polled back.
func loadContainersCmd(cl awsx.ContainerLister, instanceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), containerTimeout)
		defer cancel()
		cs, err := cl.Containers(ctx, instanceID)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ssm:SendCommand")}
		}
		return containersMsg{containers: cs}
	}
}

// loadECSClustersCmd lists the ECS clusters in the resolved region.
func loadECSClustersCmd(l awsx.ECSLister) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		cs, err := l.Clusters(ctx)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ecs:ListClusters")}
		}
		return ecsClustersMsg{clusters: cs}
	}
}

// loadECSTasksCmd lists the running tasks of a cluster, already resolved to the
// node each one runs on.
func loadECSTasksCmd(l awsx.ECSLister, cluster string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		ts, err := l.Tasks(ctx, cluster)
		if err != nil {
			return errMsg{err: err, action: deniedAction(err, "ecs:DescribeTasks")}
		}
		return ecsTasksMsg{cluster: cluster, tasks: ts}
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

// checkUpdateCmd asks GitHub for the newest release and classifies how this
// copy of awsx was installed.
func checkUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
		defer cancel()
		method, path, err := update.Installed()
		if err != nil {
			return updateCheckMsg{err: err}
		}
		rel, err := update.Latest(ctx)
		if err != nil {
			return updateCheckMsg{err: err}
		}
		return updateCheckMsg{latest: rel.Tag, method: method, path: path}
	}
}

// applyUpdateCmd downloads the release and replaces the binary in place.
func applyUpdateCmd(tag, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
		defer cancel()
		return updateDoneMsg{err: update.Install(ctx, tag, path)}
	}
}

// brewUpgradeCmd hands the terminal to `brew upgrade`, capturing stderr so a
// failure survives the TUI redraw.
func brewUpgradeCmd() tea.Cmd {
	cmd := brewUpgradeExec()
	buf := &strings.Builder{}
	cmd.Stderr = io.MultiWriter(cmd.Stderr, buf)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			if out := strings.TrimSpace(buf.String()); out != "" {
				return updateDoneMsg{err: fmt.Errorf("%w: %s", err, out)}
			}
		}
		return updateDoneMsg{err: err}
	})
}
