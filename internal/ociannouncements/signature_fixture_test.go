package ociannouncements

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func verifySignature(t *testing.T, r *http.Request, key *rsa.PrivateKey) {
	t.Helper()
	auth := r.Header.Get("Authorization")
	headers := regexp.MustCompile(`headers="([^"]+)"`).FindStringSubmatch(auth)
	signature := regexp.MustCompile(`signature="([^"]+)"`).FindStringSubmatch(auth)
	if len(headers) != 2 || len(signature) != 2 {
		t.Error("missing signature")
		return
	}
	want := "date (request-target) host"
	if headers[1] != want {
		t.Errorf("signed headers=%q", headers[1])
	}
	var parts []string
	for _, name := range strings.Fields(headers[1]) {
		value := r.Header.Get(name)
		switch name {
		case "host":
			value = r.Host
		case "(request-target)":
			value = strings.ToLower(r.Method) + " " + r.URL.RequestURI()
		}
		if value == "" {
			t.Errorf("missing signed field %s", name)
		}
		parts = append(parts, name+": "+value)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	sig, err := base64.StdEncoding.DecodeString(signature[1])
	if err != nil || rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig) != nil {
		t.Error("signature verification failed")
	}
}
