package ociannouncements

import (
	"errors"
	"strings"
	"testing"
)

func TestDetailPreservesExplicitNullPropertyAndAffectedResources(t *testing.T) {
	body := strings.TrimSuffix(minimal, "}") + `,"affectedResources":[
	{"resourceId":"first","additionalProperties":[{"name":"label","value":null},{"name":"label","value":""}]},
	{"resourceId":"second","additionalProperties":[{"name":"label","value":null}]}]}`
	n, err := decodeNotice([]byte(body), noticeID)
	if err != nil {
		t.Fatal(err)
	}
	r := n.Snapshot().Resources
	if len(r) != 2 || r[0].ResourceID != "first" || r[1].ResourceID != "second" {
		t.Fatal("affected resources lost")
	}
	a, b := r[0].AdditionalProperties, r[1].AdditionalProperties
	if len(a) != 2 || len(b) != 1 || *a[0].Name != "label" || a[0].Value != nil || a[1].Value == nil || *a[1].Value != "" || b[0].Value != nil {
		t.Fatal("null and empty properties were dropped or conflated")
	}
}

func TestDetailNullCompatibilityKeepsPropertyValidationStrict(t *testing.T) {
	for name, property := range map[string]string{
		"missing-value": `{"name":"label"}`,
		"number":        `{"name":"label","value":1}`,
		"boolean":       `{"name":"label","value":false}`,
		"object":        `{"name":"label","value":{}}`,
		"array":         `{"name":"label","value":[]}`,
		"missing-name":  `{"value":null}`,
		"null-name":     `{"name":null,"value":null}`,
		"number-name":   `{"name":1,"value":null}`,
		"value-alias":   `{"name":"label","Value":null}`,
		"duplicate":     `{"name":"label","value":null,"value":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.TrimSuffix(minimal, "}") + `,"affectedResources":[{"resourceId":"first","additionalProperties":[` + property + `]}]}`
			n, err := decodeNotice([]byte(body), noticeID)
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != InvalidResponse || n.Key().NoticeID != "" {
				t.Fatal("invalid property accepted or partial notice returned", err)
			}
		})
	}
}
