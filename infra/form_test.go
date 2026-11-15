package infra_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type formVariable struct {
	Description string
	Type        string
	ValueType   string
	Visible     any
	DependsOn   map[string]any
	Required    bool
	Enum        []string
	Default     any
	Minimum     int
	Maximum     int
	Pattern     string
	MaxLength   int
}
type stackForm struct {
	SchemaVersion  string
	Variables      map[string]formVariable
	VariableGroups []struct {
		Title     string
		Variables []string
	}
}

func readForm(t *testing.T) stackForm {
	t.Helper()
	requireTerraform(t)
	path, err := filepath.Abs("schema.yaml")
	if err != nil {
		t.Fatal(err)
	}
	output, err := terraform(t, t.TempDir(), fmt.Sprintf("jsonencode(yamldecode(file(%q)))\n", path), "console", "-no-color")
	var inner string
	var form stackForm
	if err != nil || json.Unmarshal(output, &inner) != nil || json.Unmarshal([]byte(inner), &form) != nil {
		t.Fatalf("form decode: %v %s", err, output)
	}
	return form
}

func TestNativeCollectionValuesAreRequired(t *testing.T) {
	form := readForm(t)
	for _, name := range []string{"mysql_regions", "mysql_compartments", "destinations", "routes"} {
		field := form.Variables[name]
		if !form.Variables[field.ValueType].Required {
			t.Errorf("%s entries must require a value; a required collection does not require its children", name)
		}
	}
}

func TestNativeFormContracts(t *testing.T) {
	form := readForm(t)
	if form.SchemaVersion != "1.1.0" || len(form.VariableGroups) == 0 {
		t.Fatal("missing grouped native RM form")
	}
	for name, kind := range map[string]string{"destinations": "map", "routes": "map", "mysql_regions": "list", "mysql_compartments": "list"} {
		field, ok := form.Variables[name]
		helper, exists := form.Variables[field.ValueType]
		if !ok || field.Type != kind || !field.Required || !exists || helper.Visible != false {
			t.Fatalf("invalid native collection %s", name)
		}
	}
	secret := form.Variables[form.Variables["destinations"].ValueType]
	if secret.Type != "oci:kms:secret:id" || secret.DependsOn["compartmentId"] != "${secret_compartment_ocid}" {
		t.Fatal("missing scoped Vault selector")
	}
	subnet := form.Variables["existing_subnet_ocid"]
	if subnet.DependsOn["vcnId"] != "${existing_vcn_ocid}" || subnet.DependsOn["hidePublicSubnet"] != true || subnet.DependsOn["hideAdSubnet"] != true {
		t.Fatal("subnet picker lacks required VCN dependency/private regional filtering")
	}
	for _, name := range []string{"tenancy_ocid", "region"} {
		if form.Variables[name].Visible != false {
			t.Fatalf("system field %s exposed", name)
		}
	}
	for _, name := range []string{"active_baseline_id", "replay", "archive", "runtime", "release_sha256"} {
		if _, ok := form.Variables[name]; ok {
			t.Fatalf("internal/activation field %s exposed", name)
		}
	}
	grouped := map[string]bool{}
	for _, group := range form.VariableGroups {
		for _, name := range group.Variables {
			name = strings.TrimSuffix(strings.TrimPrefix(name, "${"), "}")
			if _, ok := form.Variables[name]; !ok || grouped[name] {
				t.Fatalf("unresolved/duplicate group field %s", name)
			}
			grouped[name] = true
		}
	}
	for name, field := range form.Variables {
		if field.Visible != false && !grouped[name] {
			t.Fatalf("ungrouped visible input %s", name)
		}
	}
}
