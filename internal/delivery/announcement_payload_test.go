package delivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestAnnouncementProjectionUsesActualCategoryAndClosedFields(t *testing.T) {
	for _, tc := range []struct {
		category string
		ids      []string
	}{
		{"ACTION_REQUIRED", []string{announcementDB}},
		{"SCHEDULED_MAINTENANCE", []string{announcementInstance}},
		{"PRODUCTION_EVENT_NOTIFICATION", []string{announcementInstance}},
		{"FUTURE_CATEGORY", []string{announcementDB, announcementInstance}},
	} {
		t.Run(tc.category, func(t *testing.T) {
			b, r, plan, groups := announcementFixture(t, tc.category, "reference<>&", tc.ids...)
			private := "private-source"
			r.Summary, r.Description, r.ChainID = &private, &private, &private
			created, updated := "2026-01-01T00:00:00.000Z", "2026-01-02T00:00:00.000Z"
			r.TimeCreated, r.TimeUpdated = &created, &updated
			result := b.pack(r, plan, groups)
			if len(result.Drafts) != 1 {
				t.Fatal(result)
			}
			s := result.Drafts[0].Snapshot()
			var resources []map[string]string
			for _, target := range groups[0].Resources {
				resources = append(resources, map[string]string{"resourceId": target.ResourceID, "region": target.Region})
			}
			want, _ := json.Marshal(map[string]any{
				"dedup_key": s.DedupKey, "event_action": "trigger",
				"payload": map[string]any{"summary": fmt.Sprintf("OCI announcement: %d resource(s)", len(tc.ids)), "source": "oci-maintenance-router/example", "severity": "info", "timestamp": updated,
					"custom_details": map[string]any{"formatVersion": 2, "announcementType": tc.category, "noticeId": plan.Revision.NoticeID, "revisionDigest": plan.Revision.Digest, "reference": "reference<>&", "resources": resources,
						"times": []map[string]any{{"slot": 1, "type": nil, "title": nil, "value": nil}, {"slot": 2, "type": nil, "title": nil, "value": nil}}}},
			})
			if !bytes.Equal(s.PayloadWithoutRoutingKey, want) {
				t.Fatalf("incorrect closed announcement projection: %s", s.PayloadWithoutRoutingKey)
			}
			if s.DedupKey != "omr1:"+s.BatchID || s.PayloadDigest != digest(want) {
				t.Fatal("identity/digest changed")
			}
			// Removing an updated timestamp retains the existing created fallback.
			r.TimeUpdated = nil
			if !bytes.Contains(b.pack(r, plan, groups).Drafts[0].Snapshot().PayloadWithoutRoutingKey, []byte(created)) {
				t.Fatal("created fallback lost")
			}
		})
	}
}

func TestAnnouncementPackingKeepsRecipientAndCountBoundaries(t *testing.T) {
	var ids []string
	for i := 0; i < 51; i++ {
		ids = append(ids, fmt.Sprintf("ocid1.instance.oc1.iad.%03d", i))
	}
	b, r, plan, groups := announcementFixture(t, "ACTION_REQUIRED", "sample", ids...)
	other := routing.Target{ResourceID: announcementDB, Region: "us-ashburn-1"}
	groups = append(groups, routing.Group{DestinationID: "video", Resources: []routing.Target{other}})
	result := b.pack(r, plan, groups)
	if len(result.Drafts) != 3 || len(result.Issues) != 0 {
		t.Fatal("wrong partition", result)
	}
	for i, want := range []int{50, 1, 1} {
		s := result.Drafts[i].Snapshot()
		if len(s.Resources) != want || len(s.PayloadWithoutRoutingKey) > 32768 {
			t.Fatal("bad bound")
		}
		if (i < 2 && bytes.Contains(s.PayloadWithoutRoutingKey, []byte(announcementDB))) || (i == 2 && bytes.Contains(s.PayloadWithoutRoutingKey, []byte("ocid1.instance"))) {
			t.Fatal("other recipient leaked")
		}
	}
}

func TestAnnouncementPackingRejectsLargeCategoryBeforePerResourceWork(t *testing.T) {
	var ids []string
	for i := 0; i < 256; i++ {
		ids = append(ids, fmt.Sprintf("ocid1.instance.oc1.iad.%03d", i))
	}
	b, r, plan, groups := announcementFixture(t, strings.Repeat("C", 500000), "", ids...)
	var result Result
	allocations := testing.AllocsPerRun(1, func() { result = b.pack(r, plan, groups) })
	if len(result.Drafts) != 0 || len(result.Issues) != 256 {
		t.Fatal("shared overflow did not reject all resources")
	}
	// A broad ceiling detects repeated serialization of the large shared category;
	// it intentionally does not pin individual standard-library allocation counts.
	if allocations > 2000 {
		t.Fatal("shared overflow encoded separately for each resource", allocations)
	}
}

func TestAnnouncementPackingExactByteBoundary(t *testing.T) {
	b, r, plan, groups := announcementFixture(t, "ACTION_REQUIRED", "", announcementInstance+"a", announcementInstance+"b")
	single := b.draft(r, plan, "payments", groups[0].Resources[:1]).Snapshot()
	padding := strings.Repeat("x", 32768-len(single.PayloadWithoutRoutingKey))
	r.Reference = &padding
	result := b.pack(r, plan, groups)
	if len(result.Drafts) != 2 || len(result.Issues) != 0 || len(result.Drafts[0].Snapshot().PayloadWithoutRoutingKey) != 32768 {
		t.Fatal("exact boundary/split failed", len(result.Drafts), len(result.Issues))
	}
	padding += "x"
	result = b.pack(r, plan, groups)
	if len(result.Drafts) != 0 || len(result.Issues) != 2 {
		t.Fatal("oversize member accepted")
	}
}

func TestLegacyPublicBuilderStillRejectsBroaderAnnouncements(t *testing.T) {
	for _, tc := range []struct{ category, id string }{{"ACTION_REQUIRED", announcementDB}, {"SCHEDULED_MAINTENANCE", announcementInstance}} {
		b, r, _, _ := announcementFixture(t, tc.category, "", tc.id)
		platform := "IAAS"
		r.Platform = &platform
		r.Services = []string{"Oracle HeatWave MySQL"}
		n, err := notices.Normalize(r)
		if err != nil {
			t.Fatal(err)
		}
		compartment := "ocid1.compartment.oc1..example"
		lookups := map[string]routing.Lookup{tc.id: {State: routing.Found, Metadata: routing.Metadata{ID: &tc.id, CompartmentID: &compartment, DefinedTags: map[string]map[string]any{"Operations": {"Owner": "payments"}}}}}
		result, err := b.Build(n, false, lookups, []string{tc.id})
		want := "notice: unsupported"
		if tc.id == announcementInstance {
			want = "selection: not_ready"
		}
		if err == nil || err.Error() != want || len(result.Drafts) != 0 {
			t.Fatal("prerequisite exposed broader routing")
		}
	}
}
