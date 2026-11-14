package delivery_test

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"os"
	"testing"
)

func TestRecoveryCodecMatchesIndependentVersionedGolden(t *testing.T) {
	p, _ := prepared(t, db)
	want, err := os.ReadFile("testdata/prepared-record-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.TrimSpace(want)
	b := p.Batches()[0]
	loaded, err := delivery.DecodeRecord(want, b.Message.EnvironmentID, b.Message.Revision.NoticeID)
	if err != nil || loaded.SchemaVersion() != 2 {
		t.Fatal("independent v2 vector rejected", err)
	}
	if !bytes.Equal(encoded(t, upgraded(t, p)), want) {
		t.Fatal("v2 representation differs from specification")
	}
	if !bytes.Equal(encoded(t, loaded), want) {
		t.Fatal("v2 restart rewrote bytes")
	}
}

func TestRecoveryCodecRequiresVersionSpecificKind(t *testing.T) {
	p, _ := prepared(t, db)
	v2, err := os.ReadFile("testdata/prepared-record-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	v2 = bytes.TrimSpace(v2)
	b := p.Batches()[0]
	for _, change := range [][2]string{
		{`,"attemptKind":""`, ""},
		{`"attemptKind":""`, `"attemptKind":null`},
		{`"attemptKind":""`, `"attemptKind":"MANUAL"`},
		{`"attemptKind":""`, `"attemptKind":"AUTOMATIC"`},
		{`"attemptKind":""`, `"attemptKind":"unknown"`},
		{`"attemptKind":""`, `"AttemptKind":""`},
		{`"attemptKind":""`, `"attemptKind":"","attemptKind":""`},
		{`"schemaVersion":2`, `"schemaVersion":1`},
		{`"schemaVersion":2`, `"schemaVersion":3`},
	} {
		body := bytes.Replace(v2, []byte(change[0]), []byte(change[1]), 1)
		if bytes.Equal(body, v2) {
			t.Fatal("invalid fixture did not change")
		}
		got, err := delivery.DecodeRecord(body, b.Message.EnvironmentID, b.Message.Revision.NoticeID)
		rejectedRecord(t, got, err)
	}
}
