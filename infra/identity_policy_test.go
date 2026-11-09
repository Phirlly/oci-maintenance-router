package infra_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestActualPolicyStatementsPreserveObjectScope(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("identity.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)\bstatements\s*=\s*(\[.*?\])`).FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("cannot locate actual IAM statements")
	}
	expression := strings.NewReplacer(
		"oci_identity_dynamic_group.router.id", "local.group_id",
		"var.compartment_ocid", "local.compartment_id",
		"oci_objectstorage_bucket.state.name", "local.bucket",
		"\n", " ",
	).Replace(string(match[1]))
	for _, environment := range []string{"alpha", "beta"} {
		t.Run(environment, func(t *testing.T) {
			group := "ocid1.dynamicgroup.oc1.." + environment
			compartment := "ocid1.compartment.oc1.." + environment
			bucket := "example-router-" + environment
			dir := t.TempDir()
			writeJSON(t, filepath.Join(dir, "main.tf.json"), map[string]any{
				"locals": map[string]string{"group_id": group, "compartment_id": compartment, "bucket": bucket},
			})
			output, err := terraform(t, dir, "jsonencode("+expression+")\n", "console", "-no-color")
			var encoded string
			var got []string
			if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &got) != nil {
				t.Fatalf("render policy: %v %s", err, output)
			}
			// Oracle's object-level IAM contract uses quoted folder patterns.
			// Exact statements also guard the principal, bucket and permission limits.
			want := []string{
				fmt.Sprintf("Allow dynamic-group id %s to manage objects in compartment id %s where all {target.bucket.name = '%s', request.permission = 'OBJECT_READ', any {target.object.name = 'configuration/router.json', target.object.name = 'configuration/catalog.json'}}", group, compartment, bucket),
				fmt.Sprintf("Allow dynamic-group id %s to manage objects in compartment id %s where all {target.bucket.name = '%s', target.object.name = 'deployment-checks/*', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}", group, compartment, bucket),
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("rendered IAM scope differs from documented contract:\ngot  %q\nwant %q", got, want)
			}
		})
	}
}
