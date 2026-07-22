package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/davidsgoncalves/awsx/internal/config"
)

// Clients bundles the SDK-backed readers for a single profile/region.
type Clients struct {
	sts *sts.Client
	ec2 *ec2.Client
	ssm *ssm.Client
	rds *rds.Client
}

// NewClients loads shared config for profile and returns typed clients plus the
// resolved region. When regionOverride is non-empty it is used directly;
// otherwise the region is resolved from the profile/env (config.ErrNoRegion if
// none). It reads the official AWS CLI SSO cache; it never creates credentials.
func NewClients(ctx context.Context, profile, regionOverride string) (*Clients, string, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithSharedConfigProfile(profile))
	if err != nil {
		return nil, "", fmt.Errorf("load profile %q: %w", profile, err)
	}
	region := regionOverride
	if region == "" {
		region, err = config.ResolveRegion(cfg.Region)
		if err != nil {
			return nil, "", err
		}
	}
	cfg.Region = region
	return newClientsFromConfig(cfg), region, nil
}

// newClientsFromConfig builds the SDK-backed readers from a resolved config.
func newClientsFromConfig(cfg awssdk.Config) *Clients {
	return &Clients{
		sts: sts.NewFromConfig(cfg),
		ec2: ec2.NewFromConfig(cfg),
		ssm: ssm.NewFromConfig(cfg),
		rds: rds.NewFromConfig(cfg),
	}
}

// WhoAmI implements IdentityProvider.
func (c *Clients) WhoAmI(ctx context.Context) (Identity, error) {
	out, err := c.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		Account: deref(out.Account),
		Arn:     deref(out.Arn),
		UserID:  deref(out.UserId),
	}, nil
}

// RunningInstances implements EC2Lister.
func (c *Clients) RunningInstances(ctx context.Context) ([]Instance, error) {
	var out []Instance
	p := ec2.NewDescribeInstancesPaginator(c.ec2, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{
			Name:   ptr("instance-state-name"),
			Values: []string{"running"},
		}},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, res := range page.Reservations {
			for _, inst := range res.Instances {
				out = append(out, Instance{
					ID:        deref(inst.InstanceId),
					Name:      nameTag(inst.Tags),
					State:     string(inst.State.Name),
					Type:      string(inst.InstanceType),
					PrivateIP: deref(inst.PrivateIpAddress),
				})
			}
		}
	}
	return out, nil
}

// OnlineInstanceIDs implements SSMLister.
func (c *Clients) OnlineInstanceIDs(ctx context.Context) (map[string]bool, error) {
	online := map[string]bool{}
	p := ssm.NewDescribeInstanceInformationPaginator(c.ssm, &ssm.DescribeInstanceInformationInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, info := range page.InstanceInformationList {
			if string(info.PingStatus) == "Online" {
				online[deref(info.InstanceId)] = true
			}
		}
	}
	return online, nil
}

func nameTag(tags []ec2types.Tag) string {
	for _, t := range tags {
		if deref(t.Key) == "Name" {
			return deref(t.Value)
		}
	}
	return ""
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
