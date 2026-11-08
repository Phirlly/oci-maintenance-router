package delivery

import "oci-maintenance-router/internal/routing"

func selectGroups(groups []routing.Group, selected []string) ([]routing.Group, error) {
	if len(selected) > 256 {
		return nil, &Error{"selection", "too_large"}
	}
	total := 0
	for _, id := range selected {
		total += len(id)
		if total > 1<<20 {
			return nil, &Error{"selection", "too_large"}
		}
	}
	ready := map[string]bool{}
	for _, group := range groups {
		for _, r := range group.Resources {
			ready[r.ResourceID] = true
		}
	}
	wanted := map[string]bool{}
	for _, id := range selected {
		if wanted[id] {
			return nil, &Error{"selection", "duplicate"}
		}
		if !ready[id] {
			return nil, &Error{"selection", "not_ready"}
		}
		wanted[id] = true
	}
	var result []routing.Group
	// Resolve already supplies sorted destinations and resource IDs.
	for _, group := range groups {
		g := routing.Group{DestinationID: group.DestinationID}
		for _, r := range group.Resources {
			if wanted[r.ResourceID] {
				g.Resources = append(g.Resources, r)
			}
		}
		if len(g.Resources) > 0 {
			result = append(result, g)
		}
	}
	return result, nil
}
