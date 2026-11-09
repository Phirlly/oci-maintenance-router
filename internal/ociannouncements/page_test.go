package ociannouncements

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestPageKeepsIDsAndOpaqueContinuation(t *testing.T) {
	body := `{"items":[{"id":"ocid1.announcement.oc1..example","announcementType":"UNRELATED"},{"id":"ocid1.announcement.oc1..example","services":null}],"userStatuses":[],"future":true}`
	token := " +/%?é= "
	p, err := decodePage([]byte(body), http.Header{"Opc-Next-Page": {token}}, "previous")
	if err != nil || !reflect.DeepEqual(p.IDs, []string{noticeID, noticeID}) || p.NextPage != token {
		t.Fatalf("page changed: %+v %v", p, err)
	}
	p, err = decodePage([]byte(`{"items":[]}`), http.Header{"Opc-Next-Page": {"next"}}, "")
	if err != nil || len(p.IDs) != 0 || p.NextPage != "next" {
		t.Fatal("empty page stopped pagination")
	}
	p, err = decodePage([]byte(`{"items":[]}`), nil, "")
	if err != nil || len(p.IDs) != 0 || p.NextPage != "" {
		t.Fatal("terminal page rejected")
	}
}

func TestPageRejectsMalformedEntriesAndContinuation(t *testing.T) {
	for _, body := range []string{"null", "[]", "{}", `{"items":null}`, `{"items":{}}`, `{"Items":[]}`, `{"items":[null]}`, `{"items":[{}]}`, `{"items":[{"ID":"x"}]}`, `{"items":[{"id":3}]}`, `{"items":[{"id":"ocid1.announcement.oc2..x"}]}`, `{"items":[{"id":"ocid1.announcement.oc1..example"},{}]}`} {
		p, err := decodePage([]byte(body), nil, "")
		if err == nil || len(p.IDs) != 0 || p.NextPage != "" {
			t.Errorf("partial/invalid page: %s", body)
		}
	}
	for _, values := range [][]string{{}, {""}, {"x", "y"}, {"same"}, {"control\n"}, {"\x7f"}, {string([]byte{0xff})}, {strings.Repeat("x", 4097)}} {
		p, err := decodePage([]byte(`{"items":[]}`), http.Header{"Opc-Next-Page": values}, "same")
		if err == nil || len(p.IDs) != 0 || p.NextPage != "" {
			t.Fatalf("invalid token accepted: %q", values)
		}
	}
}
