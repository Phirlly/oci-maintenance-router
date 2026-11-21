package noticeledger

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestAnnouncementPreparationKeepsExactEnclosingFormatAndFirstPin(t *testing.T) {
	n := source(db, otherDB)
	r := migrateAnnouncements(t, recoveryRecord(t, observed(t, n, db, otherDB)))
	at := clock().Add(time.Second)
	first := preparations(t, n, db)
	r, err := r.Prepare(at, first...)
	if err != nil || r.data.delivery.SchemaVersion() != 3 {
		t.Fatal("first legacy draft produced wrong format", err)
	}
	before := r.Snapshot().Batches[0]
	first[0].SecretVersion = 99
	values := append(first, preparations(t, n, otherDB)...)
	next, err := r.Prepare(at, values...)
	if err != nil || next.data.delivery.SchemaVersion() != 3 || len(next.Snapshot().Batches) != 2 {
		t.Fatal(err)
	}
	got, _ := next.data.delivery.Batch(before.Message.BatchID)
	if got.SecretVersion != before.SecretVersion || !bytes.Equal(got.Message.PayloadWithoutRoutingKey, before.Message.PayloadWithoutRoutingKey) {
		t.Fatal("duplicate retargeted first work")
	}
	reload(t, next)
}

func TestAnnouncementApprovalPreparesEmptyAndExistingRecords(t *testing.T) {
	n := normalized(t, source(db, otherDB))
	r := migrateAnnouncements(t, recoveryRecord(t, observed(t, source(db, otherDB), db, otherDB)))
	at := clock().Add(time.Second)
	r, err := r.Hold(delivery.ReasonOperator, at)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{db, otherDB} {
		q := approvalRequest(t, r, n.Key().Digest)
		q.RequestID = []string{"first", "second"}[i]
		next, action, err := observer(t, nil).ApproveRecovery(r, q, n, false, lookups(db, otherDB), at, preparations(t, source(db, otherDB), id)...)
		if err != nil || action.Status != RecoveryApproved || next.data.delivery.SchemaVersion() != 3 {
			t.Fatal("approval lost outer format", err)
		}
		started, ref, err := next.BeginApproved(q.RequestID, action.BatchIDs[0], q.ComponentFingerprint, approvedContext(next, action.BatchIDs[0], at))
		if err != nil {
			t.Fatal(err)
		}
		r, err = started.Finish(ref, 202, at, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		reload(t, r)
	}
	for _, row := range r.Snapshot().Revisions[0].Resources {
		if row.Disposition != "API_ACCEPTED" {
			t.Fatal("receipt lost")
		}
	}
}

func TestAnnouncementPreparationRejectsWrongNestedFormatBeforePersistence(t *testing.T) {
	modern := announcementRecord(t, "announcement-mysql").data.delivery
	for _, version := range []int{0, 1, 2, 3, 5} {
		// Public Builder cannot yet produce modern drafts. Test the private
		// first-record adoption with an independently decoded modern record.
		bad, err := adoptDeliveryFormat(modern, version)
		if err == nil || bad.SchemaVersion() != 0 {
			t.Fatal("new record escaped old enclosing admission", version)
		}
		bad, err = prepareDeliveries(modern, version, nil)
		if err == nil || bad.SchemaVersion() != 0 {
			t.Fatal("existing wrong format admitted", version)
		}
	}
	next, err := adoptDeliveryFormat(modern, 4)
	if err != nil || next.SchemaVersion() != 3 {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2, 3, 4} {
		r, err := prepareDeliveries(delivery.Record{}, version, preparations(t, source(db), db))
		want := map[int]int{1: 1, 2: 1, 3: 2, 4: 3}[version]
		if err != nil || r.SchemaVersion() != want {
			t.Fatal("wrong first-record format", version, err)
		}
	}
}
