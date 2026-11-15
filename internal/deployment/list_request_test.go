package deployment

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func listInvocation(cursor string) string {
	fields := map[string]string{"operation": "list-notices"}
	if cursor != "" {
		fields["cursor"] = cursor
	}
	body, _ := json.Marshal(fields)
	return string(body)
}

func TestListCommandAcceptsBoundedRequest(t *testing.T) {
	body := listInvocation("")
	if _, err := parseCommand([]byte(body)); err != nil {
		t.Fatal("list command rejected", err)
	}
	padded := body + strings.Repeat(" ", 1024-len(body))
	if _, err := parseCommand([]byte(padded)); err != nil {
		t.Fatal("exact request budget rejected", err)
	}
	if _, err := parseCommand([]byte(padded + " ")); err == nil {
		t.Fatal("oversized request accepted")
	}
}

func TestListCommandRejectsAuthorityAndMalformedFields(t *testing.T) {
	for _, body := range []string{
		`{"operation":"list-notices","noticeId":"ocid1.announcement.oc1..example"}`,
		`{"operation":"list-notices","mode":"LIVE"}`,
		`{"operation":"list-notices","configurationSha256":"` + strings.Repeat("a", 64) + `"}`,
		`{"operation":"list-notices","cursor":null}`,
		`{"operation":"list-notices","cursor":"","cursor":""}`,
		listInvocation("bad"), listInvocation("") + `{}`,
	} {
		if _, err := parseCommand([]byte(body)); err == nil {
			t.Fatalf("invalid listing request accepted: %q", body)
		}
	}
}

func TestListDiscoversIDForSavedReview(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, false)
	out := httptest.NewRecorder()
	f.h.Serve(operationalContext(t), strings.NewReader(listInvocation("")), out)
	var got struct {
		Status string `json:"status"`
		Saved  struct {
			Evidence string   `json:"evidence"`
			IDs      []string `json:"noticeIds"`
		} `json:"savedNotices"`
	}
	if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Status != "listed" || got.Saved.Evidence != "RETAINED_IDS" || len(got.Saved.IDs) != 1 || got.Saved.IDs[0] != f.notice.Key().NoticeID {
		t.Fatalf("retained ID unavailable: %s", out.Body)
	}
	review := invokeSavedReview(t, f.h, got.Saved.IDs[0], "")
	if review.Status != "reviewed" || review.Review.NoticeID != got.Saved.IDs[0] {
		t.Fatalf("discovered notice cannot be inspected: %+v", review)
	}
}
