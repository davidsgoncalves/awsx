package aws

import (
	"context"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// RDSInstances implements RDSLister. It lists database instances that have a
// resolvable endpoint, sorted by name.
func (c *Clients) RDSInstances(ctx context.Context) ([]RDSInstance, error) {
	var out []RDSInstance
	p := rds.NewDescribeDBInstancesPaginator(c.rds, &rds.DescribeDBInstancesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, db := range page.DBInstances {
			if db.Endpoint == nil || db.Endpoint.Address == nil {
				continue // no endpoint yet (e.g. creating)
			}
			out = append(out, RDSInstance{
				Name:     deref(db.DBInstanceIdentifier),
				Engine:   deref(db.Engine),
				Endpoint: deref(db.Endpoint.Address),
				Port:     int(portOf(db.Endpoint.Port)),
				VpcID:    vpcOf(db),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func portOf(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func vpcOf(db rdstypes.DBInstance) string {
	if db.DBSubnetGroup == nil {
		return ""
	}
	return deref(db.DBSubnetGroup.VpcId)
}
