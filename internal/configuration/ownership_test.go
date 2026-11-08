package configuration

import (
	"strings"
	"testing"
)

func TestOwnershipSelectorNames(t *testing.T) {
	selectors := []struct{ kind, field string }{
		{"defined", "namespace"}, {"defined", "key"}, {"freeform", "key"},
	}
	for _, selector := range selectors {
		t.Run(selector.kind+"/"+selector.field, func(t *testing.T) {
			for _, name := range []string{"", "two words", "a.b", " leading", "trailing ", "a\t", "a\n", "a\x7f", "日本", strings.Repeat("a", 101)} {
				all := settings(t)
				owner := map[string]any{"kind": selector.kind, "key": "Owner"}
				if selector.kind == "defined" {
					owner["namespace"] = "Operations"
				}
				owner[selector.field] = name
				all["ownership"] = owner
				wantError(t, encoded(t, all), "ownership", "invalid")
			}
			for _, name := range []string{"Owner", strings.Repeat("x", 100), "!\"#$%&'()*+,-/:;<=>?@[\\]^_`{|}~"} {
				all := settings(t)
				owner := map[string]any{"kind": selector.kind, "key": "Owner"}
				if selector.kind == "defined" {
					owner["namespace"] = "Operations"
				}
				owner[selector.field] = name
				all["ownership"] = owner
				got := decoded(t, all)
				if selector.field == "namespace" && got.Ownership.Namespace != name || selector.field == "key" && got.Ownership.Key != name {
					t.Fatal("selector bytes changed")
				}
			}
		})
	}
}
