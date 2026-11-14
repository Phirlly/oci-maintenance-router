package configuration

func (c *Config) readPreparation(r objectReader) {
	c.InstallationID = r.text("installationId")
	p := readObject(r.values["preparation"], "preparation", r.err, []string{"initialize"}, nil)
	initialize, ok := p.values["initialize"].(bool)
	if !ok {
		p.fail("preparation.initialize", "type")
	}
	c.Preparation = Preparation{Initialize: initialize}
}

func (c Config) validatePreparation() error {
	if !baselinePattern.MatchString(c.InstallationID) {
		return &Error{"installationId", "invalid"}
	}
	if c.Mode != "PREVIEW" && c.Mode != "PAUSED" {
		return &Error{"mode", "invalid"}
	}
	return nil
}
