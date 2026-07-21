package aws

import "sort"

// Join returns SSM-capable targets: running instances that are online in SSM,
// sorted by display name. This is the EC2 x SSM cross described in PRD 6.9.
func Join(instances []Instance, onlineIDs map[string]bool) []Target {
	var out []Target
	for _, inst := range instances {
		if inst.State != "running" {
			continue
		}
		if !onlineIDs[inst.ID] {
			continue
		}
		out = append(out, Target{Instance: inst, SSMOnline: true})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return DisplayName(out[i].Instance) < DisplayName(out[j].Instance)
	})
	return out
}
