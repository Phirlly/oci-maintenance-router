package configuration

// Decode accepts an independently verified region/service catalog. Never build
// that catalog from the untrusted configuration itself. This does no cloud I/O.
func Decode(data []byte, catalog Catalog) (Config, error) {
	value, err := strictJSON(data)
	if err != nil {
		return Config{}, err
	}
	version, err := configurationVersion(value)
	if err != nil {
		return Config{}, err
	}
	required := []string{
		"schemaVersion", "environmentName", "tenancyId", "deploymentRegion", "regionMap",
		"compartmentIds", "mysqlServices", "ownership", "destinations", "routes",
		"pagerDutyRegion", "intakeTopicId", "operatorTopicId",
	}
	optional := []string{"mode", "activeBaselineId", "replay"}
	if version == 2 || version == 3 || version == 4 || version == 5 || version == 6 || version == 7 {
		required = append(required, "mode", "installationId", "preparation")
		optional = nil
	}
	if version == 3 || version == 4 || version == 5 || version == 6 || version == 7 {
		optional = []string{"diagnostic"}
	}
	if version == 5 || version == 6 || version == 7 {
		optional = append(optional, "operatorTest")
	}
	if version == 7 {
		optional = append(optional, "recovery")
	}
	r := readObject(value, "configuration", &err, required, optional)
	if err != nil {
		return Config{}, err
	}
	config := Config{
		SchemaVersion: version, EnvironmentName: r.text("environmentName"),
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
	if err = config.validateControls(catalog); err != nil {
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
	if c.SchemaVersion == 2 || c.SchemaVersion == 3 || c.SchemaVersion == 4 || c.SchemaVersion == 5 || c.SchemaVersion == 6 || c.SchemaVersion == 7 {
		c.readPreparation(r)
		if c.SchemaVersion == 3 || c.SchemaVersion == 4 || c.SchemaVersion == 5 || c.SchemaVersion == 6 || c.SchemaVersion == 7 {
			c.readDiagnostic(r)
		}
		if c.SchemaVersion == 5 || c.SchemaVersion == 6 || c.SchemaVersion == 7 {
			c.readOperatorTest(r)
		}
		if c.SchemaVersion == 7 {
			c.readRecovery(r)
		}
		return
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
