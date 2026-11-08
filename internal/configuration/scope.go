package configuration

import (
	"slices"
	"unicode/utf8"
)

func validateCatalog(c Catalog) error {
	if len(c.Regions) == 0 || len(c.MySQLServices) == 0 {
		return &Error{"catalog", "invalid"}
	}
	keys := map[string]bool{}
	for region, key := range c.Regions {
		if !regionPattern.MatchString(region) || !regionKeyPattern.MatchString(key) || keys[key] {
			return &Error{"catalog", "invalid"}
		}
		keys[key] = true
	}
	if !uniqueNonempty(c.MySQLServices) {
		return &Error{"catalog", "invalid"}
	}
	return nil
}

func (c Config) validateScope(catalog Catalog) error {
	if err := validateCatalog(catalog); err != nil {
		return err
	}
	if !namePattern.MatchString(c.EnvironmentName) {
		return &Error{"environmentName", "invalid"}
	}
	if !globalOCID(c.TenancyID, "tenancy") {
		return &Error{"tenancyId", "invalid"}
	}
	if _, ok := catalog.Regions[c.DeploymentRegion]; !ok {
		return &Error{"deploymentRegion", "invalid"}
	}
	if len(c.RegionMap) == 0 {
		return &Error{"regionMap", "invalid"}
	}
	for region, key := range c.RegionMap {
		if catalog.Regions[region] != key || key == "" {
			return &Error{"regionMap", "invalid"}
		}
	}
	if !uniqueNonempty(c.CompartmentIDs) {
		return &Error{"compartmentIds", "invalid"}
	}
	for _, id := range c.CompartmentIDs {
		if !globalOCID(id, "compartment") {
			return &Error{"compartmentIds", "invalid"}
		}
	}
	if !uniqueNonempty(c.MySQLServices) {
		return &Error{"mysqlServices", "invalid"}
	}
	for _, service := range c.MySQLServices {
		if !slices.Contains(catalog.MySQLServices, service) {
			return &Error{"mysqlServices", "invalid"}
		}
	}
	return nil
}

func uniqueNonempty(values []string) bool {
	if len(values) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || !utf8.ValidString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
