package deployment

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func reviewInvocation(id, cursor string) string {
	fields := map[string]string{"operation": "review-notice", "noticeId": id}
	if cursor != "" {
		fields["cursor"] = cursor
	}
	body, _ := json.Marshal(fields)
	return string(body)
}

func businessObjects(objects *operationalStore) map[string]string {
	result := map[string]string{}
	for name, obj := range objects.objects {
		if name != "control/lease.json" {
			result[name] = obj.ETag + ":" + digestFixture(obj.Body)
		}
	}
	return result
}

func TestSavedReviewInspectsKnownNoticeWithoutExecutingRecovery(t *testing.T) {
	h, s, objects, env := operationalFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	_, _, id := configureRecovery(t, h, s, objects, env, noticeledger.CloseRevision)
	before, writes := businessObjects(objects), objects.version
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(reviewInvocation(id, "")), out)
	var got struct {
		Status string `json:"status"`
		Review struct {
			Evidence string `json:"evidence"`
			NoticeID string `json:"noticeId"`
			Items    []struct {
				Kind          string `json:"kind"`
				Code          string `json:"code"`
				ResourceIndex int    `json:"resourceIndex"`
			} `json:"items"`
		} `json:"review"`
	}
	if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Status != "reviewed" || got.Review.Evidence != "LAST_SAVED" || got.Review.NoticeID != id || len(got.Review.Items) != 1 {
		t.Fatalf("saved work was not exposed: %s", out.Body)
	}
	item := got.Review.Items[0]
	if item.Kind != "RESOURCE" || item.Code != "lookup_missing" || item.ResourceIndex != 0 {
		t.Fatalf("wrong retained problem: %+v", item)
	}
	if !reflect.DeepEqual(before, businessObjects(objects)) || objects.version != writes+2 {
		t.Fatal("inspection changed business state or did not acquire/release ownership")
	}
}
