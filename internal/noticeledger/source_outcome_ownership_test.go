package noticeledger

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestSourceOutcomeSnapshotsAndConcurrentProposalsAreOwned(t *testing.T) {
	r := observed(t, source(db), db)
	before := encoded(t, r)
	snapshot := r.Snapshot()
	snapshot.SourceOutcome.State = SourceUnknown
	snapshot.SourceOutcome.RevisionDigest = "mutated"
	snapshot.SourceOutcome.RecordedAt = "mutated"
	snapshot.SourceOutcome.HTTPStatus = 503
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			n := unavailable(t, r, 404, clock())
			n.Snapshot().SourceOutcome.HTTPStatus = 503
			if n.Snapshot().SourceOutcome.HTTPStatus != 404 || !bytes.Equal(before, encoded(t, r)) {
				t.Error("concurrent proposal or snapshot aliased source outcome")
			}
		})
	}
	wg.Wait()
	if !bytes.Equal(before, encoded(t, r)) || (Record{}).Snapshot().SourceOutcome != nil {
		t.Fatal("snapshot mutated source or zero record acquired evidence")
	}
}

func TestOtherTransitionsPreserveSourceOutcome(t *testing.T) {
	src := source(db)
	r := observed(t, src, db)
	want := *r.Snapshot().SourceOutcome
	r, err := r.Prepare(clock().Add(time.Second), preparations(t, src, db)...)
	if err != nil {
		t.Fatal(err)
	}
	r, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock().Add(time.Second))
	accepted, err := r.Finish(ref, 202, clock().Add(2*time.Second), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := r.Recover(clock().Add(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	held, err := accepted.Hold(delivery.ReasonOperator, clock().Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []Record{r, accepted, recovered, held} {
		if *n.Snapshot().SourceOutcome != want {
			t.Fatal("non-source transition changed evidence")
		}
		reload(t, n)
	}
}
