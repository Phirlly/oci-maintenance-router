package configuration

import (
	"strings"
	"testing"
)

func TestDiagnosticJSONRetainsStrictParsingAndPrivacy(t *testing.T) {
	wire := string(encoded(t, diagnosticSettings(t)))
	for _, tc := range []struct{ body, field, code string }{
		{strings.Replace(wire, `"requestId":"test-1"`, `"requestId":"test-1","requestId":"other"`, 1), "configuration", "duplicate_key"},
		{strings.Replace(wire, `"nonProduction":true`, `"nonProduction":true,"\u006eonProduction":false`, 1), "configuration", "duplicate_key"},
		{strings.Replace(wire, `"test-1"`, `"\ud800"`, 1), "configuration", "invalid_json"},
		{strings.Replace(wire, `"test-1"`, "\"\xff\"", 1), "configuration", "invalid_json"},
		{strings.Replace(wire, `"nonProduction":true`, `"nonProduction":true,"private-key":"private-value"`, 1), "diagnostic", "unknown_field"},
	} {
		wantError(t, []byte(tc.body), tc.field, tc.code)
		_, err := Decode([]byte(tc.body), catalog())
		if strings.Contains(err.Error(), "private") {
			t.Fatal("private value leaked")
		}
	}
	for _, version := range []string{"3.0", "3e0", `"3"`, "10"} {
		body := strings.Replace(wire, `"schemaVersion":3`, `"schemaVersion":`+version, 1)
		wantError(t, []byte(body), "schemaVersion", "unsupported")
	}
	for _, field := range []string{"environmentName", "tenancyId", "deploymentRegion", "regionMap", "compartmentIds", "mysqlServices", "ownership", "destinations", "routes", "pagerDutyRegion", "intakeTopicId", "operatorTopicId"} {
		v := diagnosticSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
}
