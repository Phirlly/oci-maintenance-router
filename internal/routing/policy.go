package routing

import "oci-maintenance-router/internal/configuration"

// Policy owns validated settings. Use New; nil/zero policies fail closed.
type Policy struct {
	generation   string
	tenancyID    string
	regions      map[string]string
	allowed      map[string]bool
	compartments map[string]bool
	services     map[string]bool
	selected     map[string]map[string]bool
	owner        configuration.Ownership
	routes       map[string]string
}

// New requires an independently verified catalog, not one built from data.
// It retains no caller-owned maps, configuration bytes or secret references.
func New(data []byte, catalog configuration.Catalog) (*Policy, error) {
	c, err := configuration.Decode(data, catalog)
	if err != nil {
		return nil, err
	}
	generation, err := c.RoutingGeneration()
	if err != nil {
		return nil, err
	}
	p := &Policy{
		generation: generation, tenancyID: c.TenancyID, regions: map[string]string{}, allowed: map[string]bool{},
		compartments: map[string]bool{}, services: map[string]bool{},
		owner: c.Ownership, routes: map[string]string{},
	}
	for region, key := range catalog.Regions {
		for _, alias := range []string{region, key} {
			if previous, exists := p.regions[alias]; exists && previous != region {
				return nil, &configuration.Error{Field: "catalog", Code: "ambiguous_region"}
			}
			p.regions[alias] = region
		}
	}
	for region := range c.RegionMap {
		p.allowed[region] = true
	}
	for _, compartment := range c.CompartmentIDs {
		p.compartments[compartment] = true
	}
	for _, service := range c.MySQLServices {
		p.services[service] = true
	}
	if c.SchemaVersion == 9 {
		p.selected = map[string]map[string]bool{}
		for kind, labels := range c.ResourceServices {
			p.selected[kind] = map[string]bool{}
			for _, label := range labels {
				p.selected[kind][label], p.services[label] = true, true
			}
		}
	}
	for _, route := range c.Routes {
		p.routes[route.OwnerValue] = route.DestinationID
	}
	return p, nil
}
