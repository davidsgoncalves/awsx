// Package aws is the boundary between AWSX and AWS. Typed reads go through the
// SDK; the two operations that require it (sso login, start-session) shell out
// to the AWS CLI. All behavior is expressed as small interfaces so the TUI can
// be tested with fakes.
package aws

import "context"

// Identity is the caller identity from sts:GetCallerIdentity.
type Identity struct {
	Account string
	Arn     string
	UserID  string
}

// Instance is a minimal EC2 instance view.
type Instance struct {
	ID        string
	Name      string
	State     string
	Type      string
	PrivateIP string
}

// Target is an instance that can receive an SSM session.
type Target struct {
	Instance
	SSMOnline bool
}

// IdentityProvider resolves the caller identity for a profile/region.
type IdentityProvider interface {
	WhoAmI(ctx context.Context) (Identity, error)
}

// EC2Lister lists running EC2 instances in the resolved region.
type EC2Lister interface {
	RunningInstances(ctx context.Context) ([]Instance, error)
}

// SSMLister returns the set of instance IDs currently online in SSM.
type SSMLister interface {
	OnlineInstanceIDs(ctx context.Context) (map[string]bool, error)
}

// RDSInstance is a database endpoint reachable through a tunnel.
type RDSInstance struct {
	Name     string
	Engine   string
	Endpoint string
	Port     int
}

// RDSLister lists the RDS database instances in the resolved region.
type RDSLister interface {
	RDSInstances(ctx context.Context) ([]RDSInstance, error)
}

// Login performs the interactive SSO login (shell-out to the AWS CLI).
type Login interface {
	SSOLogin(ctx context.Context) error
}

// Sessioner opens an interactive SSM session, handing over the terminal.
type Sessioner interface {
	StartSession(instanceID string) error
}

// DisplayName returns the instance Name tag or the instance ID as fallback.
func DisplayName(i Instance) string {
	if i.Name != "" {
		return i.Name
	}
	return i.ID
}
