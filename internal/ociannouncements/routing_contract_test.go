package ociannouncements

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/routing"
)

func TestAcceptanceSignedAnnouncementReachesRecipientSpecificProjections(t *testing.T) {
	config, err := os.ReadFile("testdata/routing-config.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog := configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}
	location := configuration.LedgerLocation{Namespace: "example", Bucket: "ledger"}
	policy, err := routing.New(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	observer, err := noticeledger.New(config, catalog, location)
	if err != nil {
		t.Fatal(err)
	}
	builder, err := delivery.New(config, catalog, location)
	if err != nil {
		t.Fatal(err)
	}
	a, b := "ocid1.mysqldbsystem.oc1.iad.example", "ocid1.mysqldbsystem.oc1.iad.video"
	compartment := "ocid1.compartment.oc1..example"
	lookups := map[string]routing.Lookup{}
	for id, owner := range map[string]string{a: "payments", b: "video"} {
		lookups[id] = routing.Lookup{State: routing.Found, Metadata: routing.Metadata{ID: &id, CompartmentID: &compartment, DefinedTags: map[string]map[string]any{"Operations": {"Owner": owner}}}}
	}
	for _, publication := range []bool{false, true} {
		t.Run(map[bool]string{false: "without-publication", true: "with-publication"}[publication], func(t *testing.T) {
			var wire map[string]any
			if err := json.Unmarshal([]byte(detailed), &wire); err != nil {
				t.Fatal(err)
			}
			wire["description"] = "private-description-marker"
			wire["affectedResources"] = append(wire["affectedResources"].([]any), map[string]any{"resourceId": b, "additionalProperties": []any{map[string]any{"name": "private-property-marker", "value": "private-value-marker"}}})
			if !publication {
				delete(wire, "timeCreated")
				delete(wire, "timeUpdated")
			}
			body, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			key := generatedKey(t)
			client, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != "GET" || req.URL.RequestURI() != "/20180904/announcements/"+noticeID {
					t.Error("unexpected source request")
				}
				verifySignature(t, req, key)
				_, _ = w.Write(body)
			})
			n, err := client.Get(budget(t), noticeID)
			if err != nil {
				t.Fatal(err)
			}
			plan := policy.Resolve(n, false, lookups)
			if plan.Assessment.State != routing.SupportedShape || len(plan.Groups) != 2 {
				t.Fatal("actual wire notice did not produce two recipients", plan)
			}
			record, err := observer.Observe(noticeledger.Record{}, n, false, lookups, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := record.Encode()
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range []string{"private-description-marker", "private-property-marker", "private-value-marker"} {
				if bytes.Contains(encoded, []byte(raw)) {
					t.Fatal("unapproved raw source field persisted")
				}
			}
			drafts, err := builder.Build(n, false, lookups, []string{a, b})
			if err != nil || len(drafts.Drafts) != 2 || len(drafts.Issues) != 0 {
				t.Fatal("wire notice not buildable", err)
			}
			for _, draft := range drafts.Drafts {
				saved := draft.Snapshot()
				own, other := a, b
				if saved.DestinationID == "video" {
					own, other = b, a
				} else if saved.DestinationID != "payments" {
					t.Fatal("unexpected destination")
				}
				payload := string(saved.PayloadWithoutRoutingKey)
				if len(saved.Resources) != 1 || saved.Resources[0].ResourceID != own || strings.Contains(payload, other) || strings.Contains(payload, "private-") {
					t.Fatal("recipient projection leaked fields or another resource")
				}
				var event struct {
					Payload struct {
						Timestamp *string `json:"timestamp"`
						Details   struct {
							Reference string                                `json:"reference"`
							Times     []struct{ Type, Title, Value string } `json:"times"`
						} `json:"custom_details"`
					} `json:"payload"`
				}
				if json.Unmarshal(saved.PayloadWithoutRoutingKey, &event) != nil {
					t.Fatal("invalid payload")
				}
				details := event.Payload.Details
				if details.Reference != "EXAMPLE-001" || len(details.Times) != 2 || details.Times[0].Type != "START_TIME" || details.Times[0].Title != "Maintenance week starts" || details.Times[0].Value != "2026-01-16T00:00:00.000Z" || details.Times[1].Type != "END_TIME" || details.Times[1].Title != "Maintenance week ends" || details.Times[1].Value != "2026-01-22T23:59:59.000Z" {
					t.Fatal("semantic maintenance window changed", details)
				}
				if publication && (event.Payload.Timestamp == nil || *event.Payload.Timestamp != "2026-01-01T10:00:00.123Z") || !publication && event.Payload.Timestamp != nil {
					t.Fatal("optional publication context changed")
				}
			}
		})
	}
}
