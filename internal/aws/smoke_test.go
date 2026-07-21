package aws

import (
	"context"
	"os"
	"testing"
	"time"
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

	clients, region, err := NewClients(ctx, profile)
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
