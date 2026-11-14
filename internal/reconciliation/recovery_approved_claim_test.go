package reconciliation

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"strings"
	"testing"
	"time"
)

// These are admission tests at the actual conditional claim boundary, using
// real lease, selected baseline and stored handles; they do not bypass Run in
// production or stand in for its complete source/ownership checks.
func TestRecoveryApprovedClaimRequiresCapacityAuthorityAndMembership(t *testing.T) {
	for _, fault := range []string{"capacity", "current", "association", "component"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := seedApprovedRecovery(t, f, n, false)
		if fault == "association" {
			plain := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, false)
			var err error
			old, err = plain.MigrateRecovery(time.Now())
			if err != nil {
				t.Fatal(err)
			}
			body, _ := old.Encode()
			f.live.objects.seed(noticeName(n.Key().NoticeID), body)
		}
		ctx := leaseBudget(t)
		r := processingAttemptRun(t, f, ctx, strings.Repeat("d", 64))
		saved, err := r.store.Load(ctx, n.Key().NoticeID)
		if err != nil {
			t.Fatal(err)
		}
		want := "approval_changed"
		work := processingNoticeWork{component: savedMembership(old)}
		switch fault {
		case "capacity":
			r.result.snapshot.Attempts = make([]ProcessingAttempt, 1000)
			want = "result_capacity"
		case "current":
			f.live.objects.seed("configuration/router.json", []byte("changed"))
			want = "configuration_changed"
		case "component":
			work.component = strings.Repeat("f", 64)
			want = "approval_blocked"
		}
		writes := len(f.live.objects.noticeWrites())
		_, err = r.claimApproved(saved, saved.Record().Snapshot().Batches[0], bytes.Repeat([]byte{'k'}, 32), work)
		if err == nil || processingFailure(ctx, err).Code != want || len(f.live.objects.noticeWrites()) != writes || f.sent.Load() != 0 {
			t.Fatal("invalid approved claim acquired dispatch authority", fault, err)
		}
	}
}
