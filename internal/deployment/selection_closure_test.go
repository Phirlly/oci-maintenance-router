package deployment

import (
	"reflect"
	"testing"
)

func TestLegacyClosureReviewRefusesNewAnnouncementRecord(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..selected-closure", 0, false)
	three, err := f.record.MigrateRecovery(reviewTime())
	if err != nil {
		t.Fatal(err)
	}
	f.save(t, three)
	four, err := three.MigrateAnnouncements(reviewTime())
	if err != nil {
		t.Fatal(err)
	}
	f.save(t, four)
	before := businessObjects(f.objects)
	got := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, "CLOSE_REVISION", f.notice.Key().Digest, ""))
	if got["status"] == "reviewed" || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("older controls provided closure authority for notice4", got)
	}
	f.s = configureResourceSelection(t, f.h, f.s, f.objects, "PAUSED", "NONE", "instance")
	got = closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, "CLOSE_REVISION", f.notice.Key().Digest, ""))
	if got["status"] != "reviewed" || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("selected controls cannot review4 without mutation", got)
	}
	intent := reviewedClosureIntent(t, f, "selected-close")
	applyClosureIntent(t, &f, intent)
	closed := invokeOperational(t, f.h, closeInvocation(f.s, "selected-close"))
	if closed.Status != "closure-completed" {
		t.Fatal("selected PAUSED closure failed", closed)
	}
	after := businessObjects(f.objects)
	closed = invokeOperational(t, f.h, closeInvocation(f.s, "selected-close"))
	if closed.Status != "closure-completed" || !reflect.DeepEqual(after, businessObjects(f.objects)) {
		t.Fatal("selected closure repeated business work", closed)
	}
}
