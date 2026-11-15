package deployment

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func closureResponse(t *testing.T, h *Handler, input string) map[string]any {
	t.Helper()
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(input), out)
	var result map[string]any
	if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil || out.Body.Len() > 8192 {
		t.Fatalf("invalid bounded response: %v %s", err, out.Body)
	}
	for _, private := range []string{"private-reference-not-for-output", "vaultsecret", "ownerToken", "payload", "dedupKey", "secretVersion"} {
		if strings.Contains(out.Body.String(), private) {
			t.Fatalf("private field exposed: %s", private)
		}
	}
	return result
}

func TestClosureReviewProjectsLegacyTargetsWithoutWriting(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..closure", 1, false)
	before := businessObjects(f.objects)
	result := closureResponse(t, f.h, closureReviewRequest(f.notice.Snapshot().NoticeID, "", "", ""))
	if result["status"] != "reviewed" {
		t.Fatalf("catalog missing: %+v", result)
	}
	page := result["closureReview"].(map[string]any)
	if page["view"] != "TARGETS" || page["evidence"] != "LAST_SAVED" {
		t.Fatal(page)
	}
	target := f.record.Snapshot().Revisions[0].Digest
	result = closureResponse(t, f.h, closureReviewRequest(f.notice.Snapshot().NoticeID, "CLOSE_REVISION", target, ""))
	if result["status"] != "reviewed" {
		t.Fatal(result)
	}
	page = result["closureReview"].(map[string]any)
	items := page["items"].([]any)
	last := items[len(items)-1].(map[string]any)
	if last["kind"] != "CLOSURE" {
		t.Fatal("eligible target lacks final code", items)
	}
	body, err := base64.StdEncoding.DecodeString(last["code"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var intent map[string]any
	if json.Unmarshal(body, &intent) != nil || intent["noticeId"] != f.notice.Snapshot().NoticeID || intent["targetId"] != target || intent["kind"] != "CLOSE_REVISION" || intent["version"] != float64(1) {
		t.Fatal("wrong copied intent", intent)
	}
	if !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("read-only review changed business state")
	}
}
