package configuration

import (
	"strings"
	"testing"
)

func TestRecoveryControlsRequireExactTypedFields(t *testing.T) {
	for _, field := range []string{"requestId", "noticeId", "kind", "targetId", "reviewFingerprint", "routingGeneration", "componentFingerprint"} {
		v := recoverySettings(t, "RETRY_BATCH")
		delete(v["recovery"].(map[string]any), field)
		wantError(t, encoded(t, v), "recovery."+field, "required")
		for _, bad := range []any{nil, true, 1, []any{}, map[string]any{}} {
			v := recoverySettings(t, "RETRY_BATCH")
			v["recovery"].(map[string]any)[field] = bad
			wantError(t, encoded(t, v), "recovery."+field, "type")
		}
	}
	for _, bad := range []any{nil, true, 1, "private", []any{}, []any{recoverySettings(t, "RETRY_BATCH")["recovery"]}} {
		v := recoverySettings(t, "RETRY_BATCH")
		v["recovery"] = bad
		wantError(t, encoded(t, v), "recovery", "type")
	}
}

func TestRecoveryControlsValidateExactBoundsAndKinds(t *testing.T) {
	cases := map[string][]string{
		"requestId": {"", "with space", "a.b", "é", strings.Repeat("a", 129)},
		"noticeId":  {"", "ocid1.announcement.oc2..a", "ocid1.announcement.oc1.iad.a", "ocid1.instance.oc1..a", "ocid1.announcement.oc1..", "ocid1.announcement.oc1../a", strings.Repeat("a", 4097)},
		"kind":      {"", "RETRY", "retry_batch", "CLOSE", "APPROVE_REVISION "},
	}
	for _, f := range []string{"targetId", "reviewFingerprint", "routingGeneration", "componentFingerprint"} {
		cases[f] = []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}
	}
	for field, values := range cases {
		for _, value := range values {
			v := recoverySettings(t, "RETRY_BATCH")
			v["recovery"].(map[string]any)[field] = value
			wantError(t, encoded(t, v), "recovery."+field, "invalid")
		}
	}
	for _, id := range []string{"a", strings.Repeat("A_-9", 32)} {
		v := recoverySettings(t, "RETRY_BATCH")
		v["recovery"].(map[string]any)["requestId"] = id
		if decoded(t, v).Recovery.RequestID != id {
			t.Fatal("valid ID normalized")
		}
	}
	for _, id := range []string{"ocid1.announcement.oc1..future.opaque", "ocid1.announcement.oc1.." + strings.Repeat("a", 4096-len("ocid1.announcement.oc1.."))} {
		v := recoverySettings(t, "RETRY_BATCH")
		v["recovery"].(map[string]any)["noticeId"] = id
		if decoded(t, v).Recovery.NoticeID != id {
			t.Fatal("valid notice reference normalized")
		}
	}
	v := recoverySettings(t, "RETRY_BATCH")
	v["recovery"].(map[string]any)["noticeId"] = "ocid1.announcement.oc1.." + strings.Repeat("a", 4097-len("ocid1.announcement.oc1.."))
	wantError(t, encoded(t, v), "recovery.noticeId", "invalid")
}

func TestClosureRecoveryForbidsComponentPrerequisite(t *testing.T) {
	for _, kind := range []string{"CLOSE_BATCH", "CLOSE_REVISION"} {
		decoded(t, recoverySettings(t, kind))
		for _, value := range []any{"", strings.Repeat("a", 64), nil} {
			v := recoverySettings(t, kind)
			v["recovery"].(map[string]any)["componentFingerprint"] = value
			wantError(t, encoded(t, v), "recovery.componentFingerprint", "forbidden")
		}
	}
}
