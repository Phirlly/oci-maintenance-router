package pagerduty

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// This tests the concrete transport/model contract, not coordinator authority.
func TestAcceptanceBuiltSavedDeliverySurvivesActualPagerDutyExchange(t *testing.T) {
	config, err := os.ReadFile("testdata/routing-config.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog := configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}
	builder, err := delivery.New(config, catalog, configuration.LedgerLocation{Namespace: "example", Bucket: "ledger"})
	if err != nil {
		t.Fatal(err)
	}
	a, b := "ocid1.mysqldbsystem.oc1.iad.payments", "ocid1.mysqldbsystem.oc1.iad.video"
	platform, compartment := "IAAS", "ocid1.compartment.oc1..example"
	n, err := notices.Normalize(notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: "SCHEDULED_MAINTENANCE", Platform: &platform, Services: []string{"Oracle HeatWave MySQL"}, Resources: []notices.Resource{{ResourceID: a}, {ResourceID: b}}})
	if err != nil {
		t.Fatal(err)
	}
	lookups := map[string]routing.Lookup{}
	for id, owner := range map[string]string{a: "payments", b: "video"} {
		lookups[id] = routing.Lookup{State: routing.Found, Metadata: routing.Metadata{ID: &id, CompartmentID: &compartment, DefinedTags: map[string]map[string]any{"Operations": {"Owner": owner}}}}
	}
	result, err := builder.Build(n, false, lookups, []string{a})
	if err != nil || len(result.Drafts) != 1 || len(result.Issues) != 0 {
		t.Fatal("draft construction", err)
	}
	r, err := delivery.NewRecord(delivery.Preparation{Draft: result.Drafts[0], SecretVersion: 7})
	if err != nil {
		t.Fatal(err)
	}
	before := r.Batches()[0]
	reload := func(record delivery.Record) delivery.Record {
		body, e := record.Encode()
		if e != nil {
			t.Fatal(e)
		}
		got, e := delivery.DecodeRecord(body, before.Message.EnvironmentID, n.Key().NoticeID)
		if e != nil {
			t.Fatal(e)
		}
		return got
	}
	r = reload(r)
	at := time.Now().UTC().Truncate(time.Millisecond)
	r, ref, err := r.Begin(before.Message.BatchID, delivery.AttemptContext{OwnerToken: "contract-worker", RoutingGeneration: before.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	r = reload(r)
	saved := r.Batches()[0]
	var calls atomic.Int32
	client, _ := tlsClient(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		body, e := io.ReadAll(req.Body)
		expected := append([]byte(`{"routing_key":"`+syntheticKey+`",`), saved.Message.PayloadWithoutRoutingKey[1:]...)
		if e != nil || !bytes.Equal(body, expected) || bytes.Contains(body, []byte(b)) {
			t.Error("transport changed saved payload or leaked another recipient")
		}
		var envelope struct {
			RoutingKey string `json:"routing_key"`
			DedupKey   string `json:"dedup_key"`
			Payload    struct {
				Details struct {
					Resources []struct {
						ResourceID string `json:"resourceId"`
					} `json:"resources"`
				} `json:"custom_details"`
			} `json:"payload"`
		}
		if json.Unmarshal(body, &envelope) != nil || envelope.RoutingKey != syntheticKey || envelope.DedupKey != before.Message.DedupKey || len(envelope.Payload.Details.Resources) != 1 || envelope.Payload.Details.Resources[0].ResourceID != a {
			t.Error("incorrect wire identity/recipient")
		}
		w.WriteHeader(202)
	})
	response, err := client.Send(budget(t), saved.Message.PayloadWithoutRoutingKey, []byte(syntheticKey))
	if err != nil || response.Status != 202 || calls.Load() != 1 {
		t.Fatal("concrete transport rejected built payload", err)
	}
	r, err = r.Finish(ref, response.Status, at.Add(time.Second), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	after := reload(r).Batches()[0]
	encoded, _ := r.Encode()
	if after.State != delivery.APIAccepted || after.AttemptCount != 1 || after.SecretVersion != 7 || !reflect.DeepEqual(after.Message, before.Message) || bytes.Contains(encoded, []byte(syntheticKey)) {
		t.Fatal("result lost immutable identity/receipt or persisted credential")
	}
}
