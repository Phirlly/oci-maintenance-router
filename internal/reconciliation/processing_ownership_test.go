package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociownership"
	"oci-maintenance-router/internal/routing"
	"testing"
	"time"
)

func TestMaintenanceUsesCurrentTagsForSavedBatchWithoutBlockingPeer(t *testing.T) {
	a, b := processingDB, "ocid1.mysqldbsystem.oc1.iad.video"
	n := processingNotice(t, 1, a, b)
	f := processingFixtureFor(t, n)
	f.configure(t, func(v map[string]any) {
		v["destinations"] = []any{map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.iad.payments"}, map[string]any{"id": "video", "secretId": "ocid1.vaultsecret.oc1.iad.video"}}
		v["routes"] = []any{map[string]any{"ownerValue": "payments", "destinationId": "payments"}, map[string]any{"ownerValue": "video", "destinationId": "video"}}
	})
	lookup := f.p.mysql["us-ashburn-1"]
	f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, e := lookup(ctx, id)
		if id == b {
			m.FreeformTags["owner"] = "video"
		}
		return m, e
	}
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, false)
	read := f.p.mysql["us-ashburn-1"]
	f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, e := read(ctx, id)
		if id == a {
			m.FreeformTags["owner"] = "video"
		}
		return m, e
	}
	s := f.run(t)
	if s.Accepted != 1 || s.Prepared != 0 || f.current.Load() != 0 || len(s.Issues) != 1 || s.Issues[0].Code != "ownership_changed" {
		t.Fatal("changed owner not isolated", s)
	}
	for _, saved := range f.saved(t, n.Key().NoticeID).Snapshot().Batches {
		if saved.Message.DestinationID == "payments" && saved.State != delivery.ReviewRequired {
			t.Fatal("changed owner sent")
		}
		if saved.Message.DestinationID == "video" && saved.State != delivery.APIAccepted {
			t.Fatal("independent owner blocked")
		}
		for _, before := range old.Snapshot().Batches {
			if saved.Message.BatchID == before.Message.BatchID && saved.Message.PayloadDigest != before.Message.PayloadDigest {
				t.Fatal("saved payload rebuilt")
			}
		}
	}
}

func TestMaintenanceLookupFailuresStayUnresolvedAndDoNotInferDeletion(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		f.p.mysql["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) {
			return routing.Metadata{}, &ociownership.Error{Code: ociownership.Service, Status: status}
		}
		s := f.run(t)
		row := f.saved(t, n.Key().NoticeID).Snapshot().Revisions[0].Resources[0]
		if s.Accepted != 0 || len(s.Issues) != 1 || f.current.Load() != 0 || row.Disposition != "UNRESOLVED" {
			t.Fatal("lookup failure treated as delivery or deletion")
		}
	}
}

func TestMaintenanceBaselineSuppressesExactObservedPair(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	s := f.run(t)
	if s.Suppressed != 1 || s.Prepared != 0 || f.current.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("baseline pair replayed")
	}
}

func TestMaintenanceOutsideCompartmentIsSkippedWithoutReadingSecrets(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	lookup := f.p.mysql["us-ashburn-1"]
	f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, e := lookup(ctx, id)
		m.CompartmentID = chain("ocid1.compartment.oc1..child")
		return m, e
	}
	s := f.run(t)
	if s.Accepted != 0 || s.Prepared != 0 || len(s.Issues) != 0 || f.current.Load() != 0 {
		t.Fatal("unselected child included")
	}
}
