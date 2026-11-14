package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"testing"
)

func TestMaintenanceExpandedRootBaselineDoesNotSuppressOldChildCell(t *testing.T) {
	rootDB, childDB, otherDB := processingDB, "ocid1.mysqldbsystem.oc1.iad.child", "ocid1.mysqldbsystem.oc1.iad.other"
	n := processingNotice(t, 1, rootDB, childDB, otherDB)
	f := processingFixtureFor(t, n)
	scope := []string{"ocid1.compartment.oc1..example", "ocid1.tenancy.oc1..example"}
	cfg := preparationConfig(t, func(v map[string]any) { v["compartmentIds"] = scope })
	prep := preparationFor(t, f.live.objects, cfg, []notices.Notice{n})
	prepareSuccessfully(t, prep, f.live.lease)
	f.configure(t, func(v map[string]any) { v["compartmentIds"] = scope })
	lookup := f.p.mysql["us-ashburn-1"]
	f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, e := lookup(ctx, id)
		if id == rootDB {
			m.CompartmentID = chain("ocid1.tenancy.oc1..example")
		}
		if id == otherDB {
			m.CompartmentID = chain("ocid1.compartment.oc1..unselected-child")
		}
		return m, e
	}
	s := f.run(t)
	if s.Suppressed != 1 || s.Accepted != 1 || len(s.Issues) != 0 {
		t.Fatal("scope layers were globally unioned or root treated recursively", s)
	}
	b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if len(b.Message.Resources) != 1 || b.Message.Resources[0].ResourceID != childDB {
		t.Fatal("wrong exact-cell recipients")
	}
}

func TestMaintenanceLateResolvedOwnerCreatesOnlyPreviouslyUnassignedWork(t *testing.T) {
	other := "ocid1.mysqldbsystem.oc1.iad.other"
	n := processingNotice(t, 1, processingDB, other)
	f := processingFixtureFor(t, n)
	lookup := f.p.mysql["us-ashburn-1"]
	f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, e := lookup(ctx, id)
		if id == other {
			delete(m.FreeformTags, "owner")
		}
		return m, e
	}
	first := f.run(t)
	if first.Accepted != 1 || len(first.Issues) != 1 {
		t.Fatal("partial owner resolution not exercised")
	}
	old := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	f.p.mysql["us-ashburn-1"] = lookup
	second := f.run(t)
	if second.Accepted != 1 || second.Prepared != 1 || f.sent.Load() != 2 || len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 2 {
		t.Fatal("late owner replayed assigned work")
	}
	for _, b := range f.saved(t, n.Key().NoticeID).Snapshot().Batches {
		if b.Message.BatchID == old.Message.BatchID && b.AttemptCount != old.AttemptCount {
			t.Fatal("old receipt replayed")
		}
	}
}
