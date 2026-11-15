package deployment

import (
	"reflect"
	"strings"
	"testing"
)

func TestClosureReviewProjectsEverySavedSchemaWithoutMigrationWrites(t *testing.T) {
	for _, schema := range []int{1, 2, 3} {
		f := newSavedReviewFixture(t, "ocid1.announcement.oc1..legacy-closure", 0, false)
		if schema == 1 {
			for name, obj := range f.objects.objects {
				if !strings.HasPrefix(name, "notices/") {
					continue
				}
				body := string(obj.Body)
				start := strings.Index(body, `,"sourceOutcome":`)
				end := strings.Index(body, `,"delivery":`)
				obj.Body = []byte(strings.Replace(body[:start]+body[end:], `"schemaVersion":2`, `"schemaVersion":1`, 1))
				f.objects.objects[name] = obj
			}
		}
		if schema == 3 {
			next, err := f.record.MigrateRecovery(reviewTime())
			if err != nil {
				t.Fatal(err)
			}
			f.save(t, next)
		}
		before := businessObjects(f.objects)
		got := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, "CLOSE_REVISION", f.notice.Key().Digest, ""))
		if got["status"] != "reviewed" || got["closureReview"].(map[string]any)["noticeSchemaVersion"] != float64(schema) || !reflect.DeepEqual(before, businessObjects(f.objects)) {
			t.Fatal("legacy review mutated or failed", schema, got)
		}
		rows := got["closureReview"].(map[string]any)["items"].([]any)
		if rows[len(rows)-1].(map[string]any)["kind"] != "CLOSURE" {
			t.Fatal("empty revision cannot be reviewed")
		}
	}
}
