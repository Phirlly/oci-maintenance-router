package configuration

import (
	"strings"
	"testing"
)

const recoveryNotice = "ocid1.announcement.oc1..maintenance"

func recoverySettings(t *testing.T, kind string) map[string]any {
	t.Helper()
	v := liveSettings(t)
	v["schemaVersion"] = 7
	q := map[string]any{
		"requestId": "recovery-1", "noticeId": recoveryNotice, "kind": kind,
		"targetId": strings.Repeat("a", 64), "reviewFingerprint": strings.Repeat("b", 64),
		"routingGeneration": strings.Repeat("c", 64),
	}
	if kind == "RETRY_BATCH" || kind == "APPROVE_REVISION" {
		q["componentFingerprint"] = strings.Repeat("d", 64)
	}
	v["recovery"] = q
	return v
}

func TestRecoveryControlsPreserveOneOwnedRequestInEveryMode(t *testing.T) {
	for _, kind := range []string{"RETRY_BATCH", "APPROVE_REVISION", "CLOSE_BATCH", "CLOSE_REVISION"} {
		for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
			v := recoverySettings(t, kind)
			v["mode"] = mode
			c := decoded(t, v)
			want := RecoveryRequest{RequestID: "recovery-1", NoticeID: recoveryNotice, Kind: kind,
				TargetID: strings.Repeat("a", 64), ReviewFingerprint: strings.Repeat("b", 64), RoutingGeneration: strings.Repeat("c", 64)}
			if kind == "RETRY_BATCH" || kind == "APPROVE_REVISION" {
				want.ComponentFingerprint = strings.Repeat("d", 64)
			}
			if c.SchemaVersion != 7 || c.Mode != mode || c.Recovery == nil || *c.Recovery != want || c.Diagnostic.ReceiptCode != receiptCode || c.OperatorTest.ReceiptCode != operatorCode {
				t.Fatal("new schema lost exact recovery or existing controls")
			}
			second := decoded(t, v)
			c.Recovery.TargetID = "changed"
			v["recovery"].(map[string]any)["requestId"] = "changed"
			if *second.Recovery != want {
				t.Fatal("decoded request aliases another value")
			}
			delete(v, "recovery")
			if decoded(t, v).Recovery != nil {
				t.Fatal("absence invented a recovery request")
			}
		}
	}
}

func TestRecoveryControlsDoNotReplaceModeAndReceiptRequirements(t *testing.T) {
	for _, field := range []string{"mode", "installationId", "preparation"} {
		v := recoverySettings(t, "CLOSE_BATCH")
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
	for _, field := range []string{"diagnostic", "operatorTest"} {
		v := recoverySettings(t, "CLOSE_REVISION")
		delete(v, field)
		wantError(t, encoded(t, v), field, "required_for_live")
		v = recoverySettings(t, "CLOSE_REVISION")
		delete(v[field].(map[string]any), "receiptCode")
		wantError(t, encoded(t, v), field+".receiptCode", "required_for_live")
	}
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		v := recoverySettings(t, "RETRY_BATCH")
		v["mode"] = mode
		delete(v, "diagnostic")
		delete(v, "operatorTest")
		if mode == "TEST" {
			wantError(t, encoded(t, v), "diagnostic", "required_for_test")
		} else {
			decoded(t, v)
		}
	}
}

func TestRecoveryControlsOwnDecodedBytes(t *testing.T) {
	wire := encoded(t, recoverySettings(t, "RETRY_BATCH"))
	c, err := Decode(wire, catalog())
	if err != nil {
		t.Fatal(err)
	}
	want := *c.Recovery
	for i := range wire {
		wire[i] = 'x'
	}
	if *c.Recovery != want {
		t.Fatal("caller bytes changed retained request")
	}
}
