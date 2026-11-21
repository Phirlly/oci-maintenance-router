package ociownership

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestComputeProjectionPreservesTagsAndExcludesInstanceUserData(t *testing.T) {
	key := generatedKey(t)
	for _, location := range []string{compartment, tenancy} {
		c, err := NewCompute("us-ashburn-1", "iad", tenancy, syntheticKey{key: key})
		if err != nil {
			t.Fatal(err)
		}
		c, _ = bindTLSClient(t, c, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"id": instanceID, "compartmentId": location,
				"definedTags":  map[string]any{"Operations": map[string]any{"Owner": "payments", "owner": "video", "number": 42, "null": nil}},
				"freeformTags": map[string]string{"Owner": "payments", "owner": "video", "spaced": " unchanged "},
				"displayName":  "synthetic-private-display", "metadata": map[string]string{"user_data": "synthetic-private-user-data"},
				"extendedMetadata": map[string]any{"nested": []any{"synthetic-private-extra", true}}, "lifecycleState": "FUTURE_STATE",
			})
		})
		m, err := c.Get(budget(t), instanceID)
		id := instanceID
		want := routing.Metadata{ID: &id, CompartmentID: &location,
			DefinedTags:  map[string]map[string]any{"Operations": {"Owner": "payments", "owner": "video", "number": json.Number("42"), "null": nil}},
			FreeformTags: map[string]string{"Owner": "payments", "owner": "video", "spaced": " unchanged "}}
		if err != nil || !reflect.DeepEqual(m, want) {
			t.Fatalf("unexpected ownership projection: %v", err)
		}
		encoded, err := json.Marshal(m)
		if err != nil || strings.Contains(string(encoded), "synthetic-private") {
			t.Fatal("unrelated instance fields escaped")
		}
	}
}

func TestComputeMalformedMetadataReturnsOnlyControlledFailure(t *testing.T) {
	key := generatedKey(t)
	for _, change := range []string{"id", "compartmentId", "freeformTags", "duplicate metadata"} {
		c, err := NewCompute("us-ashburn-1", "iad", tenancy, syntheticKey{key: key})
		if err != nil {
			t.Fatal(err)
		}
		c, _ = bindTLSClient(t, c, func(w http.ResponseWriter, r *http.Request) {
			body := map[string]any{"id": instanceID, "compartmentId": compartment, "metadata": map[string]string{"user_data": "synthetic-private-user-data"}}
			switch change {
			case "id":
				body["id"] = dbID
			case "compartmentId":
				body["compartmentId"] = "ocid1.tenancy.oc1..foreign"
			case "freeformTags":
				body["freeformTags"] = map[string]any{"Owner": 42}
			case "duplicate metadata":
				io.WriteString(w, `{"id":"`+instanceID+`","compartmentId":"`+compartment+`","metadata":{"user_data":"synthetic-private-a","user_data":"synthetic-private-b"}}`)
				return
			}
			json.NewEncoder(w).Encode(body)
		})
		m, err := c.Get(budget(t), instanceID)
		wantError(t, err, InvalidResponse, 200)
		wantEmpty(t, m)
	}
}
