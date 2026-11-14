package reconciliation

import (
	"context"
	"fmt"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"strings"
	"testing"
)

func TestMaintenanceOversizeResourceRemainsVisibleWhilePeerDelivers(t *testing.T) {
	large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
	n := processingNotice(t, 1, processingDB, large)
	f := processingFixtureFor(t, n)
	s := f.run(t)
	if s.Accepted != 1 || s.Prepared != 1 || len(s.Issues) != 1 || s.Issues[0].Code != "payload_too_large" || s.Issues[0].ResourceID != large {
		t.Fatal("capacity issue hid or blocked fitting peer")
	}
	record := f.saved(t, n.Key().NoticeID).Snapshot()
	if len(record.Batches) != 1 || len(record.Batches[0].Message.Resources) != 1 || record.Batches[0].Message.Resources[0].ResourceID != processingDB {
		t.Fatal("oversize member assigned or mixed into message")
	}
}

func TestMaintenanceStateCapacityPreservesPreparedAndAcceptedPeers(t *testing.T) {
	ids := make([]string, 256)
	for i := range ids {
		ids[i] = fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.%03d%s", i, strings.Repeat("x", 3300))
	}
	n := processingNotice(t, 1, ids...)
	f := processingFixtureFor(t, n)
	s := f.run(t)
	if s.Prepared == 0 || s.Accepted != s.Prepared || len(s.Issues) == 0 {
		t.Fatal("state-capacity boundary not exercised", s.Prepared, s.Accepted, len(s.Issues))
	}
	for _, issue := range s.Issues {
		if issue.Code != "state_capacity" {
			t.Fatal("wrong preparation capacity reason", issue.Code)
		}
	}
	if len(f.saved(t, n.Key().NoticeID).Snapshot().Revisions[0].Resources) != 256 {
		t.Fatal("capacity admission evicted resources")
	}
	observations := savedPreparationIssues(t, f, n.Key().NoticeID).Snapshot().Observations
	if len(observations) == 0 {
		t.Fatal("state capacity failures not durable")
	}
	for _, observation := range observations {
		if observation.Code != "state_capacity" {
			t.Fatal("wrong retained preparation failure")
		}
	}
}

func TestMaintenanceReviewResultBoundsStopWithoutFalseCompletion(t *testing.T) {
	for _, largeText := range []bool{false, true} {
		ns := []notices.Notice{}
		for number := 1; number <= 4; number++ {
			ids := []string{}
			count := 256
			if largeText {
				count = 1
			}
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.%03d", i)
				if largeText {
					id += strings.Repeat("x", 280000)
				}
				ids = append(ids, id)
			}
			ns = append(ns, processingNotice(t, number, ids...))
		}
		f := processingFixtureFor(t, ns...)
		f.p.mysql["us-ashburn-1"] = func(_ context.Context, id string) (routing.Metadata, error) {
			return routing.Metadata{ID: chain(id), CompartmentID: chain("ocid1.compartment.oc1..example")}, nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		e, ok := err.(*ProcessingError)
		if !ok || e.Code != "result_capacity" || r.Snapshot().Completed || f.sent.Load() != 0 {
			t.Fatal("result bound did not fail closed", err)
		}
		s := r.Snapshot()
		if len(s.Issues) > 1000 || len(s.Issues) == 0 {
			t.Fatal("invalid retained result bound")
		}
		original := s.Issues[0].Code
		s.Issues[0].Code = "changed"
		if r.Snapshot().Issues[0].Code != original {
			t.Fatal("result aliases caller view")
		}
	}
}
