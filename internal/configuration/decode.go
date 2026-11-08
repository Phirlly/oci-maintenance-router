package configuration

import "encoding/json"

// Decode accepts an independently verified region/service catalog. Never build
// that catalog from the untrusted configuration itself. This does no cloud I/O.
func Decode(data []byte, catalog Catalog) (Config, error) {
	value, err := strictJSON(data)
	if err != nil {
		return Config{}, err
	}
	r := readObject(value, "configuration", &err, []string{
		"schemaVersion", "environmentName", "tenancyId", "deploymentRegion", "regionMap",
		"compartmentIds", "mysqlServices", "ownership", "destinations", "routes",
		"pagerDutyRegion", "intakeTopicId", "operatorTopicId",
	}, []string{"mode", "activeBaselineId", "replay"})
	if err != nil {
		return Config{}, err
	}
	if r.values["schemaVersion"] == nil {
		r.fail("schemaVersion", "type")
	}
	if number, ok := r.values["schemaVersion"].(json.Number); !ok || number != "1" {
		r.fail("schemaVersion", "unsupported")
	}
	config := Config{
		SchemaVersion: 1, EnvironmentName: r.text("environmentName"),
		TenancyID: r.text("tenancyId"), DeploymentRegion: r.text("deploymentRegion"),
		RegionMap: r.stringMap("regionMap"), CompartmentIDs: r.strings("compartmentIds"),
		MySQLServices: r.strings("mysqlServices"), PagerDutyRegion: r.text("pagerDutyRegion"),
		IntakeTopicID: r.text("intakeTopicId"), OperatorTopicID: r.text("operatorTopicId"),
		Mode: "PREVIEW",
	}
	config.readRouting(r)
	config.readControls(r)
	if err != nil {
		return Config{}, err
	}
	if err = config.validateScope(catalog); err != nil {
		return Config{}, err
	}
	if err = config.validateRouting(catalog); err != nil {
		return Config{}, err
	}
	if err = config.validateControls(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c *Config) readRouting(r objectReader) {
	owner := readObject(r.values["ownership"], "ownership", r.err, []string{"kind", "key"}, []string{"namespace"})
	c.Ownership = Ownership{Kind: owner.text("kind"), Key: owner.text("key")}
	if c.Ownership.Kind == "defined" {
		if _, ok := owner.values["namespace"]; !ok {
			owner.fail("ownership.namespace", "required")
		}
		c.Ownership.Namespace = owner.text("namespace")
	} else if _, ok := owner.values["namespace"]; ok {
		owner.fail("ownership", "exclusive_selector")
	}
	for _, raw := range r.array("destinations") {
		item := readObject(raw, "destinations[]", r.err, []string{"id", "secretId"}, nil)
		c.Destinations = append(c.Destinations, Destination{item.text("id"), item.text("secretId")})
	}
	for _, raw := range r.array("routes") {
		item := readObject(raw, "routes[]", r.err, []string{"ownerValue", "destinationId"}, nil)
		c.Routes = append(c.Routes, Route{item.text("ownerValue"), item.text("destinationId")})
	}
}

func (c *Config) readControls(r objectReader) {
	if _, ok := r.values["mode"]; ok {
		c.Mode = r.text("mode")
	}
	if value, exists := r.values["activeBaselineId"]; exists && value != nil {
		c.ActiveBaselineID = r.text("activeBaselineId")
		if c.ActiveBaselineID == "" {
			r.fail("activeBaselineId", "invalid")
		}
	}
	if _, exists := r.values["replay"]; exists {
		for _, raw := range r.array("replay") {
			item := readObject(raw, "replay[]", r.err, []string{"noticeId", "revisionDigest"}, nil)
			c.Replay = append(c.Replay, Replay{item.text("noticeId"), item.text("revisionDigest")})
		}
	}
}
