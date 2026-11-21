package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func rootResourceSelection(t *testing.T, choices any, useDefault bool) ([]string, string, error) {
	t.Helper()
	source := readChecksumSource(t, "variables_routing.tf")
	variable := regexp.MustCompile(`(?ms)^variable "resource_types" \{.*?^}`).FindString(source)
	locals := regexp.MustCompile(`(?ms)^locals \{.*?^}`).FindString(source)
	if variable == "" || locals == "" {
		t.Fatal("missing root resource-type input/mapping")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(variable+"\n"+locals+"\noutput \"selected\" { value = local.resource_type_ids }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !useDefault {
		writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"resource_types": choices})
	}
	if out, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		t.Fatalf("init: %s (%v)", out, err)
	}
	out, err := terraform(t, dir, "", "plan", "-input=false", "-no-color", "-out=plan")
	if err != nil {
		return nil, string(out), err
	}
	out, err = terraform(t, dir, "", "show", "-json", "plan")
	var plan struct {
		Planned struct {
			Outputs map[string]struct{ Value []string }
		} `json:"planned_values"`
	}
	if err != nil || json.Unmarshal(out, &plan) != nil {
		t.Fatalf("plan decode: %s (%v)", out, err)
	}
	return plan.Planned.Outputs["selected"].Value, "", nil
}

func TestResourceFormUsesOneSharedSelection(t *testing.T) {
	f := readForm(t)
	field := f.Variables["resource_types"]
	if field.Type != "enum" || field.Title != "Resource types" || !field.Required || field.AdditionalProps["allowMultiple"] != true || field.Default != "HeatWave MySQL" || !reflect.DeepEqual(field.Enum, []string{"HeatWave MySQL", "Compute instances"}) {
		t.Fatalf("wrong resource selector: %+v", field)
	}
	if len(f.VariableGroups) != 7 || f.VariableGroups[1].Title != "Resources to monitor" || !reflect.DeepEqual(f.VariableGroups[1].Variables, []string{"${resource_types}", "${mysql_regions}", "${mysql_compartments}"}) {
		t.Fatal("selection added another scope/group")
	}
	if f.Variables["mysql_regions"].Title != "Regions" || f.Variables["mysql_compartments"].Title != "Compartments" {
		t.Fatal("shared scope still labelled MySQL-only")
	}
	for _, name := range []string{"mysql_compartment", "destination_secret"} {
		if !strings.Contains(f.Variables[name].Description, "reselect") {
			t.Fatal("lost native picker guidance", name)
		}
	}
}

func TestRootResourceInputDefaultsMapsAndRejectsInvalidChoices(t *testing.T) {
	for _, tc := range []struct {
		choices    []string
		want       []string
		useDefault bool
	}{
		{nil, []string{"mysqldbsystem"}, true},
		{[]string{"HeatWave MySQL"}, []string{"mysqldbsystem"}, false},
		{[]string{"Compute instances"}, []string{"instance"}, false},
		{[]string{"HeatWave MySQL", "Compute instances"}, []string{"instance", "mysqldbsystem"}, false},
	} {
		got, message, err := rootResourceSelection(t, tc.choices, tc.useDefault)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("selection: %v %s (%v)", got, message, err)
		}
	}
	for _, choices := range [][]string{{}, {"Compute instances", "Compute instances"}, {"compute instances"}, {"instance"}, {"Bucket"}} {
		_, message, err := rootResourceSelection(t, choices, false)
		if err == nil || !strings.Contains(message, "Resource types") {
			t.Fatalf("invalid choice %q accepted: %s (%v)", choices, message, err)
		}
	}
}
