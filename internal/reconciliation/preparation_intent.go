package reconciliation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Inputs are owned, sorted and validated by the strict constructor. This tuple
// deliberately excludes unrelated routing controls and changing source data.
func preparationIntent(environment, predecessor string, scope discoveryScope, services []string) []byte {
	var parent any
	if predecessor != "" {
		parent = predecessor
	}
	body, _ := json.Marshal([]any{"baseline-intent-v1", environment, parent, scope.regions, scope.compartments, services})
	return body
}
func (p *BaselinePreparation) targetID(h PreparedHistory) string {
	if p.resourceServices != nil {
		var parent any
		if h.BaselineDigest() != "" {
			parent = h.BaselineDigest()
		}
		body, _ := json.Marshal([]any{"baseline-intent-v2", h.EnvironmentID(), parent, p.discovery.scope.regions, p.discovery.scope.compartments, p.resourceServices})
		return fmt.Sprintf("p2-%x", sha256.Sum256(body))
	}
	return fmt.Sprintf("p1-%x", sha256.Sum256(preparationIntent(h.EnvironmentID(), h.BaselineDigest(), p.discovery.scope, p.services)))
}
