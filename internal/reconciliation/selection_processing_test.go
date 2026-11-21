package reconciliation

import (
	"bytes"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
)

func TestSelectedProcessingDeliversAndRetainsAcceptedAnnouncements(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kinds, ids []string
	}{
		{"Compute", []string{"instance"}, []string{resourceInstance}},
		{"MySQL", []string{"mysqldbsystem"}, []string{processingDB}},
		{"mixed", []string{"mysqldbsystem", "instance"}, []string{processingDB, resourceInstance}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := selectedNotice(t, 1, "ACTION_REQUIRED", tc.ids...)
			f, setup := selectedProcessingFixture(t, tc.kinds, nil, n)
			got := f.run(t)
			if !got.Completed || got.Accepted != 1 || f.sent.Load() != 1 {
				t.Fatal("selected announcement not delivered", got)
			}
			before := f.saved(t, n.Key().NoticeID).Snapshot()
			if before.SchemaVersion != 4 || len(before.Batches) != 1 || before.Batches[0].State != delivery.APIAccepted || len(before.Batches[0].Message.Resources) != len(tc.ids) {
				t.Fatal("incorrect saved delivery", before)
			}
			if !bytes.Contains(before.Batches[0].Message.PayloadWithoutRoutingKey, []byte(`"announcementType":"ACTION_REQUIRED"`)) {
				t.Fatal("category absent from real producer")
			}
			f.run(t)
			after := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
			if f.sent.Load() != 1 || after.Message.BatchID != before.Batches[0].Message.BatchID || !bytes.Equal(after.Message.PayloadWithoutRoutingKey, before.Batches[0].Message.PayloadWithoutRoutingKey) || setup.diagnosticSends != 1 || setup.operatorSends != 1 {
				t.Fatal("repeat processing changed or resent accepted work")
			}
		})
	}
}

func TestSelectedProcessingUsesTypedHistoricalSuppression(t *testing.T) {
	n := selectedNotice(t, 1, "PRODUCTION_EVENT_NOTIFICATION", resourceInstance)
	arrived := selectedNotice(t, 2, "PRODUCTION_EVENT_NOTIFICATION", resourceInstance)
	f, _ := selectedProcessingFixture(t, []string{"instance"}, []notices.Notice{n}, n, arrived)
	got := f.run(t)
	if got.Accepted != 1 || got.Suppressed != 1 || f.sent.Load() != 1 || len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 0 {
		t.Fatal("Compute history did not distinguish old and new arrivals", got)
	}
}
