package reconciliation

import (
	"bytes"
	"context"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestSelectedDiscoveryPersistsEachMigrationBeforeObservation(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	base := candidateRecord(t, n)
	three, err := base.MigrateRecovery(leaseTime())
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []noticeledger.Record{discoveryLegacy(t, base), base, three, announcementRecordFor(t, three)} {
		for _, available := range []bool{false, true} {
			var current []notices.Notice
			if available {
				current = []notices.Notice{n}
			}
			d, s, lease := selectedDiscovery(t, current, []noticeledger.Record{old})
			// Each uncertain response must be confirmed exactly, then use its fresh ETag.
			s.after = func(_ context.Context, c discoveryCall) error {
				if c.name == noticeName(n.Key().NoticeID) && c.method == "REPLACE" {
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return nil
			}
			got, err := d.Run(leaseBudget(t), lease)
			if err != nil || got.EnvironmentID() == "" {
				t.Fatal("migration sequence failed", old.Snapshot().SchemaVersion, err)
			}
			writes := s.noticeWrites()
			if len(writes) != 5-old.Snapshot().SchemaVersion {
				t.Fatal("transition skipped or retried", len(writes))
			}
			for i, write := range writes {
				v, err := noticeledger.Decode(write.body, got.EnvironmentID(), n.Key().NoticeID)
				want := min(old.Snapshot().SchemaVersion+i+1, 4)
				if err != nil || v.Snapshot().SchemaVersion != want || write.tag == "" {
					t.Fatal("wrong immediate transition", err)
				}
			}
			last := s.record(t, got.EnvironmentID(), n.Key().NoticeID).Snapshot()
			want := noticeledger.SourceUnavailable
			if available {
				want = noticeledger.SourceAvailable
			}
			if last.SourceOutcome.State != want || last.Recovery == nil || len(last.Recovery.Actions) != 0 {
				t.Fatal("source/history lost", last)
			}
		}
	}
}

func TestLegacyDiscoveryRejectsAnnouncementRecordsWithoutWrites(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := announcementRecordFor(t, candidateRecord(t, n))
	for _, current := range [][]notices.Notice{nil, {n}} {
		d, s, lease := discoveryFixture(t, current, []noticeledger.Record{r})
		before, _ := r.Encode()
		got, err := d.Run(leaseBudget(t), lease)
		discoveryFailure(t, got, err, "unsupported_notice_schema")
		after, _ := s.record(t, lease.manager.environmentID, n.Key().NoticeID).Encode()
		if len(s.noticeWrites()) != 0 || !bytes.Equal(before, after) {
			t.Fatal("legacy control rewrote announcement record")
		}
	}
}
