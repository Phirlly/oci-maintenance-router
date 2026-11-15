package deployment

import (
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestClosureReviewRefusesLegacyMigrationWithoutCapacity(t *testing.T) {
	for _, schema := range []string{"1", "2"} {
		t.Run(schema, func(t *testing.T) { checkClosureMigrationCapacity(t, schema) })
	}
}

func checkClosureMigrationCapacity(t *testing.T, schema string) {
	t.Helper()
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..full", 1, false)
	original, _ := f.record.Encode()
	if schema == "1" {
		body := string(original)
		start, end := strings.Index(body, `,"sourceOutcome":`), strings.Index(body, `,"delivery":`)
		original = []byte(strings.Replace(body[:start]+body[end:], `"schemaVersion":2`, `"schemaVersion":1`, 1))
	}
	encode := func(size int) []byte {
		return []byte(strings.Replace(string(original), "private-reference-not-for-output", strings.Repeat("x", size), 1))
	}
	low, high := 0, 1<<20
	for low < high {
		middle := (low + high + 1) / 2
		if _, err := noticeledger.Decode(encode(middle), f.environment, f.notice.Key().NoticeID); err == nil {
			low = middle
		} else {
			high = middle - 1
		}
	}
	body := encode(low)
	full, err := noticeledger.Decode(body, f.environment, f.notice.Key().NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	if schema == "1" {
		_, err = full.MigrateSourceOutcome(reviewTime())
	} else {
		_, err = full.MigrateRecovery(reviewTime())
	}
	if err == nil {
		t.Fatal("fixture requires migration capacity failure")
	}
	for name, obj := range f.objects.objects {
		if strings.HasPrefix(name, "notices/") {
			obj.Body = body
			f.objects.objects[name] = obj
		}
	}
	before := businessObjects(f.objects)
	got := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, "CLOSE_REVISION", f.notice.Key().Digest, ""))
	if got["code"] != "closure_projection_failed" || got["closureReview"] != nil || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("capacity failure changed or projected state", got)
	}
}
