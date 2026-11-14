package configuration

func (c *Config) readDiagnostic(r objectReader) {
	value, exists := r.values["diagnostic"]
	if !exists {
		return
	}
	d := readObject(value, "diagnostic", r.err, []string{"requestId", "secretId", "nonProduction"}, nil)
	nonProduction, ok := d.values["nonProduction"].(bool)
	if !ok {
		d.fail("diagnostic.nonProduction", "type")
	}
	c.Diagnostic = &DiagnosticRequest{d.text("requestId"), d.text("secretId"), nonProduction}
}

func (c Config) validateDiagnostic(catalog Catalog) error {
	if !baselinePattern.MatchString(c.InstallationID) {
		return &Error{"installationId", "invalid"}
	}
	if c.Mode != "PREVIEW" && c.Mode != "PAUSED" && c.Mode != "TEST" {
		return &Error{"mode", "invalid"}
	}
	if c.Diagnostic == nil {
		if c.Mode == "TEST" {
			return &Error{"diagnostic", "required_for_test"}
		}
		return nil
	}
	d := c.Diagnostic
	if !baselinePattern.MatchString(d.RequestID) {
		return &Error{"diagnostic.requestId", "invalid"}
	}
	_, regional := regionalOCID(d.SecretID, "vaultsecret", c.DeploymentRegion, catalog.Regions[c.DeploymentRegion])
	if len(d.SecretID) > 4096 || !regional {
		return &Error{"diagnostic.secretId", "invalid"}
	}
	if !d.NonProduction {
		return &Error{"diagnostic.nonProduction", "invalid"}
	}
	return nil
}
