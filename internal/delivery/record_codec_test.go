package delivery_test

import (
	"bytes"
	"os"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
)

func TestPreparedRecordIndependentGolden(t *testing.T) {
	r, _ := prepared(t, db)
	// Independently assembled with Python's JSON/SHA256 using the existing
	// synthetic payload vector and configuration, not the record encoder.
	want, err := os.ReadFile("testdata/prepared-record.json")
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.TrimSuffix(want, []byte("\n"))
	if !bytes.Equal(encoded(t, r), want) {
		t.Fatal("v1 record format changed")
	}
	restored(t, r)
}

func encoded(t *testing.T, r delivery.Record) []byte {
	t.Helper()
	data, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func restored(t *testing.T, r delivery.Record) delivery.Record {
	t.Helper()
	s := r.Batches()[0].Message
	got, err := delivery.DecodeRecord(encoded(t, r), s.EnvironmentID, s.Revision.NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Batches(), r.Batches()) {
		t.Fatal("reload changed saved state")
	}
	if !bytes.Equal(encoded(t, got), encoded(t, r)) {
		t.Fatal("reload changed saved bytes")
	}
	return got
}

func TestRecordRoundTripAllReachableStates(t *testing.T) {
	p, _ := prepared(t, db)
	s, ref := sending(t)
	wait, err := s.Finish(ref, 503, instant(), instant().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	network, err := s.Finish(ref, 0, instant(), instant())
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.Recover(instant().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := s.Finish(ref, 202, instant(), zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	permanent, err := s.Finish(ref, 400, instant(), zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	horizon, err := s.Recover(instant().Add(24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []delivery.Record{p, s, wait, network, expired, accepted, permanent, horizon} {
		restored(t, r)
	}
	for _, r := range []delivery.Record{p, wait, network, expired} {
		held, err := r.Hold(ref.BatchID, delivery.ReasonConfigurationChanged, instant().Add(2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		restored(t, held)
	}
	next, ref, err := attempt(restored(t, network), instant())
	if err != nil {
		t.Fatal(err)
	}
	accepted, err = next.Finish(ref, 202, instant(), zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	restored(t, accepted)
	data := encoded(t, accepted)
	data[0] = 'x'
	if encoded(t, accepted)[0] != '{' {
		t.Fatal("encoded buffer aliases record")
	}
}

func TestDiscardedTransitionsAndAmbiguousCrashUseSavedBytes(t *testing.T) {
	p, _ := prepared(t, db)
	s, ref, err := attempt(p, instant())
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := s.Finish(ref, 202, instant(), zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	if restored(t, p).Batches()[0].State != delivery.Prepared || restored(t, s).Batches()[0].State != delivery.Sending || accepted.Batches()[0].State != delivery.APIAccepted {
		t.Fatal("discarded candidate became durable")
	}
	// A crash before send and a crash after remote acceptance before saving the
	// receipt both leave these same SENDING bytes. Neither can imply acceptance.
	for _, scenario := range []string{"before_send", "after_remote_acceptance"} {
		t.Run(scenario, func(t *testing.T) {
			reloaded := restored(t, s)
			recovered, err := reloaded.Recover(instant().Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			retry, _, err := attempt(restored(t, recovered), instant().Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			b := retry.Batches()[0]
			if !b.Ambiguous || b.AttemptCount != 2 || b.SecretVersion != 7 || !reflect.DeepEqual(b.Message, p.Batches()[0].Message) {
				t.Fatal("crash rebuilt retry")
			}
		})
	}
}

func TestReloadChecksTrustedLookupContextAndZeroRecords(t *testing.T) {
	r, _ := prepared(t, db)
	s := r.Batches()[0].Message
	data := encoded(t, r)
	for _, ids := range [][2]string{{"", s.Revision.NoticeID}, {s.EnvironmentID, "ocid1.announcement.oc1..other"}, {"different", s.Revision.NoticeID}} {
		got, err := delivery.DecodeRecord(data, ids[0], ids[1])
		rejectedRecord(t, got, err)
	}
	if data, err := (delivery.Record{}).Encode(); err == nil || data != nil {
		t.Fatal("zero record encoded")
	}
}

func TestReloadPreservesOriginalSourceTimeAndUnicode(t *testing.T) {
	for _, stamp := range []string{"0000-01-01T00:00:00Z", "9999-12-31T23:59:59.999Z"} {
		source := record(db)
		source.TimeUpdated = text(stamp)
		source.Reference = text("shared<>&\u2028🐬")
		source.Times[0] = notices.SemanticTime{Title: text(""), Type: text("START_TIME"), Value: text(stamp)}
		r, err := delivery.NewRecord(preparations(t, builder(t, nil), source, db)...)
		if err != nil {
			t.Fatal(err)
		}
		restored(t, r)
	}
}
