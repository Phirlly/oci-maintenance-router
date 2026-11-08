package configuration

import "testing"

func TestRejectInvalidOwnership(t *testing.T) {
	for _, value := range []any{
		"Owner", map[string]any{}, map[string]any{"kind": "other", "key": "Owner"},
		map[string]any{"kind": "defined", "key": "Owner"},
		map[string]any{"kind": "defined", "namespace": "", "key": "Owner"},
		map[string]any{"kind": "freeform", "key": ""},
		map[string]any{"kind": "freeform", "key": "Owner", "namespace": ""},
		map[string]any{"kind": "freeform", "key": "Owner", "namespace": nil},
		map[string]any{"kind": "freeform", "key": "Owner", "Key": "Other"},
	} {
		all := settings(t)
		all["ownership"] = value
		if _, err := Decode(encoded(t, all), catalog()); err == nil {
			t.Fatalf("accepted ownership %#v", value)
		}
	}
}

func TestRejectInvalidDestinations(t *testing.T) {
	for _, value := range []any{
		[]any{}, "payments", []any{nil},
		[]any{map[string]any{"id": "payments"}},
		[]any{map[string]any{"id": "Payments", "secretId": "ocid1.vaultsecret.oc1.iad.example"}},
		[]any{map[string]any{"id": "payments", "secretId": "ocid1.secret.oc1.iad.example"}},
		[]any{map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.phx.example"}},
		[]any{map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1..example"}},
		[]any{map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.iad.example", "key": "secret"}},
		[]any{
			map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.iad.example"},
			map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.iad.other"},
		},
	} {
		all := settings(t)
		all["destinations"] = value
		if _, err := Decode(encoded(t, all), catalog()); err == nil {
			t.Fatalf("accepted destinations %#v", value)
		}
	}
}

func TestRejectInvalidRoutes(t *testing.T) {
	for _, value := range []any{
		[]any{}, []any{nil}, true,
		[]any{map[string]any{"ownerValue": "", "destinationId": "payments"}},
		[]any{map[string]any{"ownerValue": "pay", "destinationId": "missing"}},
		[]any{map[string]any{"ownerValue": "pay", "destinationId": nil}},
		[]any{map[string]any{"ownerValue": "pay", "destinationId": "payments", "fallback": true}},
		[]any{
			map[string]any{"ownerValue": "pay", "destinationId": "payments"},
			map[string]any{"ownerValue": "pay", "destinationId": "payments"},
		},
	} {
		all := settings(t)
		all["routes"] = value
		if _, err := Decode(encoded(t, all), catalog()); err == nil {
			t.Fatalf("accepted routes %#v", value)
		}
	}
}

func TestCaseDistinctOwnersRemainDistinct(t *testing.T) {
	all := settings(t)
	all["routes"] = []any{
		map[string]any{"ownerValue": "pay", "destinationId": "payments"},
		map[string]any{"ownerValue": "Pay", "destinationId": "payments"},
	}
	got := decoded(t, all)
	if len(got.Routes) != 2 || got.Routes[1].OwnerValue != "Pay" {
		t.Fatal("owner case collapsed")
	}
}

func TestTopicAndPagerDutyPlacement(t *testing.T) {
	for field, values := range map[string][]any{
		"pagerDutyRegion": {"us", "AU", "", 3},
		"intakeTopicId":   {"ocid1.onstopic.oc1.phx.intake", "ocid1.onstopic.oc2.iad.intake", "ocid1.vaultsecret.oc1.iad.intake"},
		"operatorTopicId": {"ocid1.onstopic.oc1.iad.intake", "ocid1.onstopic.oc1.us-ashburn-1..intake", "ocid1.onstopic.oc1.phx.operator"},
	} {
		for _, value := range values {
			all := settings(t)
			all[field] = value
			if _, err := Decode(encoded(t, all), catalog()); err == nil {
				t.Fatalf("accepted %s=%v", field, value)
			}
		}
	}
}
