package noticeledger

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func prepare(t *testing.T, r Record, p ...delivery.Preparation) Record {
	t.Helper()
	n, e := r.Prepare(clock(), p...)
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func TestPreparationPinsAssignmentsAndLateResolvedResources(t *testing.T) {
	src := source(db, otherDB)
	base := observed(t, src, db)
	p := preparations(t, src, db)
	r := prepare(t, base, p...)
	s := r.Snapshot()
	if len(s.Batches) != 1 || s.Revisions[0].Resources[0].BatchID != s.Batches[0].Message.BatchID || s.Revisions[0].Resources[0].Disposition != "ASSIGNED" || s.Revisions[0].Resources[1].Disposition != "UNRESOLVED" {
		t.Fatal("assignment did not preserve unresolved peer")
	}
	p[0].SecretVersion = 9
	again := prepare(t, r, p...)
	if again.Snapshot().Batches[0].SecretVersion != 7 || !bytes.Equal(encoded(t, again), encoded(t, r)) {
		t.Fatal("duplicate replaced original preparation")
	}
	look := lookups(db, otherDB)
	x := look[db]
	x.Metadata.FreeformTags["owner"] = "video"
	look[db] = x
	refreshed, e := observer(t, nil).Observe(r, normalized(t, src), false, look, clock())
	if e != nil {
		t.Fatal(e)
	}
	if refreshed.Snapshot().Revisions[0].Resources[0].DestinationID != "payments" {
		t.Fatal("existing assignment was retargeted")
	}
	final := prepare(t, refreshed, preparations(t, src, otherDB)...)
	if len(final.Snapshot().Batches) != 2 || len(base.Snapshot().Batches) != 0 {
		t.Fatal("late resource lost or original mutated")
	}
}

func TestPreparationRejectsMismatchedOrUnreadyWorkAtomically(t *testing.T) {
	src := source(db, otherDB)
	r := observed(t, src, db)
	valid := preparations(t, src, db)
	bad := preparations(t, src, otherDB)
	got, e := r.Prepare(clock(), append(valid, bad...)...)
	rejected(t, got, e)
	for _, p := range []delivery.Preparation{{}, {Draft: valid[0].Draft, SecretVersion: 0}} {
		got, e = r.Prepare(clock(), p)
		rejected(t, got, e)
	}
	changed := src
	changed.Reference = text("changed")
	got, e = r.Prepare(clock(), preparations(t, changed, db)...)
	rejected(t, got, e)
	got, e = r.Prepare(clock().Add(-time.Second), valid...)
	rejected(t, got, e)
	got, e = (Record{}).Prepare(clock(), valid...)
	rejected(t, got, e)
	got, e = r.Prepare(clock(), make([]delivery.Preparation, 257)...)
	rejected(t, got, e)
	if len(r.Snapshot().Batches) != 0 {
		t.Fatal("invalid preparation partially assigned")
	}
}
