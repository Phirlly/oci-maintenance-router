package ociownership

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const instanceID = "ocid1.instance.oc1.iad.example"

func TestComputeConstructorFixesEndpointWithoutCredentials(t *testing.T) {
	c, err := NewCompute("us-ashburn-1", "iad", tenancy, unusedProvider{})
	if err != nil || c == nil || c.endpoint != "https://iaas.us-ashburn-1.oraclecloud.com" {
		t.Fatal("Compute must bind its own standard OC1 control-plane endpoint")
	}
}

func TestComputeSignedReadPreservesExactIdentityAndFreshTags(t *testing.T) {
	key := generatedKey(t)
	c, err := NewCompute("us-ashburn-1", "iad", tenancy, syntheticKey{key: key})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	c, _ = bindTLSClient(t, c, func(w http.ResponseWriter, r *http.Request) {
		verifySignature(t, r, key)
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/20160918/instances/") || r.URL.RawQuery != "" || r.Header.Get("Accept") != "application/json" || !r.Close || r.ProtoMajor != 1 {
			t.Error("incorrect GetInstance request")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) != 0 {
			t.Error("GetInstance sent a body")
		}
		for _, h := range []string{"If-None-Match", "If-Match", "If-Modified-Since"} {
			if r.Header.Get(h) != "" {
				t.Error("conditional lookup enabled")
			}
		}
		owner := "payments"
		if calls.Add(1) > 1 {
			owner = "video"
		}
		id := strings.TrimPrefix(r.URL.Path, "/20160918/instances/")
		json.NewEncoder(w).Encode(map[string]any{"id": id, "compartmentId": tenancy, "freeformTags": map[string]string{"Owner": owner}})
	})
	ids := []string{instanceID, instanceID, "ocid1.instance.oc1.us-ashburn-1..example", "ocid1.instance.oc1.iad.future.example-A_1"}
	for i, id := range ids {
		m, err := c.Get(budget(t), id)
		if err != nil || m.ID == nil || *m.ID != id || m.CompartmentID == nil || *m.CompartmentID != tenancy {
			t.Fatalf("GetInstance identity/root lookup failed: %v", err)
		}
		owner := "video"
		if i == 0 {
			owner = "payments"
		}
		if m.FreeformTags["Owner"] != owner {
			t.Fatal("stale ownership metadata")
		}
		m.FreeformTags["Owner"] = "caller mutation"
	}
	if calls.Load() != int32(len(ids)) {
		t.Fatal("metadata read cached or retried")
	}
}
