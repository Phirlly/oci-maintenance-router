package infra_test

import (
	"reflect"
	"strings"
	"testing"
)

func visibleFor(t *testing.T, expression any, values map[string]string) bool {
	t.Helper()
	switch v := expression.(type) {
	case nil:
		return true
	case bool:
		return v
	case map[string]any:
		for op, raw := range v {
			args := raw.([]any)
			if op == "eq" {
				left, right := args[0].(string), args[1].(string)
				if strings.HasPrefix(left, "${") {
					left = values[strings.TrimSuffix(strings.TrimPrefix(left, "${"), "}")]
				}
				return left == right
			}
			if op == "not" {
				return !visibleFor(t, args[0], values)
			}
			if op == "and" {
				return visibleFor(t, args[0], values) && visibleFor(t, args[1], values)
			}
			if op == "or" {
				return visibleFor(t, args[0], values) || visibleFor(t, args[1], values)
			}
		}
	}
	t.Fatalf("unsupported native visibility: %#v", expression)
	return false
}

func TestSetupFormShowsOnlyApplicableTransientControls(t *testing.T) {
	f := readForm(t)
	state, action := f.Variables["routing_state"], f.Variables["setup_action"]
	if state.Default != "Paused" || !reflect.DeepEqual(state.Enum, []string{"Paused", "Enabled"}) || action.Default != "None" || !reflect.DeepEqual(action.Enum, []string{"None", "Send test", "Confirm received tests"}) {
		t.Fatal("missing simple explicit routing/setup controls")
	}
	for _, old := range []string{"mode", "test_run", "operator_test_run"} {
		if _, exists := f.Variables[old]; exists {
			t.Fatalf("old authority exposed: %s", old)
		}
	}
	for _, action := range []string{"None", "Send test", "Confirm received tests"} {
		for _, email := range []string{"", "operator@example.invalid"} {
			values := map[string]string{"routing_state": "Paused", "setup_action": action, "operator_email": email}
			for name, want := range map[string]bool{
				"setup_attempt": action != "None", "test_receipt_code": action == "Confirm received tests",
				"operator_receipt_code":        action == "Confirm received tests" && email != "",
				"test_secret_ocid":             action == "Send test" || action == "Confirm received tests" && email == "",
				"test_secret_compartment_ocid": action == "Send test" || action == "Confirm received tests" && email == "",
				"test_nonproduction":           action == "Send test" || action == "Confirm received tests" && email == "",
			} {
				field, exists := f.Variables[name]
				if !exists || visibleFor(t, field.Visible, values) != want {
					t.Errorf("%s/%q: %s visibility should be %t", action, email, name, want)
				}
			}
		}
	}
	if f.Variables["setup_attempt"].Default != nil {
		t.Fatal("test attempt must require explicit intent")
	}
}
