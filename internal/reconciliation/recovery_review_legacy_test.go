package reconciliation

import (
	"bytes"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestRecoveryReviewMigratesLegacyOnlyInMemory(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	record := seedReviewRevision(t, f, n)
	body, err := record.Encode()
	if err != nil {
		t.Fatal(err)
	}
	raw := string(body)
	start, end := strings.Index(raw, `,"sourceOutcome":`), strings.Index(raw, `,"delivery":`)
	if start < 0 || end <= start {
		t.Fatal("invalid legacy fixture")
	}
	body = []byte(strings.Replace(raw[:start]+raw[end:], `"schemaVersion":2`, `"schemaVersion":1`, 1))
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	r := reviewFixture(t, f)
	view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
	after, _ := f.saved(t, n.Key().NoticeID).Encode()
	if err != nil || view.Saved.Snapshot().SchemaVersion != 1 || view.Projected.Snapshot().SchemaVersion != 3 || view.Fingerprint == "" || view.Blocker != "" || !bytes.Equal(body, after) {
		t.Fatal("legacy review mutated or failed", view.Blocker, err)
	}
}
