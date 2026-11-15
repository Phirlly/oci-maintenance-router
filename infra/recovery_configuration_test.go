package infra_test

import (
	"strings"
	"testing"
)

func recoveryIntent(kind string) map[string]any {
	v := closureIntent()
	v["kind"], v["componentFingerprint"] = kind, strings.Repeat("d", 64)
	return v
}
func recoverySettings(kind string) map[string]any {
	s := receiptSettings()
	s["mode"] = "LIVE"
	s["recovery_review_code"], s["recovery_request_number"], s["recovery_confirm"] = closureCode(recoveryIntent(kind)), 7, true
	return s
}
func renderRecovery(t *testing.T, s map[string]any) (map[string]string, string, error) {
	t.Helper()
	return renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
}
func TestRecoveryRendererKeepsExplicitActionAndStableIdentity(t *testing.T) {
	first := ""
	for _, kind := range []string{"RETRY_BATCH", "APPROVE_REVISION"} {
		s := recoverySettings(kind)
		values, message, err := renderRecovery(t, s)
		if err != nil {
			t.Fatal(message, err)
		}
		q := renderedConfig(t, values).Recovery
		if q == nil || q.Kind != kind || q.ComponentFingerprint != strings.Repeat("d", 64) || !strings.HasPrefix(q.RequestID, "r1-") {
			t.Fatal("missing exact sending intent", q)
		}
		if first != "" && q.RequestID != first {
			t.Fatal("changed content silently changed request identity")
		}
		first = q.RequestID
		s["recovery_request_number"] = 8
		values, message, err = renderRecovery(t, s)
		if err != nil || renderedConfig(t, values).Recovery.RequestID == first {
			t.Fatal("new number did not change intent", message, err)
		}
	}
}
func TestInactiveRecoveryControlsDoNotChangeConfiguration(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		s := receiptSettings()
		s["mode"] = mode
		before, message, err := renderRecovery(t, s)
		if err != nil {
			t.Fatal(message, err)
		}
		s["recovery_review_code"], s["recovery_request_number"], s["recovery_confirm"] = "stale", 0, false
		after, message, err := renderRecovery(t, s)
		if err != nil || before["configuration"] != after["configuration"] {
			t.Fatal("inactive intent changed routing", mode, message, err)
		}
	}
}
