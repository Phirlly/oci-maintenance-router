package deployment

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func reviewedClosureIntent(t *testing.T, f savedReviewFixture, id string) map[string]any {
	t.Helper()
	rows := closurePages(t, f, "CLOSE_REVISION", f.notice.Key().Digest)
	body, err := base64.StdEncoding.DecodeString(rows[len(rows)-1]["code"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var intent map[string]any
	if json.Unmarshal(body, &intent) != nil {
		t.Fatal("bad review code")
	}
	delete(intent, "version")
	intent["requestId"] = id
	return intent
}

func applyClosureIntent(t *testing.T, f *savedReviewFixture, intent map[string]any) {
	t.Helper()
	f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) { v["recovery"] = intent })
}

func TestClosureRejectsStaleReviewAndRequiresNewSameNoticeIntentID(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..stale-close", 1, false)
	intent := reviewedClosureIntent(t, f, "c1-one")
	next, err := f.record.RecordSourceUnavailable(404, reviewTime().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	f.save(t, next)
	applyClosureIntent(t, &f, intent)
	for i := 0; i < 2; i++ {
		got := invokeOperational(t, f.h, closeInvocation(f.s, "c1-one"))
		if got.Status != "not-ready" || got.Code != "closure_not_completed" || got.Recovery == nil || got.Recovery.Status != "REJECTED" || got.Recovery.Reason != "stale_review" {
			t.Fatal("stale request accepted", got)
		}
	}
	updated := reviewedClosureIntent(t, f, "c1-one")
	applyClosureIntent(t, &f, updated)
	before := businessObjects(f.objects)
	got := invokeOperational(t, f.h, closeInvocation(f.s, "c1-one"))
	if got.Status != "not-ready" || got.Code != "recovery_intent_conflict" || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("changed intent reused same identity", got)
	}
	updated["requestId"] = "c1-two"
	applyClosureIntent(t, &f, updated)
	got = invokeOperational(t, f.h, closeInvocation(f.s, "c1-two"))
	if got.Status != "closure-completed" {
		t.Fatal("new explicitly reviewed request refused", got)
	}
}

func TestClosureRequestIDIsIndependentAcrossNoticeRecords(t *testing.T) {
	for _, id := range []string{"ocid1.announcement.oc1..first", "ocid1.announcement.oc1..second"} {
		f := newSavedReviewFixture(t, id, 1, false)
		intent := reviewedClosureIntent(t, f, "c1-same-number")
		applyClosureIntent(t, &f, intent)
		got := invokeOperational(t, f.h, closeInvocation(f.s, "c1-same-number"))
		if got.Status != "closure-completed" {
			t.Fatal("per-notice request incorrectly treated as global", got)
		}
	}
}
