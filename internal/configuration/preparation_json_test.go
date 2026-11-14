package configuration

import (
	"strings"
	"testing"
)

func TestPreparationJSONKeepsStrictNamesDuplicatesAndPrivacy(t *testing.T) {
	wire := string(encoded(t, preparationSettings(t)))
	for _, tt := range []struct{ body, field, code string }{
		{strings.Replace(wire, `"initialize":true`, `"initialize":true,"initialize":false`, 1), "configuration", "duplicate_key"},
		{strings.Replace(wire, `"initialize":true`, `"initialize":true,"\u0069nitialize":false`, 1), "configuration", "duplicate_key"},
		{strings.Replace(wire, `"installationId":"installation-1"`, `"installationId":"installation-1","installationId":"other"`, 1), "configuration", "duplicate_key"},
		{strings.Replace(wire, `"initialize":true`, `"initialize":true,"private-key":"private-value"`, 1), "preparation", "unknown_field"},
		{strings.Replace(wire, `"initialize":true`, `"initialize":true,"Initialize":true`, 1), "preparation", "unknown_field"},
		{strings.Replace(wire, `"installation-1"`, `"\ud800"`, 1), "configuration", "invalid_json"},
		{strings.Replace(wire, `"installation-1"`, "\"\xff\"", 1), "configuration", "invalid_json"},
		{wire + " {}", "configuration", "invalid_json"},
	} {
		wantError(t, []byte(tt.body), tt.field, tt.code)
		_, err := Decode([]byte(tt.body), catalog())
		if strings.Contains(err.Error(), "private") {
			t.Fatal("raw control contents escaped")
		}
	}
	for _, field := range []string{"environmentName", "tenancyId", "deploymentRegion", "regionMap", "compartmentIds", "mysqlServices", "ownership", "destinations", "routes", "pagerDutyRegion", "intakeTopicId", "operatorTopicId"} {
		v := preparationSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
}
