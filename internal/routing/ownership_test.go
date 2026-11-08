package routing_test

import (
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestDefinedOwnershipSelection(t *testing.T) {
	p, n := policy(t, "defined"), notice(t, record(db))
	for _, tc := range []struct {
		name   string
		tags   map[string]map[string]any
		reason routing.Reason
	}{
		{"exact", map[string]map[string]any{"Operations": {"Owner": "payments"}}, ""},
		{"ascii case insensitive", map[string]map[string]any{"oPERATIONS": {"oWNER": "payments"}}, ""},
		{"other tags irrelevant", map[string]map[string]any{"Other": {"Owner": "video"}, "Operations": {"Other": "video", "Owner": "payments"}}, ""},
		{"no tags", nil, routing.ReasonTagMissing},
		{"missing namespace", map[string]map[string]any{"Other": {"Owner": "payments"}}, routing.ReasonTagMissing},
		{"missing key", map[string]map[string]any{"Operations": nil}, routing.ReasonTagMissing},
		{"no namespace whitespace normalization", map[string]map[string]any{"Operations ": {"Owner": "payments"}}, routing.ReasonTagMissing},
		{"no key whitespace normalization", map[string]map[string]any{"Operations": {"Owner ": "payments"}}, routing.ReasonTagMissing},
		{"namespace collision equal values", map[string]map[string]any{"Operations": {"Owner": "payments"}, "operations": {"Owner": "payments"}}, routing.ReasonTagAmbiguous},
		{"key collision conflicting values", map[string]map[string]any{"Operations": {"Owner": "payments", "owner": "video"}}, routing.ReasonTagAmbiguous},
		{"key collision equal values", map[string]map[string]any{"Operations": {"Owner": "payments", "owner": "payments"}}, routing.ReasonTagAmbiguous},
		{"null", map[string]map[string]any{"Operations": {"Owner": nil}}, routing.ReasonTagInvalid},
		{"number", map[string]map[string]any{"Operations": {"Owner": 42}}, routing.ReasonTagInvalid},
		{"list", map[string]map[string]any{"Operations": {"Owner": []string{"payments"}}}, routing.ReasonTagInvalid},
		{"empty", map[string]map[string]any{"Operations": {"Owner": ""}}, routing.ReasonTagInvalid},
		{"invalid utf8", map[string]map[string]any{"Operations": {"Owner": "\xff"}}, routing.ReasonTagInvalid},
		{"unknown owner", map[string]map[string]any{"Operations": {"Owner": "unknown"}}, routing.ReasonOwnerUnmapped},
		{"no value case folding", map[string]map[string]any{"Operations": {"Owner": "Payments"}}, routing.ReasonOwnerUnmapped},
		{"no value trimming", map[string]map[string]any{"Operations": {"Owner": " payments "}}, routing.ReasonOwnerUnmapped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := metadata(db, "video") // A different freeform owner must never be a fallback.
			lookup.Metadata.DefinedTags = tc.tags
			got := p.Resolve(n, false, map[string]routing.Lookup{db: lookup})
			assertOwnership(t, got, tc.reason)
		})
	}
}

func TestFreeformSelectionIsExact(t *testing.T) {
	p, n := policy(t, "freeform"), notice(t, record(db))
	for _, tc := range []struct {
		tags   map[string]string
		reason routing.Reason
	}{
		{map[string]string{"Owner": "payments", "owner": "video"}, ""},
		{map[string]string{"owner": "payments"}, routing.ReasonTagMissing},
		{nil, routing.ReasonTagMissing},
		{map[string]string{"Owner": ""}, routing.ReasonTagInvalid},
		{map[string]string{"Owner": "\xff"}, routing.ReasonTagInvalid},
		{map[string]string{"Owner": "Payments"}, routing.ReasonOwnerUnmapped},
		{map[string]string{"Owner": " payments "}, routing.ReasonOwnerUnmapped},
	} {
		lookup := metadata(db, "video")
		lookup.Metadata.FreeformTags = tc.tags
		assertOwnership(t, p.Resolve(n, false, map[string]routing.Lookup{db: lookup}), tc.reason)
	}
}

func TestOwnershipMetadataBounds(t *testing.T) {
	n := notice(t, record(db))
	for _, kind := range []string{"defined", "freeform"} {
		invalidLarge := metadata(db, strings.Repeat("\xff", (1<<20)+1))
		assertOwnership(t, policy(t, kind).Resolve(n, false, map[string]routing.Lookup{db: invalidLarge}), routing.ReasonMetadataLimit)
		lookup := metadata(db, strings.Repeat("x", (1<<20)+1))
		assertOwnership(t, policy(t, kind).Resolve(n, false, map[string]routing.Lookup{db: lookup}), routing.ReasonMetadataLimit)
		lookup = metadata(db, strings.Repeat("x", 1<<20))
		assertOwnership(t, policy(t, kind).Resolve(n, false, map[string]routing.Lookup{db: lookup}), routing.ReasonOwnerUnmapped)
	}
	for _, namespaceLimit := range []bool{true, false} {
		lookup := metadata(db, "payments")
		for i := 0; i < 255; i++ {
			if namespaceLimit {
				lookup.Metadata.DefinedTags[fmt.Sprint(i)] = nil
			} else {
				lookup.Metadata.DefinedTags["Operations"][fmt.Sprint(i)] = nil
			}
		}
		p := policy(t, "defined")
		assertOwnership(t, p.Resolve(n, false, map[string]routing.Lookup{db: lookup}), "")
		if namespaceLimit {
			lookup.Metadata.DefinedTags["overflow"] = nil
		} else {
			lookup.Metadata.DefinedTags["Operations"]["overflow"] = nil
		}
		assertOwnership(t, p.Resolve(n, false, map[string]routing.Lookup{db: lookup}), routing.ReasonMetadataLimit)
	}
}

func assertOwnership(t *testing.T, plan routing.Plan, reason routing.Reason) {
	t.Helper()
	if len(plan.Decisions) != 1 {
		t.Fatalf("missing ownership decision: %+v", plan)
	}
	d := plan.Decisions[0]
	if reason == "" {
		if d.Disposition != routing.Ready || d.DestinationID != "payments" || d.Reason != "" || len(plan.Groups) != 1 || plan.Groups[0].DestinationID != "payments" {
			t.Fatalf("wrong owner: %+v", plan)
		}
	} else if d.Disposition != routing.Unresolved || d.Reason != reason || d.DestinationID != "" || len(plan.Groups) != 0 {
		t.Fatalf("unresolved ownership routed: %+v, want %s", plan, reason)
	}
}
