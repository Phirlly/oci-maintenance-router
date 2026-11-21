package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestSelectedDiscoveryResumesOnlyConfirmedMigrationParents(t *testing.T) {
	for _, refusedVersion := range []int{2, 3, 4} {
		for _, conflict := range []bool{false, true} {
			n := candidateNotice(t, 1, nil, true)
			old := discoveryLegacy(t, candidateRecord(t, n))
			d, s, lease := selectedDiscovery(t, []notices.Notice{n}, []noticeledger.Record{old})
			source := d.collector.announcements.(announcementDetails)
			source.get = func(context.Context, string) (notices.Notice, error) { return n, nil }
			d.collector.announcements = source
			s.before = func(_ context.Context, c discoveryCall) error {
				if c.method != "REPLACE" || c.name != noticeName(n.Key().NoticeID) {
					return nil
				}
				r, err := noticeledger.Decode(c.body, lease.manager.environmentID, n.Key().NoticeID)
				if err != nil {
					t.Fatal(err)
				}
				if r.Snapshot().SchemaVersion != refusedVersion {
					return nil
				}
				if conflict {
					return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
				}
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			result, err := d.Run(leaseBudget(t), lease)
			code := "write_failed"
			if conflict {
				code = "conflict"
			}
			discoveryFailure(t, result, err, code)
			if got := s.record(t, lease.manager.environmentID, n.Key().NoticeID).Snapshot().SchemaVersion; got != refusedVersion-1 {
				t.Fatal("lost confirmed parent", got)
			}
			s.before = nil
			if _, err = d.Run(leaseBudget(t), lease); err != nil {
				t.Fatal("restart failed", err)
			}
			got := s.record(t, lease.manager.environmentID, n.Key().NoticeID).Snapshot()
			if got.SchemaVersion != 4 || got.SourceOutcome.State != noticeledger.SourceAvailable || len(got.Revisions) != 1 {
				t.Fatal("restart lost observation", got)
			}
		}
	}
}
