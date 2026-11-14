package configuration

func (c Config) validateControls(catalog Catalog) error {
	if c.SchemaVersion == 2 {
		return c.validatePreparation()
	}
	if c.SchemaVersion == 3 || c.SchemaVersion == 4 {
		return c.validateDiagnostic(catalog)
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
