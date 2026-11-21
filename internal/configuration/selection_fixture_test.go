package configuration

import (
	"encoding/json"
	"os"
	"testing"
)

func selectionFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func selectionSettings(t *testing.T) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(selectionFixture(t, "selected"), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func selectionCatalog(t *testing.T) Catalog {
	t.Helper()
	c, err := DecodeCatalog(selectionFixture(t, "selected-catalog"))
	if err != nil {
		t.Fatal("approved selected-resource catalog rejected", err)
	}
	return c
}

func selectedConfig(t *testing.T, v map[string]any) Config {
	t.Helper()
	c, err := Decode(encoded(t, v), selectionCatalog(t))
	if err != nil {
		t.Fatal("valid resource selection rejected", err)
	}
	return c
}
