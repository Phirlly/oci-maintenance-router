package ociannouncements

import (
	"strings"
	"testing"
)

func TestMalformedJSONFailsAtBothReadBoundaries(t *testing.T) {
	for _, extra := range []string{
		`,"id":"duplicate"`, `,"\u0069d":"escaped-duplicate"`, `,"future":{"x":1,"x":2}`,
		`,"future":"\ud800"`, `,"future":"\udfff"`, `,"future":"\ud800\u0041"`,
		`,"future":"\ud800x"`, `,"future":"\ud800\uD800"`,
		`,"future":` + strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32),
	} {
		for _, base := range []string{minimal, `{"items":[],"id":"ignored"}`} {
			body := []byte(strings.TrimSuffix(base, "}") + extra + "}")
			n, e := decodeNotice(body, noticeID)
			p, e2 := decodePage(body, nil, "")
			if e == nil || e2 == nil || n.Key().NoticeID != "" || len(p.IDs) != 0 {
				t.Errorf("invalid JSON accepted: %q", extra)
			}
		}
	}
	for _, body := range [][]byte{nil, []byte("{"), []byte(minimal + "{}"), []byte(strings.Replace(minimal, "SCHEDULED", string([]byte{0xff}), 1))} {
		if _, err := decodeNotice(body, noticeID); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
}

func TestValidUnicodeEscapesAndIgnoredNestedData(t *testing.T) {
	for _, value := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"\u0041\n\t\""`, strings.Repeat("[", 31) + "0" + strings.Repeat("]", 31)} {
		body := strings.TrimSuffix(minimal, "}") + `,"future":` + value + "}"
		if _, err := decodeNotice([]byte(body), noticeID); err != nil {
			t.Errorf("valid ignored value rejected: %s %v", value, err)
		}
	}
}

func TestWireAndDecodedValueBounds(t *testing.T) {
	for _, base := range []string{minimal, `{"items":[]}`} {
		for _, suffix := range []string{
			`,"future":"` + strings.Repeat("x", 2<<20) + `"`,
			`,"future":[` + strings.Repeat("0,", 65536) + "0]",
		} {
			body := []byte(strings.TrimSuffix(base, "}") + suffix + "}")
			if _, err := decodeNotice(body, noticeID); err == nil {
				t.Fatal("unbounded detail")
			}
			if _, err := decodePage(body, nil, ""); err == nil {
				t.Fatal("unbounded page")
			}
		}
	}
}
