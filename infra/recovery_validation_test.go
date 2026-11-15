package infra_test

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRecoveryRendererRejectsUnreviewedOrInactiveSending(t *testing.T) {
	for _, kind := range []string{"mode", "closure", "missing-number", "zero", "fraction", "large", "code", "kind", "version", "null", "numeric-hash", "extra", "duplicate", "whitespace", "padding", "notice", "long-notice", "oversized", "reordered", "component"} {
		t.Run(kind, func(t *testing.T) {
			s := recoverySettings("RETRY_BATCH")
			v := recoveryIntent("RETRY_BATCH")
			switch kind {
			case "mode":
				s["mode"] = "PAUSED"
			case "closure":
				s["closure_confirm"] = true
			case "missing-number":
				delete(s, "recovery_request_number")
			case "zero":
				s["recovery_request_number"] = 0
			case "fraction":
				s["recovery_request_number"] = 1.5
			case "large":
				s["recovery_request_number"] = 2147483648
			case "kind":
				v["kind"] = "CLOSE_BATCH"
			case "version":
				v["version"] = "1"
			case "null":
				v["targetId"] = nil
			case "numeric-hash":
				v["targetId"] = 123
			case "extra":
				v["unknown"] = "value"
			case "notice":
				v["noticeId"] = "wrong"
			case "long-notice":
				v["noticeId"] = "ocid1.announcement.oc1.." + strings.Repeat("a", 4096)
			case "component":
				delete(v, "componentFingerprint")
			}
			code := closureCode(v)
			body, _ := base64.StdEncoding.DecodeString(code)
			switch kind {
			case "code":
				code = "invalid"
			case "oversized":
				code = strings.Repeat("A", 6004)
			case "reordered":
				text := string(body)
				text = strings.Replace(text, `,"version":1`, ``, 1)
				code = base64.StdEncoding.EncodeToString([]byte(`{"version":1,` + text[1:]))
			case "duplicate":
				code = base64.StdEncoding.EncodeToString([]byte(strings.Replace(string(body), `"version":1`, `"version":1,"version":1`, 1)))
			case "whitespace":
				code = base64.StdEncoding.EncodeToString(append(body, ' '))
			case "padding":
				code += "\n"
			}
			s["recovery_review_code"] = code
			_, message, err := renderRecovery(t, s)
			if err == nil || !strings.Contains(message, "Recovery") {
				t.Fatal("invalid sending intent admitted", kind, message, err)
			}
		})
	}
}

func TestRecoveryFormDefaultsToNoActionAndCanBeCleared(t *testing.T) {
	f := readForm(t)
	for _, name := range []string{"recovery_review_code", "recovery_request_number", "recovery_confirm"} {
		v, ok := f.Variables[name]
		if !ok || v.Visible != true || v.Required {
			t.Fatal("missing or uncleareable control", name)
		}
	}
	if f.Variables["recovery_review_code"].Default != "" || f.Variables["recovery_review_code"].MaxLength != 6000 || f.Variables["recovery_confirm"].Default != false || f.Variables["recovery_request_number"].Default != nil {
		t.Fatal("implicit sending intent")
	}
}
