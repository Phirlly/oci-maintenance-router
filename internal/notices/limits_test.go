package notices

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestNoticeInputByteBoundary(t *testing.T) {
	input := minimal()
	fixture, err := os.ReadFile("testdata/minimal-input.json")
	if err != nil {
		t.Fatal(err)
	}
	base := bytes.TrimSuffix(fixture, []byte("\n"))
	input.Description = text(strings.Repeat("x", (1<<20)-len(base)))
	normalized(t, input)
	*input.Description += "x"
	wantError(t, input, "notice", "too_large")
	for _, fill := range []string{strings.Repeat("x", (1<<20)+1), strings.Repeat("<", 200000)} {
		input = minimal()
		input.LifecycleState = text(fill) // Excluded from digest, still bounded input.
		wantError(t, input, "notice", "too_large")
	}
}

func TestNormalizedSnapshotByteBoundary(t *testing.T) {
	fixture, err := os.ReadFile("testdata/minimal-input.json")
	if err != nil {
		t.Fatal(err)
	}
	input := minimal()
	input.TimeCreated = text("2026-01-01T00:00:00Z")
	// Replace null with a quoted date; UTC millisecond formatting adds four bytes.
	dateGrowth := len(*input.TimeCreated) + 2 - len("null")
	padding := (1 << 20) - len(bytes.TrimSuffix(fixture, []byte("\n"))) - dateGrowth - 4
	input.Description = text(strings.Repeat("x", padding))
	got := normalized(t, input)
	again := normalized(t, got.Snapshot())
	if got.Key() != again.Key() {
		t.Fatal("exact-limit snapshot changed revision on repeated normalization")
	}
	*input.Description += "x"
	wantError(t, input, "notice", "too_large")
}

func TestDuplicatesCountTowardInputLimit(t *testing.T) {
	input := minimal()
	input.Services = make([]string, (1<<20)+1)
	wantError(t, input, "notice", "too_large")
	input = minimal()
	input.Resources = make([]Resource, (1<<20)+1)
	wantError(t, input, "notice", "too_large")
	input = minimal()
	input.Resources = []Resource{{ResourceID: "example", AdditionalProperties: make([]Property, (1<<20)+1)}}
	wantError(t, input, "notice", "too_large")
	input = minimal()
	input.Services = make([]string, 2000)
	for i := range input.Services {
		input.Services[i] = strings.Repeat("a", 600)
	}
	wantError(t, input, "notice", "too_large")
}

func TestCollectionOverheadIsBoundedBeforeInspectingEntries(t *testing.T) {
	input := minimal()
	input.Resources = make([]Resource, 50000)
	wantError(t, input, "notice", "too_large")
	input = minimal()
	input.Resources = []Resource{{ResourceID: "example", AdditionalProperties: make([]Property, 100000)}}
	wantError(t, input, "notice", "too_large")
}

func TestUniqueResourceBoundary(t *testing.T) {
	input := minimal()
	for i := 0; i < 256; i++ {
		input.Resources = append(input.Resources, Resource{ResourceID: fmt.Sprintf("resource-%03d", i)})
	}
	input.Resources = append(input.Resources, input.Resources[0])
	got := normalized(t, input)
	if len(got.Snapshot().Resources) != 256 {
		t.Fatal("duplicate consumed a unique-resource slot")
	}
	input.Resources = append(input.Resources, Resource{ResourceID: "resource-256"})
	wantError(t, input, "resources", "too_many")
}
