package reconciliation

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestDiscoveryRefusesRecoverySchemaBeforeNoticeMutation(t *testing.T) {
	for _, available := range []bool{false, true} {
		n := candidateNotice(t, 1, nil, true)
		old := candidateRecord(t, n)
		at, err := time.Parse(time.RFC3339Nano, old.Snapshot().UpdatedAt)
		if err != nil {
			t.Fatal(err)
		}
		r, err := old.MigrateRecovery(at)
		if err != nil {
			t.Fatal(err)
		}
		current := []notices.Notice{}
		if available {
			current = append(current, n)
		}
		d, s, lease := discoveryFixture(t, current, []noticeledger.Record{r})
		before, _ := r.Encode()
		result, err := d.Run(leaseBudget(t), lease)
		discoveryFailure(t, result, err, "unsupported_notice_schema")
		after, _ := s.record(t, lease.manager.environmentID, n.Key().NoticeID).Encode()
		if len(s.noticeWrites()) != 0 || !bytes.Equal(before, after) {
			t.Fatal("discovery adopted recovery schema")
		}
	}
}

func TestMaintenanceRefusesRecoverySchemaIntroducedAfterDiscovery(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	now := f.p.discovery.now
	reads := 0
	beforeWrites := -1
	var wanted []byte
	f.p.discovery.now = func() time.Time {
		at := now()
		reads++
		if reads == 4 {
			old := f.saved(t, n.Key().NoticeID)
			next, err := old.MigrateRecovery(at)
			if err != nil {
				t.Fatal(err)
			}
			wanted, err = next.Encode()
			if err != nil {
				t.Fatal(err)
			}
			f.live.objects.seed(noticeName(n.Key().NoticeID), wanted)
			beforeWrites = len(f.live.objects.noticeWrites())
		}
		return at
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || beforeWrites < 0 || f.sent.Load() != 0 {
		t.Fatal("processor adopted recovery schema", err)
	}
	after, _ := f.saved(t, n.Key().NoticeID).Encode()
	if len(f.live.objects.noticeWrites()) != beforeWrites || !bytes.Equal(wanted, after) {
		t.Fatal("processor mutated new recovery state")
	}
}
