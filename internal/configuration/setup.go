package configuration

import (
	"encoding/json"
	"strconv"
)

const MaxSetupAttempt int64 = 2147483647

func (c *Config) readSetup(r objectReader) {
	value := r.values["setup"]
	s := readObject(value, "setup", r.err, []string{"action"}, []string{"attempt", "secretId", "nonProduction", "pagerDutyCode", "operatorCode"})
	c.Setup = &SetupRequest{Action: s.text("action")}
	required := []string{"action"}
	switch c.Setup.Action {
	case "NONE":
	case "SEND_TEST":
		required = append(required, "attempt", "secretId", "nonProduction")
	case "CONFIRM":
		required = append(required, "attempt", "pagerDutyCode", "operatorCode")
	default:
		s.fail("setup.action", "invalid")
	}
	s = readObject(value, "setup", r.err, required, nil)
	if c.Setup.Action == "SEND_TEST" || c.Setup.Action == "CONFIRM" {
		c.Setup.Attempt = setupAttempt(s)
	}
	if c.Setup.Action == "SEND_TEST" {
		c.Setup.SecretID = s.text("secretId")
		var ok bool
		c.Setup.NonProduction, ok = s.values["nonProduction"].(bool)
		if !ok {
			s.fail("setup.nonProduction", "type")
		}
	}
	if c.Setup.Action == "CONFIRM" {
		c.Setup.PagerDutyCode, c.Setup.OperatorCode = s.text("pagerDutyCode"), s.text("operatorCode")
	}
	if value, exists := r.values["operatorChannel"]; exists {
		o := readObject(value, "operatorChannel", r.err, []string{"subscriptionId", "endpoint"}, nil)
		c.OperatorChannel = &OperatorChannel{o.text("subscriptionId"), o.text("endpoint")}
	}
}

func setupAttempt(r objectReader) int64 {
	number, ok := r.values["attempt"].(json.Number)
	if !ok {
		r.fail("setup.attempt", "type")
		return 0
	}
	n, err := strconv.ParseInt(string(number), 10, 64)
	if err != nil || n < 1 || n > MaxSetupAttempt || strconv.FormatInt(n, 10) != string(number) {
		r.fail("setup.attempt", "invalid")
		return 0
	}
	return n
}

func (c Config) validateSetup(catalog Catalog) error {
	if !baselinePattern.MatchString(c.InstallationID) {
		return &Error{"installationId", "invalid"}
	}
	if c.Mode != "PREVIEW" && c.Mode != "PAUSED" && c.Mode != "TEST" && c.Mode != "LIVE" {
		return &Error{"mode", "invalid"}
	}
	s := c.Setup
	if (c.Mode == "TEST") != (s.Action != "NONE") {
		return &Error{"setup.action", "mode_mismatch"}
	}
	if c.OperatorChannel == nil {
		if c.Mode == "TEST" || c.Mode == "LIVE" {
			return &Error{"operatorChannel", "required"}
		}
	} else if err := c.validateOperatorChannel(c.OperatorChannel.SubscriptionID, c.OperatorChannel.Endpoint, "operatorChannel", catalog); err != nil {
		return err
	}
	if s.Action == "SEND_TEST" {
		_, regional := regionalOCID(s.SecretID, "vaultsecret", c.DeploymentRegion, catalog.Regions[c.DeploymentRegion])
		if len(s.SecretID) > 4096 || !regional {
			return &Error{"setup.secretId", "invalid"}
		}
		if !s.NonProduction {
			return &Error{"setup.nonProduction", "invalid"}
		}
	}
	if s.Action == "CONFIRM" {
		if !receiptPattern.MatchString(s.PagerDutyCode) {
			return &Error{"setup.pagerDutyCode", "invalid"}
		}
		if !operatorReceiptPattern.MatchString(s.OperatorCode) {
			return &Error{"setup.operatorCode", "invalid"}
		}
	}
	if s.Action != "NONE" && c.Recovery != nil {
		return &Error{"recovery", "forbidden_during_test"}
	}
	return c.validateRecovery()
}
