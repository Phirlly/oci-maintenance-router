package noticeledger

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func unavailable(t *testing.T, r Record, status int, at time.Time) Record {
	t.Helper()
	n, err := r.RecordSourceUnavailable(status, at)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUnavailableSourceRecordsOnlyAllowedStatusesAndRetainsHold(t *testing.T) {
	r := observed(t, source(db), db)
	before := encoded(t, r)
	for _, status := range []int{401, 403, 404, 408, 429, 500, 502, 503, 504} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			n := unavailable(t, r, status, clock())
			want := SourceOutcome{State: SourceUnavailable, RecordedAt: clock().Format(timeFormat), HTTPStatus: status}
			if !n.Follows(r) || n.Snapshot().SourceOutcome == nil || *n.Snapshot().SourceOutcome != want || n.Snapshot().HoldReason != delivery.ReasonSourceUnavailable || n.Snapshot().Revisions[0].Resources[0].Disposition != "REVIEW_REQUIRED" {
				t.Fatal("source failure or atomic hold missing")
			}
			reload(t, n)
		})
	}
	for _, status := range []int{-1, 0, 200, 400, 402, 409, 501, 505, 599, 600} {
		n, err := r.RecordSourceUnavailable(status, clock())
		rejected(t, n, err)
	}
	for _, at := range []time.Time{{}, clock().Add(-time.Second), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		n, err := r.RecordSourceUnavailable(404, at)
		rejected(t, n, err)
	}
	n, err := (Record{}).RecordSourceUnavailable(404, clock())
	rejected(t, n, err)
	if !bytes.Equal(before, encoded(t, r)) {
		t.Fatal("proposal mutated predecessor")
	}
}

func TestSourceSuccessNeverClearsExistingReview(t *testing.T) {
	r := observed(t, source(db), db)
	for _, reason := range []delivery.Reason{delivery.ReasonOperator, delivery.ReasonConfigurationChanged, delivery.ReasonSourceChanged, delivery.ReasonSecretUnavailable, delivery.ReasonSourceUnavailable} {
		held, err := r.Hold(reason, clock())
		if err != nil {
			t.Fatal(err)
		}
		lost := unavailable(t, held, 404, clock())
		lost = unavailable(t, lost, 503, clock().Add(time.Second))
		if lost.Snapshot().HoldReason != reason || lost.Snapshot().SourceOutcome.HTTPStatus != 503 {
			t.Fatal("prior hold masked or erased independent latest failure")
		}
		next, err := observer(t, nil).Observe(lost, normalized(t, source(db)), false, lookups(db), clock().Add(time.Minute))
		if err != nil || next.Snapshot().HoldReason != reason || next.Snapshot().SourceOutcome.State != SourceAvailable || next.Snapshot().Revisions[0].Resources[0].Disposition != "REVIEW_REQUIRED" {
			t.Fatal("source success cleared review", err)
		}
		reload(t, next)
	}
}

func TestUnavailableSourcePreservesActiveResultsAndPendingHolds(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	held := unavailable(t, r, 404, clock())
	if held.Snapshot().Batches[0].State != delivery.ReviewRequired {
		t.Fatal("pending batch remained eligible")
	}
	sending, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	lost := unavailable(t, sending, 503, clock())
	if !bytes.Equal(nestedDelivery(t, sending), nestedDelivery(t, lost)) {
		t.Fatal("in-flight attempt changed")
	}
	for _, status := range []int{202, 503, 0} {
		retryAt := clock()
		if status == 202 {
			retryAt = time.Time{}
		}
		n, err := lost.Finish(ref, status, clock(), retryAt)
		if err != nil {
			t.Fatal(err)
		}
		b := n.Snapshot().Batches[0]
		if (status == 202 && b.State != delivery.APIAccepted) || (status != 202 && b.State != delivery.ReviewRequired) || (status == 0 && !b.Ambiguous) || *n.Snapshot().SourceOutcome != *lost.Snapshot().SourceOutcome || b.SecretVersion != 7 || !bytes.Equal(b.Message.PayloadWithoutRoutingKey, r.Snapshot().Batches[0].Message.PayloadWithoutRoutingKey) {
			t.Fatal("late outcome lost receipt, ambiguity or original identity")
		}
		reload(t, n)
	}
	n, err := lost.Recover(clock().Add(time.Minute))
	if err != nil || n.Snapshot().Batches[0].State != delivery.ReviewRequired || !n.Snapshot().Batches[0].Ambiguous {
		t.Fatal("expired active result did not preserve ambiguity and review", err)
	}
	reload(t, n)
}
