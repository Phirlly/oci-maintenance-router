package delivery_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestIndependentBatchAndPayloadGolden(t *testing.T) {
	n := notice(t, record(db))
	s := single(t, build(t, builder(t, nil), n, map[string]routing.Lookup{db: metadata(db, "payments")}, db))
	// Independent Python standard-library JSON/SHA256 vector, not builder output.
	const tuple = `[1,"f8cffa22beb4df0f1675fa9016195e88248905435a8c65205e2916d8c6e2d8c0","ocid1.announcement.oc1..example","4ee26b11ad20285c32e3706088ab9f373e965f41b636c225a4edfa3d03f79875","payments",["ocid1.mysqldbsystem.oc1.iad.example"]]`
	const batch = "06d5eb1ef09820ac2ca5b2dd142537736950b13a1b18e89ca912a40ec353c7b7"
	sum := sha256.Sum256([]byte(tuple))
	if hex.EncodeToString(sum[:]) != batch || s.BatchID != batch || s.DedupKey != "omr1:"+batch || s.Revision != n.Key() {
		t.Fatalf("identity contract changed: %+v", s)
	}
	want, err := os.ReadFile("testdata/payload.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s.PayloadWithoutRoutingKey, bytes.TrimSuffix(want, []byte("\n"))) || s.PayloadDigest != "04c9a4ee7fa449d1184a2b6d77f2ebe3371200419792c0fae65e1b61caf42e91" {
		t.Fatalf("payload encoding/digest changed: %s %s", s.PayloadWithoutRoutingKey, s.PayloadDigest)
	}
}

func TestBatchIdentityChangesWithItsMeaning(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*notices.Record, map[string]routing.Lookup, *configuration.LedgerLocation)
	}{
		{"environment", func(r *notices.Record, m map[string]routing.Lookup, l *configuration.LedgerLocation) {
			l.Bucket = "other-ledger"
		}},
		{"notice", func(r *notices.Record, m map[string]routing.Lookup, l *configuration.LedgerLocation) {
			r.NoticeID = "ocid1.announcement.oc1..other"
		}},
		{"revision", func(r *notices.Record, m map[string]routing.Lookup, l *configuration.LedgerLocation) {
			r.Times[0].Title = text("Maintenance week begins")
		}},
		{"destination", func(r *notices.Record, m map[string]routing.Lookup, l *configuration.LedgerLocation) {
			m[db] = metadata(db, "video")
		}},
		{"resource", func(r *notices.Record, m map[string]routing.Lookup, l *configuration.LedgerLocation) {
			r.Resources[0].ResourceID = db + "replacement"
			m[db+"replacement"] = metadata(db+"replacement", "payments")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, m, l := record(db), map[string]routing.Lookup{db: metadata(db, "payments")}, ledger()
			before := single(t, build(t, builder(t, nil), notice(t, r), m, db))
			tc.change(&r, m, &l)
			b, err := delivery.New(settings(t, nil), catalog(), l)
			if err != nil {
				t.Fatal(err)
			}
			after := single(t, build(t, b, notice(t, r), m, r.Resources[0].ResourceID))
			if before.BatchID == after.BatchID {
				t.Fatal("meaning changed without identity")
			}
		})
	}
}

func TestSelectionOrderDoesNotChangeDrafts(t *testing.T) {
	other := db + "other"
	n := notice(t, record(other, db, db))
	m := map[string]routing.Lookup{other: metadata(other, "billing"), db: metadata(db, "payments")}
	b := builder(t, nil)
	a := single(t, build(t, b, n, m, other, db))
	z := single(t, build(t, b, n, m, db, other))
	if !reflect.DeepEqual(a, z) || len(a.Resources) != 2 || a.Resources[0].ResourceID != db {
		t.Fatal("unstable selection/group order")
	}
	// Same revision/destination, but a later resource must form a separate batch.
	later := single(t, build(t, b, n, m, other))
	if a.BatchID == later.BatchID || len(later.Resources) != 1 || later.Resources[0].ResourceID != other {
		t.Fatal("new selection grew an existing batch")
	}
}
