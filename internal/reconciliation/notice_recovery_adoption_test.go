package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestDiscoveryPreservesRecoverySchemaAndRecordsAvailability(t *testing.T) {
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
		result, err := d.Run(leaseBudget(t), lease)
		if err != nil || result.EnvironmentID() == "" {
			t.Fatal("discovery rejected recovery schema", err)
		}
		after := s.record(t, lease.manager.environmentID, n.Key().NoticeID).Snapshot()
		want := noticeledger.SourceUnavailable
		if available {
			want = noticeledger.SourceAvailable
		}
		if after.SchemaVersion != 3 || after.Recovery == nil || len(after.Recovery.Actions) != 0 || after.SourceOutcome.State != want || len(after.Revisions) != 1 || after.Revisions[0].Digest != n.Key().Digest {
			t.Fatal("discovery changed retained history or lost availability")
		}
	}
}

func TestMaintenanceAdoptsRecoveryMigrationIntroducedAfterDiscovery(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	now := f.p.discovery.now
	reads := 0
	migrated := false
	f.p.discovery.now = func() time.Time {
		at := now()
		reads++
		if reads == 4 {
			old := f.saved(t, n.Key().NoticeID)
			next, err := old.MigrateRecovery(at)
			if err != nil {
				t.Fatal(err)
			}
			wanted, err := next.Encode()
			if err != nil {
				t.Fatal(err)
			}
			f.live.objects.seed(noticeName(n.Key().NoticeID), wanted)
			migrated = true
		}
		return at
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err != nil || !r.Snapshot().Completed || !migrated || f.sent.Load() != 1 {
		t.Fatal("processor rejected compatible recovery migration", err)
	}
	after := f.saved(t, n.Key().NoticeID).Snapshot()
	if after.SchemaVersion != 3 || len(after.Batches) != 1 || after.Batches[0].State != delivery.APIAccepted || len(after.Recovery.Actions) != 0 {
		t.Fatal("ordinary processing fabricated recovery authority")
	}
}
