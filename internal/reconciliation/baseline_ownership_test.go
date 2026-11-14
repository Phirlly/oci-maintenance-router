package reconciliation

import (
	"bytes"
	"sync"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestBaselineSnapshotsAndConcurrentReadsOwnTheirValues(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := baselineResult(t, []notices.Notice{n}, nil, nil, nil)
	b := buildBaseline(t, "initial", r)
	want := baselineWire(t, b)
	var done sync.WaitGroup
	for range 10 {
		done.Add(1)
		go func() {
			defer done.Done()
			layers := b.Layers()
			layers[0].Cells[0].CompartmentID = "caller change"
			layers[0].Revisions[0].Digest = "caller change"
			layers[0].CompletedAt = "caller change"
			body, err := b.Encode()
			if err != nil || !bytes.Equal(body, want) || !b.Covers(r) || !b.Suppresses("us-ashburn-1", baselineCompartment, n.Key()) {
				t.Error("concurrent snapshot changed baseline")
			}
			clear(body)
		}()
	}
	done.Wait()
	if !bytes.Equal(want, baselineWire(t, b)) {
		t.Fatal("returned encoding aliases retained state")
	}
	view := r.Candidates().Current()
	view[0].Revision.Digest = "caller change"
	again := buildBaseline(t, "initial", r)
	if !bytes.Equal(want, baselineWire(t, again)) {
		t.Fatal("discovery snapshot changed baseline inputs")
	}
}

func TestBaselineConstructionCanonicalizesInputOrderAndDuplicates(t *testing.T) {
	a, b := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	first := baselineResult(t, []notices.Notice{b, a, b}, nil, []string{baselineOtherCompartment, baselineCompartment}, []string{"us-phoenix-1", "us-ashburn-1"})
	second := baselineResult(t, []notices.Notice{a, b}, nil, []string{baselineCompartment, baselineOtherCompartment}, []string{"us-ashburn-1", "us-phoenix-1"})
	if !bytes.Equal(baselineWire(t, buildBaseline(t, "initial", first)), baselineWire(t, buildBaseline(t, "initial", second))) {
		t.Fatal("equivalent source/scope order changed canonical baseline")
	}
}
