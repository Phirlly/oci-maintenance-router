package deployment

import (
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestSavedReviewPreservesPreparationClassificationAndHistory(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, true)
	f.preparationIssue(t, 0)
	for _, state := range []string{"LAST_BLOCKED", "RECHECK_REQUIRED", "HISTORICAL"} {
		if state == "RECHECK_REQUIRED" {
			f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) { v["mode"] = "PREVIEW" })
		}
		if state == "HISTORICAL" {
			r, err := f.record.MigrateRecovery(reviewTime())
			if err != nil {
				t.Fatal(err)
			}
			f.save(t, r)
			q := noticeledger.RecoveryRequest{RequestID: "close-example", Kind: noticeledger.CloseRevision, TargetID: r.Snapshot().Revisions[0].Digest, RoutingGeneration: r.Snapshot().Revisions[0].RoutingGeneration}
			q.ReviewFingerprint, err = r.ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, "")
			if err != nil {
				t.Fatal(err)
			}
			next, action, err := r.CloseRecovery(q, reviewTime())
			if err != nil || action.Status != noticeledger.RecoveryClosed {
				t.Fatal(action, err)
			}
			f.save(t, next)
		}
		before, writes := businessObjects(f.objects), f.objects.version
		got := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, "")
		if got.Status != "reviewed" || got.Review == nil || len(got.Review.Items) != 1 {
			t.Fatalf("review lost preparation evidence: %+v", got)
		}
		item := got.Review.Items[0]
		if item.Kind != "PREPARATION" || item.Code != "secret_unavailable" || item.Status != state || item.DestinationID != "payments" || item.LastSeenAt != "2026-01-01T00:00:00.000Z" || item.ResourceIndex != 0 || item.RevisionDigest != f.notice.Key().Digest {
			t.Fatalf("wrong projection: %+v", item)
		}
		if !reflect.DeepEqual(before, businessObjects(f.objects)) || f.objects.version != writes+2 {
			t.Fatal("review mutated retained history")
		}
	}
}

func TestSavedReviewSupportsLegacyWithoutPersistingMigration(t *testing.T) {
	for _, schema := range []int{1, 2} {
		f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, false)
		if schema == 1 {
			for name, obj := range f.objects.objects {
				body := string(obj.Body)
				start := strings.Index(body, `,"sourceOutcome":`)
				end := strings.Index(body, `,"delivery":`)
				body = strings.Replace(body[:start]+body[end:], `"schemaVersion":2`, `"schemaVersion":1`, 1)
				obj.Body = []byte(body)
				f.objects.objects[name] = obj
			}
		}
		before := businessObjects(f.objects)
		got := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, "")
		if got.Status != "reviewed" || got.Review == nil || got.Review.NoticeSchemaVersion != schema || len(got.Review.Items) != 1 {
			t.Fatalf("legacy review failed: %+v", got)
		}
		if schema == 1 && got.Review.SourceState != "UNKNOWN" {
			t.Fatal("legacy source claimed available")
		}
		if !reflect.DeepEqual(before, businessObjects(f.objects)) {
			t.Fatal("review migrated business state")
		}
	}
}
