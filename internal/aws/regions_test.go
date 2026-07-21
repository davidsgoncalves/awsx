package aws

import (
	"sort"
	"testing"
)

func TestRegions(t *testing.T) {
	r := Regions()
	if len(r) == 0 {
		t.Fatal("region list is empty")
	}

	seen := map[string]bool{}
	for _, x := range r {
		if seen[x] {
			t.Fatalf("duplicate region: %s", x)
		}
		seen[x] = true
	}
	if !seen["us-east-1"] || !seen["sa-east-1"] {
		t.Fatalf("expected common regions missing: %v", r)
	}
	if !sort.StringsAreSorted(r) {
		t.Fatal("region list is not sorted")
	}
}
