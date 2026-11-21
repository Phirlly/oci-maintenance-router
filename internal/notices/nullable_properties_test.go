package notices

import (
	"reflect"
	"slices"
	"testing"
)

func TestNullPropertyPreservedAcrossCopiesAndNormalization(t *testing.T) {
	input := sample()
	input.Resources[0].AdditionalProperties = []Property{{text("description"), nil}}
	n := normalized(t, input)
	snapshot := n.Snapshot()
	if !reflect.DeepEqual(snapshot.Resources, input.Resources) || normalized(t, snapshot).Key() != n.Key() {
		t.Fatal("nullable property or revision changed")
	}
	*input.Resources[0].AdditionalProperties[0].Name = "changed input"
	*snapshot.Resources[0].AdditionalProperties[0].Name = "changed snapshot"
	snapshot.Resources[0].AdditionalProperties[0].Value = text("")
	got := n.Snapshot().Resources[0].AdditionalProperties
	if len(got) != 1 || *got[0].Name != "description" || got[0].Value != nil {
		t.Fatal("snapshot aliases mutable input or loses null")
	}
}

func TestNullPropertyOrderingPreservesMultiplicityAndIdentity(t *testing.T) {
	want := []Property{
		{text("a"), nil}, {text("a"), nil}, {text("a"), text("")},
		{text("a"), text("z")}, {text("b"), nil}, {text("b"), text("a")},
	}
	var key RevisionKey
	for _, reverse := range []bool{false, true} {
		input := sample()
		input.Resources[0].AdditionalProperties = slices.Clone(want)
		if reverse {
			slices.Reverse(input.Resources[0].AdditionalProperties)
		}
		n := normalized(t, input)
		if !reflect.DeepEqual(n.Snapshot().Resources[0].AdditionalProperties, want) {
			t.Fatal("null ordering or multiplicity changed")
		}
		if reverse && n.Key() != key {
			t.Fatal("property order changed revision identity")
		}
		key = n.Key()
	}
}

func TestNullPropertyIsDistinctFromEmptyAndMissingProperty(t *testing.T) {
	input := sample()
	input.Resources[0].AdditionalProperties = []Property{{text("label"), nil}}
	null := normalized(t, input)
	empty := null.Snapshot()
	empty.Resources[0].AdditionalProperties[0].Value = text("")
	missing := null.Snapshot()
	missing.Resources[0].AdditionalProperties = nil
	if null.Key() == normalized(t, empty).Key() || null.Key() == normalized(t, missing).Key() {
		t.Fatal("null was coerced to empty or removed")
	}
	input.Resources = append(input.Resources, empty.Resources[0])
	wantError(t, input, "resources", "conflicting_duplicate")
	input.Resources[1] = null.Snapshot().Resources[0]
	if len(normalized(t, input).Snapshot().Resources) != 1 {
		t.Fatal("identical nullable resources were not deduplicated")
	}
}
