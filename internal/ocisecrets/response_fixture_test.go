package ocisecrets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

const testSecretID = "ocid1.vaultsecret.oc1.exm.example"

func bundleBody(t *testing.T, id string, version int64, content []byte) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"secretId": id, "versionNumber": version,
		"secretBundleContent": map[string]any{"contentType": "BASE64", "content": base64.StdEncoding.EncodeToString(content)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func wantError(t *testing.T, err error, code Code, status int) {
	t.Helper()
	var got *Error
	if !errors.As(err, &got) || got.Code != code || got.Status != status {
		t.Fatalf("error=%v; want %s/status%d", err, code, status)
	}
}

func wantEmpty(t *testing.T, b Bundle) {
	t.Helper()
	if b.Version() != 0 || b.Bytes() != nil {
		t.Fatal("failure returned usable secret data")
	}
}
