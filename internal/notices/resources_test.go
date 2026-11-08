package notices

import (
	"reflect"
	"slices"
	"testing"
)

func TestNormalizeResourceSets(t *testing.T) {
	input := sample()
	input.Services = []string{"Z", "A", "A", "a"}
	other := sample().Resources[0]
	other.ResourceID = "another-unresolved-id"
	duplicate := sample().Resources[0]
	slices.Reverse(duplicate.AdditionalProperties)
	input.Resources = append(input.Resources, other, duplicate)
	got := normalized(t, input).Snapshot()
	if !reflect.DeepEqual(got.Services, []string{"A", "Z", "a"}) || len(got.Resources) != 2 ||
		got.Resources[0].ResourceID != other.ResourceID || got.Resources[1].ResourceID != input.Resources[0].ResourceID {
		t.Fatal("sets were not sorted/deduplicated")
	}
	if len(input.Resources) != 3 || input.Services[0] != "Z" {
		t.Fatal("source set mutated")
	}
}

func TestConflictingResourceDuplicatesRequireReview(t *testing.T) {
	for _, change := range []func(*Resource){
		func(r *Resource) { r.ResourceName = text("different") },
		func(r *Resource) { r.ResourceName = nil },
		func(r *Resource) { r.RegionDisplay = text("different") },
		func(r *Resource) { r.AdditionalProperties[0].Value = text("different") },
		func(r *Resource) { r.AdditionalProperties = nil },
	} {
		input := sample()
		other := sample().Resources[0]
		change(&other)
		input.Resources = append(input.Resources, other)
		wantError(t, input, "resources", "conflicting_duplicate")
	}
}

func TestPropertiesSortAndRetainMultiplicity(t *testing.T) {
	input := sample()
	input.Resources[0].AdditionalProperties = []Property{
		{text("b"), text("1")}, {text("a"), text("2")}, {text("a"), text("1")}, {text("a"), text("1")},
	}
	got := normalized(t, input).Snapshot().Resources[0].AdditionalProperties
	want := []Property{{text("a"), text("1")}, {text("a"), text("1")}, {text("a"), text("2")}, {text("b"), text("1")}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("properties changed: %+v", got)
	}
}
