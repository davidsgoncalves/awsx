// Package aws is the boundary between AWSX and AWS. Typed reads go through the
// SDK; the two operations that require it (sso login, start-session) shell out
// to the AWS CLI. All behavior is expressed as small interfaces so the TUI can
// be tested with fakes.
package aws

import (
	"context"
	"sort"
)

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
	VpcID     string
	Tags      map[string]string
}

// TagPair is one instance tag, key and value.
type TagPair struct {
	Key   string
	Value string
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
	VpcID    string
}

// RDSLister lists the RDS database instances in the resolved region.
type RDSLister interface {
	RDSInstances(ctx context.Context) ([]RDSInstance, error)
}

// Container is a Docker container running on an EC2 instance. Service is the
// Compose service label (com.docker.compose.service) and is empty for
// containers not managed by Compose.
type Container struct {
	ID      string
	Name    string
	Service string
	Image   string
	Status  string
}

// ContainerLister lists the Docker containers running on an instance.
type ContainerLister interface {
	Containers(ctx context.Context, instanceID string) ([]Container, error)
}

// Login performs the interactive SSO login (shell-out to the AWS CLI).
type Login interface {
	SSOLogin(ctx context.Context) error
}

// Sessioner opens an interactive SSM session, handing over the terminal.
type Sessioner interface {
	StartSession(instanceID string) error
}

// SortedTags returns the instance tags except Name, ordered by key. Name is
// omitted because it is already the display name.
func SortedTags(i Instance) []TagPair {
	out := make([]TagPair, 0, len(i.Tags))
	for k, v := range i.Tags {
		if k == "Name" {
			continue
		}
		out = append(out, TagPair{Key: k, Value: v})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Key < out[b].Key })
	return out
}

// DisplayName returns the instance Name tag or the instance ID as fallback.
func DisplayName(i Instance) string {
	if i.Name != "" {
		return i.Name
	}
	return i.ID
}
