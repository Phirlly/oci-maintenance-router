package noticeledger

import (
	"bytes"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

func TestCapturedGenerationHoldsExistingPendingBatch(t *testing.T) {
	src := source(db)
	base := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	changed := observer(t, func(v map[string]any) { v["routes"].([]any)[0].(map[string]any)["destinationId"] = "video" })
	o, e := changed.Capture(normalized(t, src))
	if e != nil {
		t.Fatal(e)
	}
	next := applyCaptured(t, o, base)
	view := next.Snapshot()
	if view.Revisions[0].RoutingGeneration == base.Snapshot().Revisions[0].RoutingGeneration || view.Batches[0].State != delivery.ReviewRequired || view.Batches[0].Reason != delivery.ReasonConfigurationChanged {
		t.Fatal("captured generation failed to hold pending work")
	}
	reload(t, next)
}

func TestCapturedObservationCanBeReusedConcurrently(t *testing.T) {
	src := source(db)
	src.Reference = text("original")
	o := captured(t, normalized(t, src))
	before := observed(t, src)
	beforeBytes := encoded(t, before)
	want := encoded(t, applyCaptured(t, o, before))
	type result struct {
		data []byte
		err  error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		go func() {
			r, e := o.Apply(before, clock())
			if e != nil {
				results <- result{err: e}
				return
			}
			b, e := r.Encode()
			view := r.Snapshot()
			*view.Revisions[0].Source.Reference = "changed"
			view.Revisions[0].Resources[0].ResourceID = "changed"
			results <- result{b, e}
		}()
	}
	for i := 0; i < 8; i++ {
		got := <-results
		if got.err != nil || !bytes.Equal(want, got.data) {
			t.Fatal("shared observation changed", got.err)
		}
	}
	if !bytes.Equal(beforeBytes, encoded(t, before)) || !bytes.Equal(want, encoded(t, applyCaptured(t, o, before))) {
		t.Fatal("reuse mutated input or observation")
	}
}
