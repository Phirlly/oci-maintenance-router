package infra_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFormCoversActualTerraformInputs(t *testing.T) {
	form := readForm(t)
	files, err := filepath.Glob("variables_*.tf")
	if err != nil {
		t.Fatal(err)
	}
	declarations := regexp.MustCompile(`(?s)variable\s+"([^"]+)"\s*\{\s*type\s*=\s*([^\n]+)`)
	inputs := map[string]bool{}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range declarations.FindAllStringSubmatch(string(data), -1) {
			name, kind := match[1], strings.TrimSpace(match[2])
			inputs[name] = true
			field, ok := form.Variables[name]
			if !ok {
				t.Fatalf("missing form input %s", name)
			}
			multiselect := name == "resource_types" && field.Type == "enum" && field.AdditionalProps["allowMultiple"] == true
			if kind == "bool" && field.Type != "boolean" || kind == "map(string)" && field.Type != "map" || kind == "list(string)" && field.Type != "list" && !multiselect || kind == "number" && field.Type != "integer" {
				t.Fatalf("type mismatch for %s: %s/%s", name, kind, field.Type)
			}
		}
	}
	if len(inputs) != 34 {
		t.Fatalf("review new input contract: %d", len(inputs))
	}
	for name, field := range form.Variables {
		if !inputs[name] && field.Visible != false {
			t.Fatalf("unexpected visible helper %s", name)
		}
		for _, value := range field.DependsOn {
			ref, ok := value.(string)
			if !ok {
				continue
			}
			ref = strings.TrimSuffix(strings.TrimPrefix(ref, "${"), "}")
			if _, ok := form.Variables[ref]; !ok {
				t.Fatalf("unresolved dependency %s.%s", name, ref)
			}
		}
	}
}
