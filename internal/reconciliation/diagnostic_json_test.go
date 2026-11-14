package reconciliation

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestDiagnosticJSONRejectsAliasesUnicodeAndNoncanonicalFields(t *testing.T) {
	body, err := os.ReadFile("testdata/diagnostic-sending.json")
	if err != nil {
		t.Fatal(err)
	}
	i := diagnosticIntentFixture()
	for _, bad := range [][]byte{
		append([]byte{255}, body...), []byte("[" + strings.Repeat("[", 1000) + strings.Repeat("]", 1000) + "]"),
		bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"SchemaVersion":1`), 1),
		bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"extra":null`), 1),
		bytes.Replace(body, []byte(`"installationId":"installation-1"`), []byte(`"installationId":"installation-1","installationId":"installation-1"`), 1),
		bytes.Replace(body, []byte(`"nonProduction":true`), []byte(`"nonProduction":null`), 1),
		bytes.Replace(body, []byte(`"requestId":"test-1"`), []byte(`"requestId":"\ud800"`), 1),
		bytes.Replace(body, []byte(`"requestId":"test-1"`), []byte(`"requestId":"\u0074est-1"`), 1),
		bytes.Replace(body, []byte(`,"acceptedAt":null`), nil, 1),
		append(bytes.Clone(body), []byte(`{}`)...),
	} {
		r, err := DecodeDiagnostic(bad, i.EnvironmentID, i.InstallationID, i.RequestID)
		if err == nil || r.Snapshot().State != "" {
			t.Fatal("malformed/noncanonical JSON supplied a record")
		}
		if strings.Contains(err.Error(), "installation-1") {
			t.Fatal("record content escaped diagnostics")
		}
	}
}

func TestDiagnosticDecoderHasExactByteAdmissionBoundary(t *testing.T) {
	body, err := os.ReadFile("testdata/diagnostic-sending.json")
	if err != nil {
		t.Fatal(err)
	}
	i := diagnosticIntentFixture()
	atLimit := append(bytes.Clone(body), bytes.Repeat([]byte(" "), diagnosticLimit-len(body))...)
	r, err := DecodeDiagnostic(atLimit, i.EnvironmentID, i.InstallationID, i.RequestID)
	diagnosticRejected(t, r, err, "noncanonical")
	r, err = DecodeDiagnostic(append(atLimit, ' '), i.EnvironmentID, i.InstallationID, i.RequestID)
	diagnosticRejected(t, r, err, "too_large")
}
