package configuration

import "testing"

func setupSettings(t *testing.T, mode, action string) map[string]any {
	t.Helper()
	v := preparationSettings(t)
	v["schemaVersion"], v["mode"] = 8, mode
	v["operatorChannel"] = map[string]any{"subscriptionId": operatorSubscription, "endpoint": "Team@example.invalid"}
	s := map[string]any{"action": action}
	switch action {
	case "SEND_TEST":
		s["attempt"], s["secretId"], s["nonProduction"] = 1, "ocid1.vaultsecret.oc1.iad.test", true
	case "CONFIRM":
		s["attempt"], s["pagerDutyCode"], s["operatorCode"] = 1, receiptCode, operatorCode
	}
	v["setup"] = s
	return v
}
