package ocistorage

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
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
	if r.Method == "PUT" {
		want += " content-length content-type x-content-sha256"
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

func TestSignedWholeObjectRequests(t *testing.T) {
	key := generatedKey(t)
	for _, operation := range []string{"get", "create", "replace", "empty"} {
		t.Run(operation, func(t *testing.T) {
			var count atomic.Int32
			payload := []byte("saved\x00bytes雪")
			if operation == "empty" {
				payload = nil
			}
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				if r.URL.RequestURI() != "/n/example/b/ledger/o/notices%2Fa%20%3F%23%25%2F%E9%9B%AA" || r.ProtoMajor != 1 {
					t.Error("path or protocol changed")
				}
				verifySignature(t, r, key)
				body, _ := io.ReadAll(r.Body)
				if r.Header.Get("Range") != "" || r.Header.Get("Accept-Encoding") != "" {
					t.Error("partial/compressed request")
				}
				if operation == "get" {
					if r.Method != "GET" || len(body) != 0 || r.Header.Get("If-Match") != "" || r.Header.Get("If-None-Match") != "" {
						t.Error("unexpected get contract")
					}
				} else {
					if r.Method != "PUT" || string(body) != string(payload) || r.ContentLength != int64(len(payload)) || r.Header.Get("Content-Type") != "application/json" {
						t.Error("put bytes changed")
					}
					hash := sha256.Sum256(body)
					if r.Header.Get("X-Content-Sha256") != base64.StdEncoding.EncodeToString(hash[:]) {
						t.Error("body hash mismatch")
					}
					if operation == "replace" {
						if r.Header.Get("If-Match") != "\"opaque-value\"" || r.Header.Get("If-None-Match") != "" {
							t.Error("replace condition changed")
						}
					} else if r.Header.Get("If-None-Match") != "*" || r.Header.Get("If-Match") != "" {
						t.Error("create condition changed")
					}
				}
				w.Header().Set("ETag", "\"next-tag\"")
				if operation == "get" {
					_, _ = w.Write(payload)
				}
			})
			name := "notices/a ?#%/雪"
			var tag string
			var err error
			switch operation {
			case "get":
				var obj Object
				obj, err = c.Get(budget(t), name)
				tag = obj.ETag
				if string(obj.Body) != string(payload) {
					t.Error("read bytes changed")
				}
			case "replace":
				tag, err = c.Replace(budget(t), name, "\"opaque-value\"", payload)
			default:
				tag, err = c.Create(budget(t), name, payload)
			}
			if err != nil || tag != "\"next-tag\"" || count.Load() != 1 {
				t.Fatalf("operation failed: %q %v count%d", tag, err, count.Load())
			}
		})
	}
}

func TestUploadOwnsBytesBeforeSigning(t *testing.T) {
	payload := []byte("original")
	provider := syntheticKey{key: generatedKey(t), beforeKey: func() { copy(payload, "modified") }}
	c, _ := tlsClient(t, provider, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "original" {
			t.Error("caller mutation changed upload")
		}
		w.Header().Set("ETag", "saved")
	})
	if _, err := c.Create(budget(t), "state", payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != "modified" {
		t.Fatal("mutation boundary not exercised")
	}
}
