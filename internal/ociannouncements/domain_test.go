package ociannouncements

import (
	"fmt"
	"strings"
	"testing"
)

func TestDetailPreservesDomainRevisionRules(t *testing.T) {
	n, err := decodeNotice([]byte(detailed), noticeID)
	if err != nil {
		t.Fatal(err)
	}
	changedContext := strings.Replace(strings.Replace(detailed, "INACTIVE", "ACTIVE", 1), "2026-01-01T10:00:00.123456Z", "2026-01-02T10:00:00Z", 1)
	same, err := decodeNotice([]byte(changedContext), noticeID)
	if err != nil || same.Key() != n.Key() {
		t.Fatal("publication context changed revision")
	}
	changed, err := decodeNotice([]byte(strings.Replace(detailed, "configured window", "updated window", 1)), noticeID)
	if err != nil || changed.Key() == n.Key() {
		t.Fatal("source update with same timestamps lost")
	}
	// Duplicate resources collapse only when their content agrees.
	resource := `{"resourceId":"r","additionalProperties":[{"name":"","value":""}]}`
	body := strings.TrimSuffix(minimal, "}") + `,"services":["z","a","a"],"affectedResources":[` + resource + "," + resource + "]}"
	n, err = decodeNotice([]byte(body), noticeID)
	if err != nil || len(n.Snapshot().Resources) != 1 || strings.Join(n.Snapshot().Services, ",") != "a,z" {
		t.Fatal("domain normalization bypassed")
	}
	body = strings.Replace(body, `"value":""`, `"value":"different"`, 1)
	if _, err = decodeNotice([]byte(body), noticeID); err == nil {
		t.Fatal("conflicting resource content accepted")
	}
}

func TestDomainAndPageCapacity(t *testing.T) {
	var resources []string
	for i := 0; i < 257; i++ {
		resources = append(resources, fmt.Sprintf(`{"resourceId":"r%d"}`, i))
	}
	body := strings.TrimSuffix(minimal, "}") + `,"affectedResources":[` + strings.Join(resources, ",") + "]}"
	if _, err := decodeNotice([]byte(body), noticeID); err == nil {
		t.Fatal("domain resource bound bypassed")
	}
	body = strings.TrimSuffix(minimal, "}") + `,"description":"` + strings.Repeat("x", 1<<20) + `"}`
	if _, err := decodeNotice([]byte(body), noticeID); err == nil {
		t.Fatal("domain byte bound bypassed")
	}
	item := `{"id":"` + noticeID + `"}`
	for _, count := range []int{100, 101} {
		body = `{"items":[` + strings.TrimSuffix(strings.Repeat(item+",", count), ",") + "]}"
		p, err := decodePage([]byte(body), nil, "")
		if count == 100 && (err != nil || len(p.IDs) != 100) {
			t.Fatal("full page rejected")
		}
		if count == 101 && (err == nil || len(p.IDs) != 0) {
			t.Fatal("oversize page accepted")
		}
	}
}
