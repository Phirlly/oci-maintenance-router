package reconciliation

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestRecoveryDoesNotImplicitlyAdoptAnnouncementFormat(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	at := time.Now().Add(-time.Minute)
	r := f.seedBatch(t, n, at, delivery.Prepared, true)
	r, err := r.MigrateRecovery(at)
	if err == nil {
		r, err = r.MigrateAnnouncements(at)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	q := reviewedRecovery(t, r, noticeledger.CloseRevision, n.Key().Digest)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	result, err := p.Run(leaseBudget(t), f.live.lease)
	if err == nil || !strings.Contains(err.Error(), "unsupported_notice_schema") || result.Snapshot().Completed || result.Snapshot().Recovery != nil {
		t.Fatal("runtime unexpectedly adopted notice4", err)
	}
	after, err := f.saved(t, n.Key().NoticeID).Encode()
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("runtime rewrote unsupported saved work", err)
	}
}
