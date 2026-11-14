package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"testing"
)

func TestRecoveryAvailabilityDoesNotInventMembershipOrReviveApproval(t *testing.T) {
	for _, unavailablePeer := range []bool{false, true} {
		f, approved, other := linkedRecoveryFixture(t)
		f.configure(t, func(v map[string]any) { v["schemaVersion"] = 7 })
		unavailable, available := approved.Key().NoticeID, other.Key().NoticeID
		if unavailablePeer {
			unavailable, available = available, unavailable
		}
		f.p.discovery.collector.announcements = announcementDetails{
			page: func(context.Context, string) (ociannouncements.Page, error) {
				return ociannouncements.Page{IDs: []string{available}}, nil
			},
			get: func(ctx context.Context, id string) (notices.Notice, error) {
				if id == unavailable {
					return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
				}
				return f.source.get(ctx, id)
			},
		}
		s := f.run(t)
		a := recoveryActions(t, f, approved.Key().NoticeID)[0]
		if (a.InvalidatedAt == nil) != unavailablePeer || (s.Accepted == 1) != unavailablePeer {
			t.Fatal("availability changed component authority incorrectly", unavailablePeer, s)
		}
		f.p.discovery.collector.announcements = f.source
		f.run(t)
		if (f.sent.Load() == 1) != unavailablePeer || (!unavailablePeer && recoveryActions(t, f, approved.Key().NoticeID)[0].InvalidatedAt == nil) {
			t.Fatal("source restoration revived a retained invalidation")
		}
	}
}
