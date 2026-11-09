package ocisecrets

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestSignedCurrentAndPinnedRequests(t *testing.T) {
	key := generatedKey(t)
	ids := []string{testSecretID, "ocid1.vaultsecret.oc1.us-example-1.example", "ocid1.vaultsecret.oc1.exm..example", "ocid1.vaultsecret.oc1.exm.future.example"}
	for _, id := range ids {
		for _, version := range []int64{0, 7, math.MaxInt64} {
			var calls atomic.Int32
			payload := []byte("synthetic-only-key")
			returned := version
			if returned == 0 {
				returned = 9
			}
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				query := "stage=CURRENT"
				if version != 0 {
					query = "versionNumber=" + strconv.FormatInt(version, 10)
				}
				if r.Method != "GET" || r.URL.RequestURI() != "/20190301/secretbundles/"+id+"?"+query || r.ProtoMajor != 1 {
					t.Error("method, path, selector or protocol changed")
				}
				verifySignature(t, r, key)
				body, _ := io.ReadAll(r.Body)
				if len(body) != 0 || r.Header.Get("Accept") != "application/json" || r.Header.Get("Accept-Encoding") != "" || r.Header.Get("Range") != "" {
					t.Error("unexpected retrieval body or headers")
				}
				_, _ = w.Write(bundleBody(t, id, returned, payload))
			})
			var got Bundle
			var err error
			if version == 0 {
				got, err = c.GetCurrent(budget(t), id)
			} else {
				got, err = c.GetVersion(budget(t), id, version)
			}
			if err != nil || got.Version() != returned || !bytes.Equal(got.Bytes(), payload) || calls.Load() != 1 {
				t.Fatalf("retrieval failed: %v calls%d", err, calls.Load())
			}
		}
	}
}

func TestCurrentRotationDoesNotRetargetPinnedVersion(t *testing.T) {
	var current atomic.Int64
	current.Store(1)
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		version := current.Load()
		if r.URL.Query().Get("versionNumber") != "" {
			version, _ = strconv.ParseInt(r.URL.Query().Get("versionNumber"), 10, 64)
		}
		_, _ = w.Write(bundleBody(t, testSecretID, version, []byte("synthetic-version-"+strconv.FormatInt(version, 10))))
	})
	first, err := c.GetCurrent(budget(t), testSecretID)
	if err != nil || first.Version() != 1 {
		t.Fatal("initial current selection failed")
	}
	current.Store(2)
	next, err := c.GetCurrent(budget(t), testSecretID)
	if err != nil || next.Version() != 2 || bytes.Equal(next.Bytes(), first.Bytes()) {
		t.Fatal("current result was cached across rotation")
	}
	retry, err := c.GetVersion(budget(t), testSecretID, first.Version())
	if err != nil || retry.Version() != 1 || !bytes.Equal(retry.Bytes(), first.Bytes()) {
		t.Fatal("pinned retrieval followed rotation")
	}
}

func TestUnavailablePinnedVersionNeverFallsBackToCurrent(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.RawQuery != "versionNumber=7" {
			t.Error("pinned request changed selector")
		}
		w.WriteHeader(404)
		_, _ = io.WriteString(w, "synthetic-private-error")
	})
	b, err := c.GetVersion(budget(t), testSecretID, 7)
	wantError(t, err, Service, 404)
	wantEmpty(t, b)
	if calls.Load() != 1 || err.Error() != "secret retrieval: service" {
		t.Fatal("fallback, retry or error disclosure")
	}
}
