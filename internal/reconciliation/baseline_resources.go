package reconciliation

import "strings"

const baselineMySQL = "mysqldbsystem"
const baselineInstance = "instance"

// The index uses explicit identity while retained cells keep their original
// wire representation. Omitted resourceType always means legacy MySQL.
type baselineCellKey struct{ resourceType, region, compartment string }

func (c BaselineCell) key() baselineCellKey {
	kind := c.ResourceType
	if kind == "" {
		kind = baselineMySQL
	}
	return baselineCellKey{kind, c.Region, c.CompartmentID}
}

func baselineSchema(layers []BaselineLayer) int {
	for _, layer := range layers {
		for _, cell := range layer.Cells {
			if cell.ResourceType == baselineInstance {
				return 2
			}
		}
	}
	return 1
}

func (s discoveryScope) usable() bool {
	if len(s.regions) == 0 || len(s.compartments) == 0 || len(s.resourceTypes) == 0 || len(s.resourceTypes) > 2 {
		return false
	}
	for i, kind := range s.resourceTypes {
		if (kind != baselineMySQL && kind != baselineInstance) || (i > 0 && strings.Compare(s.resourceTypes[i-1], kind) >= 0) {
			return false
		}
	}
	return true
}
