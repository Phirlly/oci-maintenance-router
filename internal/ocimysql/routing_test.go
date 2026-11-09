package ocimysql

import (
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestWireTagsRetainRoutingSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, kind, tags string
		reason           routing.Reason
	}{
		{"defined case", "defined", `"definedTags":{"oPERATIONS":{"oWNER":"payments"}}`, ""},
		{"namespace ambiguity", "defined", `"definedTags":{"Operations":{"Owner":"payments"},"operations":{"Owner":"payments"}}`, routing.ReasonTagAmbiguous},
		{"key ambiguity", "defined", `"definedTags":{"Operations":{"Owner":"payments","owner":"payments"}}`, routing.ReasonTagAmbiguous},
		{"no fallback", "defined", `"freeformTags":{"Owner":"payments"}`, routing.ReasonTagMissing},
		{"freeform exact", "freeform", `"freeformTags":{"Owner":"payments","owner":"video"}`, ""},
		{"freeform different case", "freeform", `"freeformTags":{"owner":"payments"},"definedTags":{"Operations":{"Owner":"payments"}}`, routing.ReasonTagMissing},
		{"value case", "freeform", `"freeformTags":{"Owner":"Payments"}`, routing.ReasonOwnerUnmapped},
		{"value whitespace", "freeform", `"freeformTags":{"Owner":" payments "}`, routing.ReasonOwnerUnmapped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := decodeMetadata([]byte(strings.TrimSuffix(minimal, "}")+","+tc.tags+"}"), dbID, tenancy)
			if err != nil {
				t.Fatal(err)
			}
			result := testPolicy(t, tc.kind, []string{compartment}).Resolve(testNotice(t, dbID), false, map[string]routing.Lookup{dbID: {State: routing.Found, Metadata: m}})
			if len(result.Decisions) != 1 || result.Decisions[0].Reason != tc.reason {
				t.Fatalf("wrong routing decision: %+v", result)
			}
			if tc.reason == "" {
				if len(result.Groups) != 1 || result.Groups[0].DestinationID != "payments" {
					t.Fatal("wrong recipient")
				}
			} else if len(result.Groups) != 0 || result.Decisions[0].Disposition != routing.Unresolved {
				t.Fatal("unresolved tags routed")
			}
		})
	}
}

func TestDefinedNonstringValuesCannotUseFreeformFallback(t *testing.T) {
	for _, value := range []string{"null", "true", "42", "1e999", `["payments"]`, `{"owner":"payments"}`, `""`} {
		body := strings.TrimSuffix(minimal, "}") + fmt.Sprintf(`,"definedTags":{"Operations":{"Owner":%s}},"freeformTags":{"Owner":"payments"}}`, value)
		m, err := decodeMetadata([]byte(body), dbID, tenancy)
		if err != nil {
			t.Fatal(err)
		}
		result := testPolicy(t, "defined", []string{compartment}).Resolve(testNotice(t, dbID), false, map[string]routing.Lookup{dbID: {State: routing.Found, Metadata: m}})
		if len(result.Groups) != 0 || len(result.Decisions) != 1 || result.Decisions[0].Reason != routing.ReasonTagInvalid {
			t.Fatal("nonstring tag was coerced or fell back")
		}
	}
}

func TestReplacementDatabaseAndExactCompartmentScope(t *testing.T) {
	p := testPolicy(t, "freeform", []string{tenancy})
	for _, id := range []string{dbID, "ocid1.mysqldbsystem.oc1.iad.replacement"} {
		for _, location := range []string{tenancy, compartment} {
			body := fmt.Sprintf(`{"id":%q,"compartmentId":%q,"freeformTags":{"Owner":"payments"}}`, id, location)
			m, err := decodeMetadata([]byte(body), id, tenancy)
			if err != nil {
				t.Fatal(err)
			}
			result := p.Resolve(testNotice(t, id), false, map[string]routing.Lookup{id: {State: routing.Found, Metadata: m}})
			if location == tenancy {
				if len(result.Groups) != 1 || result.Groups[0].DestinationID != "payments" || result.Groups[0].Resources[0].ResourceID != id {
					t.Fatal("replacement/root ownership lost")
				}
			} else if len(result.Groups) != 0 || result.Decisions[0].Disposition != routing.OutOfScope {
				t.Fatal("root selection included child")
			}
		}
	}
}
