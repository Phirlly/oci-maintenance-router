package infra_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestReceiptFormUsesOptionalExactCodesAndVisibleOrphanRecovery(t *testing.T) {
	f := readForm(t)
	for _, tc := range []struct {
		name, prefix string
		length       int
	}{
		{"test_receipt_code", "OMR-", 36},
		{"operator_receipt_code", "OMR-OP-", 39},
	} {
		v := f.Variables[tc.name]
		if v.Type != "string" || v.Required || v.Default != "" || v.MaxLength != tc.length || v.Pattern == "" {
			t.Fatalf("%s lacks optional bounded confirmation input", tc.name)
		}
		pattern, err := regexp.Compile(v.Pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range []string{"", tc.prefix + strings.Repeat("a", 32)} {
			if !pattern.MatchString(input) {
				t.Fatal("valid empty or exact confirmation rejected")
			}
		}
		for _, input := range []string{tc.prefix + strings.Repeat("A", 32), tc.prefix + strings.Repeat("a", 31), " " + tc.prefix + strings.Repeat("a", 32)} {
			if pattern.MatchString(input) {
				t.Fatal("malformed confirmation allowed")
			}
		}
		if !strings.Contains(v.Description, "code") || !strings.Contains(v.Description, "unchanged") {
			t.Fatal("confirmation hint must explain received code and retained test number")
		}
	}
	visible := map[string]any{"eq": []any{"${mode}", "TEST"}}
	if !reflect.DeepEqual(f.Variables["test_receipt_code"].Visible, visible) {
		t.Fatal("PagerDuty confirmation must only appear in TEST")
	}
	operatorVisible := map[string]any{"and": []any{visible, map[string]any{"or": []any{
		map[string]any{"not": []any{map[string]any{"eq": []any{"${operator_email}", ""}}}},
		map[string]any{"not": []any{map[string]any{"eq": []any{"${operator_receipt_code}", ""}}}},
	}}}}
	if !reflect.DeepEqual(f.Variables["operator_receipt_code"].Visible, operatorVisible) {
		t.Fatal("orphan operator code must remain visible for clearing in TEST")
	}
}
