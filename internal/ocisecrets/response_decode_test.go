package ocisecrets

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestDecodedBundleBindsIdentityAndVersion(t *testing.T) {
	payload := []byte{0, 255, 'k', 'e', 'y'}
	for _, version := range []int64{1, 7, math.MaxInt64} {
		for _, requested := range []int64{0, version} {
			b, err := decodeBundle(bundleBody(t, testSecretID, version, payload), testSecretID, requested)
			if err != nil || b.Version() != version || !bytes.Equal(b.Bytes(), payload) {
				t.Fatalf("version %d rejected: %v", version, err)
			}
		}
	}
	body := []byte(`{"metadata":{"arbitrary":"ignored"},"timeOfExpiry":"2000-01-01T00:00:00Z","secretId":"` + testSecretID + `","versionNumber":9,"secretBundleContent":{"contentType":"BASE64","content":"a2V5","name":"optional"}}`)
	if b, err := decodeBundle(body, testSecretID, 0); err != nil || b.Version() != 9 || string(b.Bytes()) != "key" {
		t.Fatal("optional metadata became an unsupported stage/expiry gate")
	}
}

func TestMalformedOrMismatchedBundlesReturnNoSecret(t *testing.T) {
	valid := string(bundleBody(t, testSecretID, 7, []byte("synthetic-key")))
	cases := map[string]string{
		"empty": "", "null": "null", "array": "[]", "incomplete": "{", "trailing": valid + `{}`,
		"incomplete_value":      `{"secretId":`,
		"missing_closing_brace": `{"secretId":"value"`,
		"invalid_member":        `{1:"value"}`,
		"identity":              strings.Replace(valid, testSecretID, "ocid1.vaultsecret.oc1.exm.other", 1),
		"missing_identity":      strings.Replace(valid, `"secretId":`, `"SecretId":`, 1),
		"null_identity":         strings.Replace(valid, `"`+testSecretID+`"`, "null", 1),
		"duplicate_identity":    strings.Replace(valid, "{", `{"secretId":"`+testSecretID+`",`, 1),
		"wrong_version":         strings.Replace(valid, `"versionNumber":7`, `"versionNumber":8`, 1),
		"zero_version":          strings.Replace(valid, `"versionNumber":7`, `"versionNumber":0`, 1),
		"negative_version":      strings.Replace(valid, `"versionNumber":7`, `"versionNumber":-1`, 1),
		"null_version":          strings.Replace(valid, `"versionNumber":7`, `"versionNumber":null`, 1),
		"string_version":        strings.Replace(valid, `"versionNumber":7`, `"versionNumber":"7"`, 1),
		"fraction":              strings.Replace(valid, `"versionNumber":7`, `"versionNumber":7.5`, 1),
		"overflow":              strings.Replace(valid, `"versionNumber":7`, `"versionNumber":9223372036854775808`, 1),
		"missing_content":       strings.Replace(valid, `"secretBundleContent":`, `"other":`, 1),
		"duplicate_content":     strings.Replace(valid, `"contentType":`, `"content":"a2V5","contentType":`, 1),
		"missing_discriminator": strings.Replace(valid, `"contentType":`, `"ContentType":`, 1),
		"unsupported":           strings.Replace(valid, "BASE64", "synthetic-private-discriminator", 1),
		"invalid_base64":        strings.Replace(valid, "c3ludGhldGljLWtleQ==", "not/base64!", 1),
		"invalid_utf8":          valid + string([]byte{255}),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := decodeBundle([]byte(body), testSecretID, 7)
			wantError(t, err, InvalidResponse, 200)
			wantEmpty(t, b)
			if err.Error() != "secret retrieval: invalid_response" {
				t.Fatal("decoder disclosed raw input")
			}
		})
	}
	for _, content := range []string{"null", "[]", `{}`, `{"contentType":"BASE64","content":null}`, `{"contentType":"BASE64","content":1}`, `{"contentType":"BASE64","content":""}`} {
		body := `{"secretId":"` + testSecretID + `","versionNumber":7,"secretBundleContent":` + content + `}`
		b, err := decodeBundle([]byte(body), testSecretID, 7)
		wantError(t, err, InvalidResponse, 200)
		wantEmpty(t, b)
	}
}

func TestDecodedContentLimitIsExact(t *testing.T) {
	for _, length := range []int{32 << 10, (32 << 10) + 1} {
		payload := bytes.Repeat([]byte{'k'}, length)
		b, err := decodeBundle(bundleBody(t, testSecretID, 7, payload), testSecretID, 7)
		if length == 32<<10 {
			if err != nil || !bytes.Equal(b.Bytes(), payload) {
				t.Fatalf("exact content limit rejected: %v", err)
			}
		} else {
			wantError(t, err, TooLarge, 200)
			wantEmpty(t, b)
		}
	}
}
