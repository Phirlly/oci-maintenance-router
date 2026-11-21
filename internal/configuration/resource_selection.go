package configuration

import "slices"

const resourceMySQL = "mysqldbsystem"
const resourceInstance = "instance"

func (c Config) validateResourceSelection() error {
	if !uniqueNonempty(c.ResourceTypes) {
		return &Error{"resourceTypes", "invalid"}
	}
	for _, kind := range c.ResourceTypes {
		if kind != resourceMySQL && kind != resourceInstance {
			return &Error{"resourceTypes", "unsupported"}
		}
	}
	return nil
}

// Called only after both the catalog and selected types have been validated.
func (c *Config) bindSelectedServices(catalog Catalog) {
	c.ResourceServices = make(map[string][]string, len(c.ResourceTypes))
	for _, kind := range c.ResourceTypes {
		c.ResourceServices[kind] = slices.Clone(catalog.ResourceServices[kind])
	}
}

func (c Config) canonicalSelectedServices() map[string][]string {
	services := make(map[string][]string, len(c.ResourceTypes))
	for _, kind := range c.ResourceTypes {
		labels := slices.Clone(c.ResourceServices[kind])
		slices.Sort(labels)
		services[kind] = labels
	}
	return services
}
