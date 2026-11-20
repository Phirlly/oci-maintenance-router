package reconciliation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSetupConfirmationCodecStrictRoundTrip(t *testing.T) {
	_, _, r := setupStoreFixture(t)
	body, _ := r.Encode()
	got, err := DecodeSetupConfirmation(body, r.data.EnvironmentID, r.data.InstallationID, r.ID())
	if err != nil || got.ID() != r.ID() || !got.notBefore.IsZero() {
		t.Fatal("decode manufactured proof or lost identity", err)
	}
	encoded, _ := got.Encode()
	if !bytes.Equal(body, encoded) || len(body) > 4096 {
		t.Fatal("confirmation encoding changed")
	}
	body[0] = 'x'
	again, _ := got.Encode()
	if !bytes.Equal(encoded, again) {
		t.Fatal("decode retained caller buffer")
	}
	if _, err := (SetupConfirmation{}).Encode(); err == nil {
		t.Fatal("zero record encoded")
	}
}

func TestSetupConfirmationCodecRejectsMalformedOrForeignState(t *testing.T) {
	_, _, r := setupStoreFixture(t)
	body, _ := r.Encode()
	for _, kind := range []string{"large", "utf8", "json", "unknown", "duplicate", "space", "schema", "environment", "installation", "diagnostic-request", "operator-request", "diagnostic-hash", "operator-hash", "config-hash", "time", "lookup-environment", "lookup-installation", "lookup-id"} {
		t.Run(kind, func(t *testing.T) {
			input := bytes.Clone(body)
			env, installation, id := r.data.EnvironmentID, r.data.InstallationID, r.ID()
			var fields map[string]any
			_ = json.Unmarshal(input, &fields)
			field := map[string]string{"schema": "schemaVersion", "environment": "environmentId", "installation": "installationId", "diagnostic-request": "diagnosticRequestId", "operator-request": "operatorRequestId", "diagnostic-hash": "diagnosticSHA256", "operator-hash": "operatorSHA256", "config-hash": "configurationSHA256", "time": "confirmedAt"}[kind]
			if field != "" {
				fields[field] = "invalid!"
				if kind == "schema" {
					fields[field] = 2
				}
				input, _ = json.Marshal(fields)
			}
			switch kind {
			case "large":
				input = []byte(strings.Repeat(" ", 4097))
			case "utf8":
				input = []byte{0xff}
			case "json":
				input = []byte("{")
			case "unknown":
				input = append([]byte(`{"extra":true,`), body[1:]...)
			case "duplicate":
				input = append([]byte(`{"schemaVersion":1,`), body[1:]...)
			case "space":
				input = append(input, ' ')
			case "lookup-environment":
				env = strings.Repeat("f", 64)
			case "lookup-installation":
				installation = "elsewhere"
			case "lookup-id":
				id = "sc1-" + strings.Repeat("0", 64)
			}
			got, err := DecodeSetupConfirmation(input, env, installation, id)
			if err == nil || got.ID() != "" {
				t.Fatal("malformed/foreign confirmation accepted")
			}
		})
	}
}
