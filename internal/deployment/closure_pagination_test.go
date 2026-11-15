package deployment

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func closurePages(t *testing.T, f savedReviewFixture, kind, target string) []map[string]any {
	t.Helper()
	cursor := ""
	items := []map[string]any{}
	for count := 0; count < 100; count++ {
		got := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, kind, target, cursor))
		if got["status"] != "reviewed" {
			t.Fatal(got)
		}
		page := got["closureReview"].(map[string]any)
		body, _ := json.Marshal(page)
		rows := page["items"].([]any)
		if len(rows) == 0 || len(rows) > 32 || len(body) > 6500 {
			t.Fatal("unbounded or stalled page")
		}
		for _, row := range rows {
			items = append(items, row.(map[string]any))
		}
		cursor, _ = page["nextCursor"].(string)
		if cursor == "" {
			if len(items) != int(page["totalItems"].(float64)) {
				t.Fatal("lost page items")
			}
			return items
		}
		for _, row := range rows {
			if row.(map[string]any)["kind"] == "CLOSURE" {
				t.Fatal("code preceded unfinished review")
			}
		}
	}
	t.Fatal("pagination did not terminate")
	return nil
}

func TestClosureReviewBoundsMaximumNoticeAndCompleteResources(t *testing.T) {
	id := "ocid1.announcement.oc1.." + strings.Repeat("a", 4096-len("ocid1.announcement.oc1.."))
	f := newSavedReviewFixture(t, id, 80, false)
	before := businessObjects(f.objects)
	items := closurePages(t, f, "CLOSE_REVISION", f.notice.Key().Digest)
	resources := map[float64]bool{}
	for _, row := range items {
		if row["kind"] == "RESOURCE" {
			index := row["index"].(float64)
			if resources[index] {
				t.Fatal("duplicated resource")
			}
			resources[index] = true
		}
	}
	if len(resources) != 80 || items[len(items)-1]["kind"] != "CLOSURE" {
		t.Fatal("incomplete review")
	}
	code := items[len(items)-1]["code"].(string)
	if len(code) != 5868 {
		t.Fatal("unexpected maximum code length", len(code))
	}
	body, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		t.Fatal(err)
	}
	var intent map[string]any
	_ = json.Unmarshal(body, &intent)
	if intent["noticeId"] != id || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("truncated identity or changed saved work")
	}
}

func TestClosureReviewRejectsChangedContextOrSelectedTarget(t *testing.T) {
	for _, kind := range []string{"record", "configuration", "release", "selection"} {
		t.Run(kind, func(t *testing.T) {
			f := newSavedReviewFixture(t, "ocid1.announcement.oc1..paging", 80, false)
			target := f.notice.Key().Digest
			first := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, "CLOSE_REVISION", target, ""))
			cursor := first["closureReview"].(map[string]any)["nextCursor"].(string)
			action := "CLOSE_REVISION"
			switch kind {
			case "record":
				for name, obj := range f.objects.objects {
					if strings.HasPrefix(name, "notices/") {
						obj.Body = []byte(strings.ReplaceAll(string(obj.Body), "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:01.000Z"))
						f.objects.objects[name] = obj
					}
				}
			case "configuration":
				f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) { v["mode"] = "PREVIEW" })
			case "release":
				f.s.ReleaseSHA256 = strings.Repeat("f", 64)
				body, _ := json.Marshal(f.s)
				f.h.getenv = func(string) string { return string(body) }
			case "selection":
				action = ""
				target = ""
			}
			got := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, action, target, cursor))
			if got["status"] != "not-ready" || got["code"] != "review_changed" || got["closureReview"] != nil {
				t.Fatal("changed review accepted", got)
			}
		})
	}
}

func TestClosurePageRejectsBadOffsetsAndOversizedItems(t *testing.T) {
	binding := strings.Repeat("a", 64)
	for _, cursor := range []string{"bad", binding + ":0", binding + ":01", binding + ":2"} {
		if p, err := closurePage(noticeledger.Snapshot{}, []any{"item"}, binding, command{cursor: cursor}); err == nil || p != nil {
			t.Fatal("invalid offset admitted", cursor)
		}
	}
	if p, err := closurePage(noticeledger.Snapshot{}, []any{strings.Repeat("x", 6500)}, binding, command{}); err == nil || p != nil {
		t.Fatal("oversized item truncated")
	}
}
