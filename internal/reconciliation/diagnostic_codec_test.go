package reconciliation

import (
	"bytes"
	"os"
	"testing"
)

func TestDiagnosticCanonicalRestartPreservesClaimAndMessage(t *testing.T) {
	body, err := os.ReadFile("testdata/diagnostic-sending.json")
	if err != nil {
		t.Fatal(err)
	}
	i := diagnosticIntentFixture()
	r, err := DecodeDiagnostic(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err != nil || r.Snapshot().State != DiagnosticSending || !r.Matches(i) {
		t.Fatal("canonical record could not restart", err)
	}
	encoded, err := r.Encode()
	if err != nil || !bytes.Equal(body, encoded) {
		t.Fatal("restart changed original bytes", err)
	}
	clear(body)
	s := r.Snapshot()
	clear(s.PayloadWithoutRoutingKey)
	s.Intent.SecretID = "mutated"
	clear(encoded)
	if r.Snapshot().Intent.SecretID != i.SecretID || !bytes.Contains(r.Snapshot().PayloadWithoutRoutingKey, []byte("test notification")) {
		t.Fatal("caller mutated saved message or intent")
	}
	c := diagnosticClaimFixture()
	accepted, err := r.Finish(diagnosticReference(c), 202, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := accepted.Encode()
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := DecodeDiagnostic(b, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err != nil || restarted.Snapshot().State != DiagnosticAccepted || restarted.Snapshot().SecretVersion != 7 {
		t.Fatal("accepted outcome lost on restart", err)
	}
}

func TestDiagnosticDecoderRejectsMalformedAndForeignState(t *testing.T) {
	body, err := os.ReadFile("testdata/diagnostic-sending.json")
	if err != nil {
		t.Fatal(err)
	}
	i := diagnosticIntentFixture()
	for _, bad := range [][]byte{nil, []byte("null"), []byte("[]"), []byte("private invalid"), append(bytes.Clone(body), ' '), bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":2`), 1), bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1), bytes.Replace(body, []byte(`"secretVersion":7`), []byte(`"secretVersion":7.0`), 1)} {
		r, err := DecodeDiagnostic(bad, i.EnvironmentID, i.InstallationID, i.RequestID)
		if err == nil || r.Snapshot().State != "" {
			t.Fatal("invalid private record accepted")
		}
	}
	for _, ids := range [][3]string{{"foreign", i.InstallationID, i.RequestID}, {i.EnvironmentID, "foreign", i.RequestID}, {i.EnvironmentID, i.InstallationID, "foreign"}} {
		r, err := DecodeDiagnostic(body, ids[0], ids[1], ids[2])
		diagnosticRejected(t, r, err, "identity_mismatch")
	}
}
