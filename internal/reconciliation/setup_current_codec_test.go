package reconciliation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSetupCurrentCanonicalRecordAndStrictBinding(t *testing.T) {
	d, o := setupCurrentIntents(t, 1)
	r, err := newSetupCurrent(1, d, o)
	if err != nil {
		t.Fatal(err)
	}
	body, err := r.encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSetupCurrent(body, d.EnvironmentID, d.InstallationID)
	got, _ := decoded.encode()
	if err != nil || !bytes.Equal(body, got) || decoded.parent != nil {
		t.Fatal("canonical state failed to restart", err)
	}
	for _, private := range []string{d.SecretID, o.TopicID, o.SubscriptionID, "@", "receiptCode", "payload", "nonProduction"} {
		if bytes.Contains(body, []byte(private)) {
			t.Fatal("current selector copied channel/private state")
		}
	}
	for _, bad := range [][]byte{nil, []byte("{}"), append(bytes.Clone(body), '\n'), bytes.Replace(body, []byte(`"attempt":1`), []byte(`"attempt":1,"attempt":2`), 1), bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":2`), 1), []byte(strings.Repeat("x", 4097))} {
		if _, err := decodeSetupCurrent(bad, d.EnvironmentID, d.InstallationID); err == nil {
			t.Fatal("corrupt/noncanonical selector accepted")
		}
	}
	var v map[string]any
	_ = json.Unmarshal(body, &v)
	v["diagnosticRequestId"] = "d1-legacy"
	bad, _ := json.Marshal(v)
	if _, err := decodeSetupCurrent(bad, d.EnvironmentID, d.InstallationID); err == nil {
		t.Fatal("legacy request silently adopted")
	}
	if _, err := decodeSetupCurrent(body, strings.Repeat("c", 64), d.InstallationID); err == nil {
		t.Fatal("foreign environment accepted")
	}
}
