package deployment

import (
	"reflect"
	"strings"
	"testing"
)

func reviewLiveFixture(t *testing.T) savedReviewFixture {
	t.Helper()
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..recovery", 1, true)
	f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) {
		v["mode"] = "LIVE"
		v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.diagnostic", "nonProduction": true, "receiptCode": "OMR-" + strings.Repeat("a", 32)}
		v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid", "receiptCode": "OMR-OP-" + strings.Repeat("a", 32)}
	})
	return f
}

func TestRecoveryCatalogIncludesTargetsWithoutIssueRows(t *testing.T) {
	f := reviewLiveFixture(t)
	before := businessObjects(f.objects)
	result := closureResponse(t, f.h, recoveryReviewRequest(f.notice.Key().NoticeID, "", "", ""))
	if result["status"] != "reviewed" {
		t.Fatal("missing catalog", result)
	}
	page := result["recoveryReview"].(map[string]any)
	if page["evidence"] != "LAST_SAVED" || page["view"] != "TARGETS" {
		t.Fatal("catalog overclaimed", page)
	}
	found := false
	for _, value := range page["items"].([]any) {
		row := value.(map[string]any)
		found = found || row["revisionId"] == f.notice.Key().Digest
		if _, ok := row["code"]; ok {
			t.Fatal("catalog issued action code")
		}
	}
	if !found || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("missing target or business mutation")
	}
}
