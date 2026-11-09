package ocimysql

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSignedUnconditionalRequestAndFreshResult(t *testing.T) {
	key := generatedKey(t)
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
		number := calls.Add(1)
		verifySignature(t, r, key)
		if r.Method != "GET" || r.URL.Path != "/20190415/dbSystems/"+dbID || r.URL.RawQuery != "" || r.Header.Get("Accept") != "application/json" || !r.Close || r.ProtoMajor != 1 {
			t.Error("incorrect metadata request")
		}
		for _, h := range []string{"If-None-Match", "If-Match", "If-Modified-Since"} {
			if r.Header.Get(h) != "" {
				t.Error("conditional lookup enabled")
			}
		}
		w.Header().Set("ETag", "same-etag")
		body := strings.TrimSuffix(minimal, "}") + fmt.Sprintf(`,"freeformTags":{"Owner":"team-%d"}}`, number)
		fmt.Fprint(w, body)
	})
	for _, owner := range []string{"team-1", "team-2"} {
		m, err := c.Get(budget(t), dbID)
		if err != nil || m.ID == nil || *m.ID != dbID || m.FreeformTags["Owner"] != owner {
			t.Fatalf("metadata stale/invalid: %v", err)
		}
		m.FreeformTags["Owner"] = "caller change"
	}
	if calls.Load() != 2 {
		t.Fatal("read was cached or retried")
	}
}

func TestSafeRegionalIDVariantsRetainExactIdentity(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/20190415/dbSystems/")
		json.NewEncoder(w).Encode(map[string]any{"id": id, "compartmentId": tenancy})
	})
	for _, id := range []string{"ocid1.mysqldbsystem.oc1.iad.example", "ocid1.mysqldbsystem.oc1.us-ashburn-1..example", "ocid1.mysqldbsystem.oc1.iad.future.example-A_1"} {
		m, err := c.Get(budget(t), id)
		if err != nil || m.ID == nil || *m.ID != id || *m.CompartmentID != tenancy {
			t.Fatalf("valid shape rejected: %v", err)
		}
	}
}
