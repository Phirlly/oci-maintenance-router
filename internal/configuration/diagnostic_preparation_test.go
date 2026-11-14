package configuration

import (
	"strings"
	"testing"
)

func TestDiagnosticSchemaPreservesPreparationControlValidation(t *testing.T) {
	for _, initialize := range []bool{false, true} {
		v := diagnosticSettings(t)
		v["preparation"] = map[string]any{"initialize": initialize}
		if decoded(t, v).Preparation.Initialize != initialize {
			t.Fatal("preparation flag changed")
		}
	}
	for _, field := range []string{"mode", "installationId", "preparation"} {
		v := diagnosticSettings(t)
		v[field] = nil
		wantError(t, encoded(t, v), field, "type")
	}
	for _, id := range []string{"", "../other", "not safe", strings.Repeat("a", 129)} {
		v := diagnosticSettings(t)
		v["installationId"] = id
		wantError(t, encoded(t, v), "installationId", "invalid")
	}
	v := diagnosticSettings(t)
	v["preparation"] = map[string]any{}
	wantError(t, encoded(t, v), "preparation.initialize", "required")
	for _, value := range []any{nil, "true", 1} {
		v := diagnosticSettings(t)
		v["preparation"] = map[string]any{"initialize": value}
		wantError(t, encoded(t, v), "preparation.initialize", "type")
	}
	for _, v := range []map[string]any{settings(t), preparationSettings(t)} {
		v["mode"] = "TEST"
		wantError(t, encoded(t, v), "mode", "invalid")
	}
}
