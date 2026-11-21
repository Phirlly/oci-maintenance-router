package configuration

func (c Config) validateControls(catalog Catalog) error {
	if c.SchemaVersion == 8 || c.SchemaVersion == 9 {
		return c.validateSetup(catalog)
	}
	if c.SchemaVersion == 2 {
		return c.validatePreparation()
	}
	if c.SchemaVersion == 3 || c.SchemaVersion == 4 || c.SchemaVersion == 5 || c.SchemaVersion == 6 || c.SchemaVersion == 7 {
		if err := c.validateDiagnostic(catalog); err != nil {
			return err
		}
		if err := c.validateOperatorTest(catalog); err != nil {
			return err
		}
		if err := c.validateLiveInputs(); err != nil {
			return err
		}
		return c.validateRecovery()
	}
	if c.Mode != "PREVIEW" && c.Mode != "LIVE" && c.Mode != "PAUSED" {
		return &Error{"mode", "invalid"}
	}
	if c.ActiveBaselineID != "" && !baselinePattern.MatchString(c.ActiveBaselineID) {
		return &Error{"activeBaselineId", "invalid"}
	}
	if c.Mode == "LIVE" && c.ActiveBaselineID == "" {
		return &Error{"activeBaselineId", "required_for_live"}
	}
	seen := map[Replay]bool{}
	for _, selection := range c.Replay {
		if !globalOCID(selection.NoticeID, "announcement") ||
			!digestPattern.MatchString(selection.RevisionDigest) || seen[selection] {
			return &Error{"replay", "invalid"}
		}
		seen[selection] = true
	}
	return nil
}
