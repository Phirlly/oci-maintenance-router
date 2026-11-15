package ocimonitoring

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func verifySignature(t *testing.T, r *http.Request, key *rsa.PrivateKey, body []byte) {
	t.Helper()
	auth := r.Header.Get("Authorization")
	headers := regexp.MustCompile(`headers="([^"]+)"`).FindStringSubmatch(auth)
	signature := regexp.MustCompile(`signature="([^"]+)"`).FindStringSubmatch(auth)
	if len(headers) != 2 || len(signature) != 2 {
		t.Error("missing signature")
		return
	}
	want := "date (request-target) host"
	if r.Method == "POST" {
		want += " content-length content-type x-content-sha256"
		sum := sha256.Sum256(body)
		if r.Header.Get("x-content-sha256") != base64.StdEncoding.EncodeToString(sum[:]) {
			t.Error("wrong body digest")
		}
	}
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
		case "content-length":
			value = strconv.FormatInt(r.ContentLength, 10)
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
