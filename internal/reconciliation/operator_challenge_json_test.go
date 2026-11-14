package reconciliation

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestOperatorChallengeJSONRejectsMalformedAndNoncanonicalState(t *testing.T) {
	body, err := os.ReadFile("testdata/operator-publishing.json")
	if err != nil {
		t.Fatal(err)
	}
	i := operatorIntentFixture(t)
	for _, bad := range [][]byte{
		nil, []byte("null"), []byte("[]"), []byte("private invalid"), append([]byte{255}, body...), append(bytes.Clone(body), ' '),
		[]byte("[" + strings.Repeat("[", 1000) + strings.Repeat("]", 1000) + "]"), append(bytes.Clone(body), []byte(`{}`)...),
		bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1.0`), 1),
		bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"SchemaVersion":1`), 1),
		bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"extra":null`), 1),
		bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1),
		bytes.Replace(body, []byte(`"requestId":"operator-test-1"`), []byte(`"requestId":"\ud800"`), 1),
		bytes.Replace(body, []byte(`"requestId":"operator-test-1"`), []byte(`"requestId":"\u006fperator-test-1"`), 1),
		bytes.Replace(body, []byte(`"ambiguous":false`), []byte(`"ambiguous":null`), 1),
		bytes.Replace(body, []byte(`,"acceptedAt":null`), nil, 1),
	} {
		r, err := DecodeOperatorChallenge(bad, i.EnvironmentID, i.InstallationID, i.RequestID)
		if err == nil || r.Snapshot().State != "" || r.Snapshot().Payload != nil {
			t.Fatal("invalid JSON supplied partial record")
		}
		if strings.Contains(err.Error(), "installation-1") {
			t.Fatal("record escaped diagnostic")
		}
	}
	for _, ids := range [][3]string{{"other", i.InstallationID, i.RequestID}, {i.EnvironmentID, "other", i.RequestID}, {i.EnvironmentID, i.InstallationID, "other"}} {
		r, err := DecodeOperatorChallenge(body, ids[0], ids[1], ids[2])
		operatorRejected(t, r, err, "identity_mismatch")
	}
	diagnostic, err := os.ReadFile("testdata/diagnostic-sending.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeOperatorChallenge(diagnostic, i.EnvironmentID, i.InstallationID, i.RequestID); err == nil {
		t.Fatal("PagerDuty state accepted as operator challenge")
	}
}

func TestOperatorChallengeExactDecoderByteBoundary(t *testing.T) {
	r := operatorChallengeFixture(t)
	body, _ := r.Encode()
	i := r.Snapshot().Intent
	atLimit := append(bytes.Clone(body), bytes.Repeat([]byte(" "), operatorChallengeLimit-len(body))...)
	got, err := DecodeOperatorChallenge(atLimit, i.EnvironmentID, i.InstallationID, i.RequestID)
	operatorRejected(t, got, err, "noncanonical")
	got, err = DecodeOperatorChallenge(append(atLimit, ' '), i.EnvironmentID, i.InstallationID, i.RequestID)
	operatorRejected(t, got, err, "too_large")
}
