package reconciliation

import (
	"slices"
	"strings"

	"oci-maintenance-router/internal/configuration"
)

const baselineLimit = 1 << 20

// Every valid counted layer/cell/revision occupies more than32 wire bytes,
// including the layer overhead paying for its own array entry. This bounds
// malformed-array allocation without excluding any valid <=1MiB baseline.
const baselineEntries = baselineLimit / 32

type discoveryScope struct{ regions, compartments []string }

func baselineScope(config configuration.Config) discoveryScope {
	scope := discoveryScope{compartments: slices.Clone(config.CompartmentIDs)}
	for region := range config.RegionMap {
		scope.regions = append(scope.regions, region)
	}
	slices.Sort(scope.regions)
	slices.Sort(scope.compartments)
	return scope
}

func (r DiscoveryResult) baselineUsable() bool {
	return r.environmentID != "" && r.view.environmentID == r.environmentID && r.availability.environmentID == r.environmentID && leaseWallTime(r.completedAt) && len(r.scope.regions) > 0 && len(r.scope.compartments) > 0
}

func (s discoveryScope) addedCells(existing map[BaselineCell]int) ([]BaselineCell, error) {
	if len(s.regions) > baselineEntries/len(s.compartments) {
		return nil, &BaselineError{Code: "too_large"}
	}
	cells := []BaselineCell{}
	bytes := 0
	for _, region := range s.regions {
		for _, compartment := range s.compartments {
			cell := BaselineCell{region, compartment}
			if _, ok := existing[cell]; ok {
				continue
			}
			// Constructor scope is restricted to ASCII identifier shapes, so these
			// fields require no JSON escaping. The comma allowance is conservative.
			bytes += len(region) + len(compartment) + len(`{"region":"","compartmentId":""}`) + 1
			if bytes > baselineLimit {
				return nil, &BaselineError{Code: "too_large"}
			}
			cells = append(cells, cell)
		}
	}
	return cells, nil
}

func compareBaselineCells(a, b BaselineCell) int {
	if order := strings.Compare(a.Region, b.Region); order != 0 {
		return order
	}
	return strings.Compare(a.CompartmentID, b.CompartmentID)
}
func compareBaselineRevisions(a, b BaselineRevision) int {
	if order := strings.Compare(a.NoticeID, b.NoticeID); order != 0 {
		return order
	}
	return strings.Compare(a.Digest, b.Digest)
}
