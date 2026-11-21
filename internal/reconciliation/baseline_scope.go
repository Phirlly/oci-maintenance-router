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

type discoveryScope struct{ regions, compartments, resourceTypes []string }

func baselineScope(config configuration.Config) discoveryScope {
	scope := discoveryScope{compartments: slices.Clone(config.CompartmentIDs), resourceTypes: []string{baselineMySQL}}
	for region := range config.RegionMap {
		scope.regions = append(scope.regions, region)
	}
	slices.Sort(scope.regions)
	slices.Sort(scope.compartments)
	return scope
}

func (r DiscoveryResult) baselineUsable() bool {
	return r.environmentID != "" && r.view.environmentID == r.environmentID && r.availability.environmentID == r.environmentID && leaseWallTime(r.completedAt) && r.scope.usable()
}

func (s discoveryScope) addedCells(existing map[baselineCellKey]int) ([]BaselineCell, error) {
	if !s.usable() {
		return nil, &BaselineError{Code: "invalid_scope"}
	}
	if len(s.regions) > baselineEntries/len(s.compartments)/len(s.resourceTypes) {
		return nil, &BaselineError{Code: "too_large"}
	}
	cells := []BaselineCell{}
	bytes := 0
	for _, region := range s.regions {
		for _, compartment := range s.compartments {
			for _, kind := range s.resourceTypes {
				cell := BaselineCell{Region: region, CompartmentID: compartment}
				if kind == baselineInstance {
					cell.ResourceType = kind
				}
				if _, ok := existing[cell.key()]; ok {
					continue
				}
				// Validated ASCII identifiers need no escaping. Include the optional
				// type field and a conservative comma before allocating another cell.
				bytes += len(region) + len(compartment) + len(`{"region":"","compartmentId":""}`) + 1
				if cell.ResourceType != "" {
					bytes += len(`,"resourceType":""`) + len(cell.ResourceType)
				}
				if bytes > baselineLimit {
					return nil, &BaselineError{Code: "too_large"}
				}
				cells = append(cells, cell)
			}
		}
	}
	return cells, nil
}

func compareBaselineCells(a, b BaselineCell) int {
	if order := strings.Compare(a.Region, b.Region); order != 0 {
		return order
	}
	if order := strings.Compare(a.CompartmentID, b.CompartmentID); order != 0 {
		return order
	}
	return strings.Compare(a.key().resourceType, b.key().resourceType)
}
func compareBaselineRevisions(a, b BaselineRevision) int {
	if order := strings.Compare(a.NoticeID, b.NoticeID); order != 0 {
		return order
	}
	return strings.Compare(a.Digest, b.Digest)
}
