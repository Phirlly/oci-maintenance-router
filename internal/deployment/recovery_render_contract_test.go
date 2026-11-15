package deployment

import (
	"encoding/json"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func TestProducedRecoveryCodeSurvivesPinnedTerraformAndRuntimeDecoder(t *testing.T) {
	for _, kind := range []string{"RETRY_BATCH", "APPROVE_REVISION"} {
		for _, size := range []int{32, 4096} {
			prefix := "ocid1.announcement.oc1.."
			id := prefix + strings.Repeat("a", size-len(prefix))
			f, view := selectedRecoveryFixture(t, id, 1)
			produced, err := recoveryReviewCode(view, command{kind: kind, targetID: f.notice.Key().Digest})
			if err != nil {
				t.Fatal(err)
			}
			code := produced.(map[string]any)["code"].(string)
			body := renderProducedIntent(t, f, map[string]any{"mode": "LIVE", "recovery_confirm": true, "recovery_request_number": 1, "recovery_review_code": code, "test_run": 1, "test_secret_ocid": "ocid1.vaultsecret.oc1.iad.diagnostic", "test_nonproduction": true, "test_receipt_code": "OMR-" + strings.Repeat("a", 32), "operator_email": "operator@example.invalid", "operator_test_run": 1, "operator_receipt_code": "OMR-OP-" + strings.Repeat("a", 32)})
			p, failure := loadPinned(operationalContext(t), f.s, f.objects)
			if failure.Status != "" {
				t.Fatal(failure)
			}
			c, err := configuration.Decode(body, p.catalog)
			if err != nil || c.Recovery == nil || c.Recovery.NoticeID != id || c.Recovery.Kind != kind || c.Recovery.TargetID != f.notice.Key().Digest || c.Recovery.ReviewFingerprint != view.Fingerprint || c.Recovery.ComponentFingerprint != view.Component.Fingerprint || c.Recovery.RoutingGeneration != view.Routing.RoutingGeneration {
				t.Fatal("produced code changed intent", err)
			}
			var rendered map[string]any
			_ = json.Unmarshal(body, &rendered)
			if rendered["schemaVersion"] != float64(7) || len(code) > 6000 {
				t.Fatal("incompatible runtime contract")
			}
		}
	}
}
