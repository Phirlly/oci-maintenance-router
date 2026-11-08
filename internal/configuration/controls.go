package configuration

func (c Config) validateControls() error {
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
