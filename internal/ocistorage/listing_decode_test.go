package ocistorage

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func listedName(n int) string { return fmt.Sprintf("notices/%064x.json", n) }
func pageJSON(names []string, next string) []byte {
	objects := []map[string]string{}
	for _, name := range names {
		objects = append(objects, map[string]string{"name": name})
	}
	b, _ := json.Marshal(map[string]any{"objects": objects, "nextStartWith": next})
	return b
}
func emptyPage(t *testing.T, p NoticePage) {
	t.Helper()
	if p.Names != nil || p.NextStartWith != "" {
		t.Fatal("partial page escaped")
	}
}

func TestNoticePagePreservesNamesAndBodyContinuation(t *testing.T) {
	names := []string{listedName(2), listedName(1)}
	p, err := decodeNoticePage(pageJSON(names, listedName(3)))
	if err != nil || !reflect.DeepEqual(p.Names, names) || p.NextStartWith != listedName(3) {
		t.Fatalf("page=%+v err=%v", p, err)
	}
	for _, suffix := range []string{"", `,"nextStartWith":null`, `,"nextStartWith":""`} {
		p, err := decodeNoticePage([]byte(`{"objects":[]` + suffix + "}"))
		if err != nil || len(p.Names) != 0 || p.NextStartWith != "" {
			t.Fatal("valid terminal page rejected")
		}
	}
	p, err = decodeNoticePage(pageJSON(nil, listedName(3)))
	if err != nil || len(p.Names) != 0 || p.NextStartWith == "" {
		t.Fatal("empty continuation lost")
	}
	p.Names = append(p.Names, "caller mutation")
	again, err := decodeNoticePage(pageJSON(nil, listedName(3)))
	if err != nil || len(again.Names) != 0 {
		t.Fatal("results share state")
	}
}

func TestMalformedNoticePagesFailWithoutPartialResults(t *testing.T) {
	valid := string(pageJSON([]string{listedName(1)}, listedName(2)))
	for _, raw := range []string{
		"null", "[]", "{}", `{"objects":null}`, `{"objects":{}}`,
		`{"objects":[null]}`, `{"objects":[{}]}`,
		`{"objects":[{"name":null}]}`, `{"objects":[{"name":7}]}`,
		strings.Replace(valid, listedName(1), "configuration/router.json", 1),
		strings.Replace(valid, listedName(2), "notices/../other", 1),
		strings.Replace(valid, `"nextStartWith":"`+listedName(2)+`"`, `"nextStartWith":1`, 1),
		string(pageJSON([]string{listedName(1), listedName(1)}, "")),
		strings.TrimSuffix(valid, "}") + `,"Objects":[]}`,
		strings.Replace(valid, `"name":`, `"Name":`, 1),
		strings.TrimSuffix(valid, "}") + `,"prefixes":["notices/nested/"]}`,
		strings.TrimSuffix(valid, "}") + `,"prefixes":{}}`,
	} {
		p, err := decodeNoticePage([]byte(raw))
		if err == nil {
			t.Fatalf("accepted malformed page %s", raw)
		}
		emptyPage(t, p)
	}
}

func TestNoticePageLimitAndAdditiveFields(t *testing.T) {
	names := make([]string, 1000)
	for i := range names {
		names[i] = listedName(i)
	}
	p, err := decodeNoticePage(pageJSON(names, ""))
	if err != nil || len(p.Names) != 1000 {
		t.Fatal("valid maximum rejected", err)
	}
	p, err = decodeNoticePage(pageJSON(append(names, listedName(1000)), ""))
	if err == nil {
		t.Fatal("page limit not enforced")
	}
	emptyPage(t, p)
	for _, prefixes := range []string{"null", "[]"} {
		raw := `{"objects":[{"name":"` + listedName(1) + `","size":1,"future":{"ok":true}}],"prefixes":` + prefixes + `,"future":[null,1]}`
		if _, err := decodeNoticePage([]byte(raw)); err != nil {
			t.Fatal("additive field rejected", err)
		}
	}
}
