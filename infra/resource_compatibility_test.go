package infra_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestLegacyRendererByteCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings map[string]any
		want     string
	}{
		{"legacy-preview", settingsFixture(), "6cad76dc4de228b3ffc196d74f34f7e4b5841e06da4b9c851eb3ec3b672163ab"},
		{"saved-preview", setupSettings("PREVIEW", "NONE", true), "7f7dcae5f269370c166cd05800d78e26b7289074a8a2a825bbf2bfe159744c97"},
		{"paired-send", setupSettings("TEST", "SEND_TEST", true), "979402659bf0e4a70386ede27b026578594ac5a5117073f2f8502e39ac2db97b"},
		{"paired-confirm", setupSettings("TEST", "CONFIRM", true), "3d2289fbc9cbce30b1d0b3d79506bbd5fe526ecad68839da2327643e9b363e95"},
		{"diagnostic-send", setupSettings("TEST", "SEND_TEST", false), "6d5bddcfab4ff38e26270a0635cead18d722b4620205052aa12afeaf3a56aae7"},
		{"diagnostic-confirm", setupSettings("TEST", "CONFIRM", false), "dfa60be92d14d39598fd50e3e1b1068bd2a04cc36a75f2d3e40954ebebef7383"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values, message, err := renderSetup(t, tc.settings)
			if err != nil {
				t.Fatalf("render: %s (%v)", message, err)
			}
			data, err := json.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			got := hex.EncodeToString(sum[:])
			if got != tc.want {
				t.Errorf("baseline output digest = %s, want %s", got, tc.want)
			}
			// Explicit null is the same legacy omission, even with an enriched input catalog.
			tc.settings["resource_types"] = nil
			enriched, message, err := renderSelected(t, tc.settings)
			if err != nil {
				t.Fatalf("enriched legacy render: %s (%v)", message, err)
			}
			data, err = json.Marshal(enriched)
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(data) != sum {
				t.Fatal("null selection or optional catalog fields changed legacy bytes")
			}
		})
	}
}
