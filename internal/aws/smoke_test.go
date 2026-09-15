package aws

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/davidsgoncalves/awsx/internal/config"
	"github.com/davidsgoncalves/awsx/internal/profiles"
)

// TestSmoke_Boundary exercises the real AWS boundary end-to-end against a live
// profile. It is skipped unless AWSX_SMOKE_PROFILE is set, so it never runs in
// CI or the normal unit suite. Only read-only calls are made
// (sts:GetCallerIdentity, ec2:DescribeInstances, ssm:DescribeInstanceInformation).
//
// Run with: AWSX_SMOKE_PROFILE=<profile> go test ./internal/aws/ -run TestSmoke_Boundary -v
func TestSmoke_Boundary(t *testing.T) {
	profile := os.Getenv("AWSX_SMOKE_PROFILE")
	if profile == "" {
		t.Skip("set AWSX_SMOKE_PROFILE to run the live boundary smoke test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clients, region, err := NewClients(ctx, profile, "")
	if err != nil {
		t.Fatalf("NewClients(%q): %v", profile, err)
	}
	t.Logf("resolved region: %s", region)

	id, err := clients.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("WhoAmI: %v (session may be expired — try: aws sso login --profile %s)", err, profile)
	}
	t.Logf("identity: account=%s arn=%s", id.Account, id.Arn)

	instances, err := clients.RunningInstances(ctx)
	if err != nil {
		t.Fatalf("RunningInstances: %v", err)
	}
	t.Logf("running instances: %d", len(instances))

	online, err := clients.OnlineInstanceIDs(ctx)
	if err != nil {
		t.Fatalf("OnlineInstanceIDs: %v", err)
	}
	t.Logf("ssm-online instances: %d", len(online))

	targets := Join(instances, online)
	t.Logf("ssm-capable targets: %d", len(targets))
	for _, tg := range targets {
		t.Logf("  - %-24s %-12s %-10s %s", DisplayName(tg.Instance), tg.ID, tg.Type, tg.PrivateIP)
	}
}

// TestSmoke_SSODiscovery exercises the live SSO discovery path (ListAccounts,
// ListAccountRoles) against a real sso-session. Skipped unless
// AWSX_SMOKE_SSO_SESSION names a session in ~/.aws/config. Read-only.
//
// Run with: AWSX_SMOKE_SSO_SESSION=<name> go test ./internal/aws/ -run TestSmoke_SSODiscovery -v
func TestSmoke_SSODiscovery(t *testing.T) {
	name := os.Getenv("AWSX_SMOKE_SSO_SESSION")
	if name == "" {
		t.Skip("set AWSX_SMOKE_SSO_SESSION to run the live SSO discovery smoke test")
	}

	sessions, err := profiles.ParseSSOSessions(config.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	var session profiles.SSOSession
	for _, s := range sessions {
		if s.Name == name {
			session = s
		}
	}
	if session.Name == "" {
		t.Fatalf("sso-session %q not found in config", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := NewSSOClient(ctx, session, time.Now())
	if err != nil {
		t.Fatalf("NewSSOClient: %v (try: aws sso login --sso-session %s)", err, name)
	}

	accounts, err := c.Accounts(ctx)
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	t.Logf("accounts: %d", len(accounts))
	if len(accounts) == 0 {
		return
	}

	roles, err := c.Roles(ctx, accounts[0].ID)
	if err != nil {
		t.Fatalf("Roles(%s): %v", accounts[0].ID, err)
	}
	t.Logf("first account %s (%s) roles: %d", accounts[0].Name, accounts[0].ID, len(roles))
}

// TestSmoke_ECS exercises the live ECS path (ListClusters, ListTasks,
// DescribeTasks, DescribeContainerInstances) against a real profile. Skipped
// unless AWSX_SMOKE_PROFILE is set. Read-only.
//
// Run with: AWSX_SMOKE_PROFILE=<profile> go test ./internal/aws/ -run TestSmoke_ECS -v
func TestSmoke_ECS(t *testing.T) {
	profile := os.Getenv("AWSX_SMOKE_PROFILE")
	if profile == "" {
		t.Skip("set AWSX_SMOKE_PROFILE to run the live ECS smoke test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	clients, region, err := NewClients(ctx, profile, "")
	if err != nil {
		t.Fatalf("NewClients(%q): %v", profile, err)
	}
	t.Logf("resolved region: %s", region)

	clusters, err := clients.Clusters(ctx)
	if err != nil {
		t.Fatalf("Clusters: %v", err)
	}
	t.Logf("clusters: %v", clusters)

	for _, c := range clusters {
		tasks, err := clients.Tasks(ctx, c)
		if err != nil {
			t.Fatalf("Tasks(%q): %v", c, err)
		}
		t.Logf("%s: %d running containers", c, len(tasks))
		for _, task := range tasks {
			t.Logf("  - %-22s %-14s %-20s exec=%t", DisplayTask(task), task.Container, task.InstanceID, task.ExecEnabled)
		}
	}
}
