package configuration

import (
	"strings"
	"testing"
)

func preparationSettings(t *testing.T) map[string]any {
	t.Helper()
	v := settings(t)
	v["schemaVersion"] = 2
	v["mode"] = "PREVIEW"
	v["installationId"] = "installation-1"
	v["preparation"] = map[string]any{"initialize": true}
	return v
}

func TestPreparationControlsRequireExplicitVersionedIntent(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED"} {
		for _, initialize := range []bool{true, false} {
			v := preparationSettings(t)
			v["mode"] = mode
			v["preparation"] = map[string]any{"initialize": initialize}
			c := decoded(t, v)
			if c.SchemaVersion != 2 || c.Mode != mode || c.InstallationID != "installation-1" || c.Preparation.Initialize != initialize || c.ActiveBaselineID != "" || len(c.Replay) != 0 {
				t.Fatal("v2 controls lost or legacy authority populated")
			}
			v["installationId"] = "changed"
			v["preparation"].(map[string]any)["initialize"] = !initialize
			if c.InstallationID != "installation-1" || c.Preparation.Initialize != initialize {
				t.Fatal("configuration aliases caller controls")
			}
		}
	}
	legacy := decoded(t, settings(t))
	if legacy.InstallationID != "" || legacy.Preparation.Initialize {
		t.Fatal("v1 gained initialization intent")
	}
}

func TestPreparationControlsRejectMissingTypesAndLegacyFields(t *testing.T) {
	for _, field := range []string{"mode", "installationId", "preparation"} {
		v := preparationSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
		v = preparationSettings(t)
		v[field] = nil
		wantError(t, encoded(t, v), field, "type")
	}
	for _, field := range []string{"activeBaselineId", "replay"} {
		for _, value := range []any{nil, "", []any{}} {
			v := preparationSettings(t)
			v[field] = value
			wantError(t, encoded(t, v), "configuration", "unknown_field")
		}
	}
	for _, field := range []string{"installationId", "preparation"} {
		v := settings(t)
		v[field] = preparationSettings(t)[field]
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
	v := preparationSettings(t)
	v["preparation"] = map[string]any{}
	wantError(t, encoded(t, v), "preparation.initialize", "required")
	for _, value := range []any{nil, 1, "true", []any{}, map[string]any{}} {
		v := preparationSettings(t)
		v["preparation"] = map[string]any{"initialize": value}
		wantError(t, encoded(t, v), "preparation.initialize", "type")
	}
	for _, value := range []any{true, 1, "private", []any{}} {
		v := preparationSettings(t)
		v["preparation"] = value
		wantError(t, encoded(t, v), "preparation", "type")
	}
	for _, mode := range []any{"LIVE", "TEST", "preview", "", true} {
		v := preparationSettings(t)
		v["mode"] = mode
		if _, err := Decode(encoded(t, v), catalog()); err == nil {
			t.Fatal("unsupported mode admitted")
		}
	}
	for _, id := range []any{"", "../id", "a b", strings.Repeat("a", 129), true} {
		v := preparationSettings(t)
		v["installationId"] = id
		if _, err := Decode(encoded(t, v), catalog()); err == nil {
			t.Fatal("invalid installation admitted")
		}
	}
	v = preparationSettings(t)
	v["installationId"] = strings.Repeat("A", 128)
	if c := decoded(t, v); len(c.InstallationID) != 128 {
		t.Fatal("bounded valid ID rejected")
	}
}
