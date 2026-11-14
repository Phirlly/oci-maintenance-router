package configuration

import "regexp"

var receiptPattern = regexp.MustCompile(`^OMR-[0-9a-f]{32}$`)

func (c *Config) readDiagnostic(r objectReader) {
	value, exists := r.values["diagnostic"]
	if !exists {
		return
	}
	var optional []string
	if c.SchemaVersion == 4 || c.SchemaVersion == 5 || c.SchemaVersion == 6 || c.SchemaVersion == 7 {
		optional = []string{"receiptCode"}
	}
	d := readObject(value, "diagnostic", r.err, []string{"requestId", "secretId", "nonProduction"}, optional)
	nonProduction, ok := d.values["nonProduction"].(bool)
	if !ok {
		d.fail("diagnostic.nonProduction", "type")
	}
	c.Diagnostic = &DiagnosticRequest{RequestID: d.text("requestId"), SecretID: d.text("secretId"), NonProduction: nonProduction}
	if _, exists := d.values["receiptCode"]; exists {
		c.Diagnostic.ReceiptCode = d.text("receiptCode")
	}
}

func (c Config) validateDiagnostic(catalog Catalog) error {
	if !baselinePattern.MatchString(c.InstallationID) {
		return &Error{"installationId", "invalid"}
	}
	if c.Mode != "PREVIEW" && c.Mode != "PAUSED" && c.Mode != "TEST" && !((c.SchemaVersion == 6 || c.SchemaVersion == 7) && c.Mode == "LIVE") {
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
	if d.ReceiptCode != "" && !receiptPattern.MatchString(d.ReceiptCode) {
		return &Error{"diagnostic.receiptCode", "invalid"}
	}
	return nil
}
