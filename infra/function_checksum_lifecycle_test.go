package infra_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestFunctionChecksumUpgradeAndRollback(t *testing.T) {
	t.Parallel()
	f := newChecksumLifecycle(t)
	first := f.selectArchive("first synthetic archive", "")
	f.report(first)
	f.plan()
	f.run("apply", "-input=false", "-no-color", "change.tfplan")
	if got := f.diagnosticArchive(); got != first {
		t.Fatalf("baseline diagnostic = %q, want %q", got, first)
	}
	reported := first
	for _, contents := range []string{"second synthetic archive", "first synthetic archive"} {
		desired := f.selectArchive(contents, reported)
		actions := f.plan() // Persisted read state and remote report still have the old digest.
		if !reflect.DeepEqual(actions["terraform_data.function"], []string{"update"}) ||
			!reflect.DeepEqual(actions["data.terraform_remote_state.archive"], []string{"read"}) {
			t.Fatalf("archive change must update Function then read checksum: %v", actions)
		}
		f.report(desired) // Model the checksum returned after the Function update.
		f.run("apply", "-input=false", "-no-color", "change.tfplan")
		if got := f.diagnosticArchive(); got != desired {
			t.Fatalf("diagnostic = %q, want changed archive %q", got, desired)
		}
		reported = desired
	}
}

func TestFunctionChecksumFailureBlocksNewDiagnostic(t *testing.T) {
	t.Parallel()
	for _, missing := range []bool{false, true} {
		name := "wrong"
		if missing {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			f := newChecksumLifecycle(t)
			first := f.selectArchive("first synthetic archive", "")
			f.report(first)
			f.plan()
			f.run("apply", "-input=false", "-no-color", "change.tfplan")
			desired := f.selectArchive("second synthetic archive", first)
			f.plan()
			if missing {
				f.report("")
			}
			output, err := terraform(t, f.dir, "", "apply", "-input=false", "-no-color", "change.tfplan")
			if err == nil || !strings.Contains(string(output), "OCI did not report the bundled Function archive checksum.") {
				t.Fatalf("bad report must fail checksum verification: %s (%v)", output, err)
			}
			// A replacement may delete the old invocation state before the failed
			// read. It must never create the diagnostic for the new archive.
			if got := f.diagnosticArchive(); got == desired || (got != "" && got != first) {
				t.Fatalf("failed checksum allowed unexpected diagnostic %q", got)
			}
		})
	}
}

func TestFunctionChecksumUnchangedDeploymentDetectsDrift(t *testing.T) {
	t.Parallel()
	f := newChecksumLifecycle(t)
	first := f.selectArchive("first synthetic archive", "")
	f.report(first)
	f.plan()
	f.run("apply", "-input=false", "-no-color", "change.tfplan")
	for address, actions := range f.plan() {
		if !reflect.DeepEqual(actions, []string{"no-op"}) {
			t.Fatalf("unchanged deployment plans mutation at %s: %v", address, actions)
		}
	}
	f.report(strings.Repeat("0", 64))
	output, err := terraform(t, f.dir, "", "plan", "-input=false", "-no-color")
	if err == nil || !strings.Contains(string(output), "OCI did not report the bundled Function archive checksum.") {
		t.Fatalf("unchanged Function with wrong remote checksum must fail: %s (%v)", output, err)
	}
}
