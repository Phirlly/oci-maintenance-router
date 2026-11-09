package ocimysql

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/routing"
)

const dbID = "ocid1.mysqldbsystem.oc1.iad.example"
const tenancy = "ocid1.tenancy.oc1..example"
const compartment = "ocid1.compartment.oc1..example"
const minimal = `{"id":"ocid1.mysqldbsystem.oc1.iad.example","compartmentId":"ocid1.compartment.oc1..example"}`
const tagged = `{"id":"ocid1.mysqldbsystem.oc1.iad.example","compartmentId":"ocid1.compartment.oc1..example","definedTags":{"Operations":{"Owner":"payments","owner":"video"},"operations":{"Owner":"payments"},"Other":{"number":9007199254740993,"null":null,"boolean":true,"object":{"a":[1,"value"]}}},"freeformTags":{"Owner":"payments","owner":"video","empty":"","spaced":" unchanged "},"lifecycleState":"FUTURE_STATE","customerContacts":[{"email":"synthetic@example.invalid"}]}`

func TestMetadataPreservesExactNamesAndTypes(t *testing.T) {
	m, err := decodeMetadata([]byte(tagged), dbID, tenancy)
	if err != nil || m.ID == nil || *m.ID != dbID || m.CompartmentID == nil || *m.CompartmentID != compartment {
		t.Fatalf("invalid metadata identity: %v", err)
	}
	if len(m.DefinedTags) != 3 || m.DefinedTags["Operations"]["Owner"] != "payments" || m.DefinedTags["Operations"]["owner"] != "video" || m.DefinedTags["operations"]["Owner"] != "payments" {
		t.Fatal("tag names collapsed")
	}
	other := m.DefinedTags["Other"]
	if other["number"] != json.Number("9007199254740993") || other["null"] != nil || other["boolean"] != true || !reflect.DeepEqual(other["object"], map[string]any{"a": []any{json.Number("1"), "value"}}) {
		t.Fatal("defined values coerced")
	}
	if !reflect.DeepEqual(m.FreeformTags, map[string]string{"Owner": "payments", "owner": "video", "empty": "", "spaced": " unchanged "}) {
		t.Fatal("freeform tags changed")
	}
}

func TestOptionalTagsAndRootCompartment(t *testing.T) {
	for _, extra := range []string{"", `,"definedTags":null,"freeformTags":null`, `,"definedTags":{},"freeformTags":{}`} {
		for _, location := range []string{compartment, tenancy, "ocid1.compartment.oc1..future.example"} {
			body := strings.Replace(strings.TrimSuffix(minimal, "}")+extra+"}", compartment, location, 1)
			m, err := decodeMetadata([]byte(body), dbID, tenancy)
			if err != nil || m.CompartmentID == nil || *m.CompartmentID != location || len(m.DefinedTags) != 0 || len(m.FreeformTags) != 0 {
				t.Fatalf("valid optional/root case rejected: %v", err)
			}
		}
	}
}

func TestMalformedMetadataReturnsNoProjection(t *testing.T) {
	cases := []string{"null", "[]", "{}", `{"data":` + minimal + "}", strings.Replace(minimal, dbID, dbID+"other", 1)}
	for _, field := range []string{"id", "compartmentId"} {
		for _, bad := range []string{"null", "false", "3", "[]", "{}", `""`} {
			var value map[string]json.RawMessage
			json.Unmarshal([]byte(minimal), &value)
			value[field] = json.RawMessage(bad)
			body, _ := json.Marshal(value)
			cases = append(cases, string(body))
		}
		var value map[string]json.RawMessage
		json.Unmarshal([]byte(minimal), &value)
		delete(value, field)
		body, _ := json.Marshal(value)
		cases = append(cases, string(body))
	}
	for _, bad := range []string{"ocid1.tenancy.oc1..foreign", "ocid1.compartment.oc2..x", "ocid1.compartment.oc1.iad.x", "ocid1.compartment.oc1..bad/path", strings.Repeat("x", 4097)} {
		cases = append(cases, strings.Replace(minimal, compartment, bad, 1))
	}
	for _, extra := range []string{
		`,"ID":"alias"`, `,"CompartmentId":"alias"`, `,"DefinedTags":{}`, `,"FreeformTags":{}`,
		`,"definedTags":[]`, `,"definedTags":"invalid"`, `,"definedTags":{"Operations":null}`, `,"definedTags":{"Operations":[]}`, `,"definedTags":{"Operations":"invalid"}`,
		`,"freeformTags":[]`, `,"freeformTags":{"Owner":null}`, `,"freeformTags":{"Owner":42}`, `,"freeformTags":{"Owner":true}`, `,"freeformTags":{"Owner":[]}`, `,"freeformTags":{"Owner":{}}`,
	} {
		cases = append(cases, strings.TrimSuffix(minimal, "}")+extra+"}")
	}
	for _, body := range cases {
		m, err := decodeMetadata([]byte(body), dbID, tenancy)
		if err == nil || !reflect.DeepEqual(m, routing.Metadata{}) {
			t.Errorf("malformed response accepted: %s", body)
		}
	}
}
