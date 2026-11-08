package delivery_test

import (
	"reflect"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
)

func TestRecordKeepsFirstMessageAndPin(t *testing.T) {
	r, values := prepared(t, db)
	r = restored(t, r)
	want := r.Batches()
	if len(want) != 1 || want[0].State != delivery.Prepared || want[0].SecretVersion != 7 || want[0].AttemptCount != 0 || !want[0].UpdatedAt.IsZero() {
		t.Fatalf("incorrect preparation: %+v", want)
	}
	source := record(db)
	source.TimeUpdated = text("2026-03-01T00:00:00Z")
	b := builder(t, func(c map[string]any) {
		c["environmentName"] = "renamed"
		c["pagerDutyRegion"] = "EU"
		c["destinations"] = []map[string]string{{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.iad.changed"}, {"id": "video", "secretId": "ocid1.vaultsecret.oc1.iad.video"}}
	})
	changed := preparations(t, b, source, db)
	changed[0].SecretVersion = 99
	if reflect.DeepEqual(values[0].Draft.Snapshot(), changed[0].Draft.Snapshot()) {
		t.Fatal("fixture did not change settings")
	}
	after, err := r.Prepare(changed...)
	if err != nil || !reflect.DeepEqual(after.Batches(), want) {
		t.Fatalf("first bytes/pin were replaced: %v", err)
	}
	after, err = r.Prepare()
	if err != nil || !reflect.DeepEqual(after.Batches(), want) {
		t.Fatal("empty append changed record")
	}
	s := after.Batches()
	s[0].Message.Resources[0].ResourceID = "changed"
	s[0].Message.PayloadWithoutRoutingKey[0] = 'x'
	s[0].SecretVersion = 2
	if !reflect.DeepEqual(r.Batches(), want) || !reflect.DeepEqual(after.Batches(), want) {
		t.Fatal("record exposed mutable state")
	}
	if _, ok := r.Batch("missing"); ok {
		t.Fatal("unknown batch exists")
	}
}

func TestRecordRejectsInvalidAndForeignPreparationsAtomically(t *testing.T) {
	r, values := prepared(t, db)
	otherNotice := record(db)
	otherNotice.NoticeID = "ocid1.announcement.oc1..other"
	foreign, err := delivery.New(settings(t, nil), catalog(), configuration.LedgerLocation{Namespace: "other", Bucket: "ledger"})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]delivery.Preparation{
		{{}}, {{Draft: values[0].Draft}}, {{Draft: values[0].Draft, SecretVersion: -1}},
		preparations(t, builder(t, nil), otherNotice, db),
		preparations(t, foreign, record(db), db), make([]delivery.Preparation, 257),
	} {
		after, err := r.Prepare(invalid...)
		rejectedRecord(t, after, err)
		if len(r.Batches()) != 1 || r.Batches()[0].State != delivery.Prepared {
			t.Fatal("failed append mutated original")
		}
	}
	zero, err := delivery.NewRecord()
	rejectedRecord(t, zero, err)
	zero, err = (delivery.Record{}).Prepare(values...)
	rejectedRecord(t, zero, err)
}
