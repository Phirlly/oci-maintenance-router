package reconciliation

import "strings"

func recoveryConfiguration(v map[string]any) {
	v["schemaVersion"] = 7
	v["recovery"] = map[string]any{
		"requestId": "recovery-1", "noticeId": "ocid1.announcement.oc1..maintenance",
		"kind": "RETRY_BATCH", "targetId": strings.Repeat("a", 64),
		"reviewFingerprint": strings.Repeat("b", 64), "routingGeneration": strings.Repeat("c", 64),
		"componentFingerprint": strings.Repeat("d", 64),
	}
}
