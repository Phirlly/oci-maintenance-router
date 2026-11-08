package configuration

func (c Config) validateRouting(catalog Catalog) error {
	owner := c.Ownership
	if (owner.Kind != "defined" && owner.Kind != "freeform") || !validTagName(owner.Key) ||
		(owner.Kind == "defined" && !validTagName(owner.Namespace)) {
		return &Error{"ownership", "invalid"}
	}
	if len(c.Destinations) == 0 {
		return &Error{"destinations", "invalid"}
	}
	destinations := map[string]bool{}
	key := catalog.Regions[c.DeploymentRegion]
	for _, destination := range c.Destinations {
		_, regional := regionalOCID(destination.SecretID, "vaultsecret", c.DeploymentRegion, key)
		if !namePattern.MatchString(destination.ID) || destinations[destination.ID] || !regional {
			return &Error{"destinations", "invalid"}
		}
		destinations[destination.ID] = true
	}
	if len(c.Routes) == 0 {
		return &Error{"routes", "invalid"}
	}
	owners := map[string]bool{}
	for _, route := range c.Routes {
		if route.OwnerValue == "" || owners[route.OwnerValue] || !destinations[route.DestinationID] {
			return &Error{"routes", "invalid"}
		}
		owners[route.OwnerValue] = true
	}
	if c.PagerDutyRegion != "US" && c.PagerDutyRegion != "EU" {
		return &Error{"pagerDutyRegion", "invalid"}
	}
	intake, ok := regionalOCID(c.IntakeTopicID, "onstopic", c.DeploymentRegion, key)
	if !ok {
		return &Error{"intakeTopicId", "invalid"}
	}
	operator, ok := regionalOCID(c.OperatorTopicID, "onstopic", c.DeploymentRegion, key)
	if !ok || operator == intake {
		return &Error{"operatorTopicId", "invalid"}
	}
	return nil
}

func validTagName(name string) bool {
	if len(name) == 0 || len(name) > 100 {
		return false
	}
	for i := range len(name) {
		if name[i] < '!' || name[i] > '~' || name[i] == '.' {
			return false
		}
	}
	return true
}
