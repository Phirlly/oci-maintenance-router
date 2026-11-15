package deployment

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"oci-maintenance-router/internal/noticeledger"
)

// ClosureReviewPage contains only retained, whitelisted evidence. The final code
// binds reviewed state but does not grant invocation or configuration authority.
type ClosureReviewPage struct {
	Evidence            string `json:"evidence"`
	View                string `json:"view"`
	SavedAt             string `json:"savedAt"`
	NoticeSchemaVersion int    `json:"noticeSchemaVersion"`
	TotalItems          int    `json:"totalItems"`
	Items               []any  `json:"items"`
	NextCursor          string `json:"nextCursor,omitempty"`
}

func closurePage(saved noticeledger.Snapshot, items []any, binding string, request command) (*ClosureReviewPage, error) {
	want, offset, err := parseReviewCursor(request.cursor)
	if err != nil {
		return nil, err
	}
	if want != "" && want != binding {
		return nil, operationFailure("review_changed")
	}
	if offset >= len(items) {
		return nil, operationFailure("review_cursor_invalid")
	}
	view := "TARGETS"
	if request.kind != "" {
		view = "TARGET"
	}
	page := &ClosureReviewPage{Evidence: "LAST_SAVED", View: view, SavedAt: saved.UpdatedAt, NoticeSchemaVersion: saved.SchemaVersion, TotalItems: len(items)}
	for end := min(offset+32, len(items)); end > offset; end-- {
		page.Items, page.NextCursor = items[offset:end], ""
		if end < len(items) {
			page.NextCursor = binding + ":" + strconv.Itoa(end)
		}
		body, _ := json.Marshal(page)
		if len(body) <= 6500 {
			return page, nil
		}
	}
	return nil, operationFailure("closure_review_too_large")
}

type closureCodeItem struct {
	Kind     string `json:"kind"`
	Action   string `json:"action"`
	TargetID string `json:"targetId"`
	Code     string `json:"code"`
}

func makeClosureCode(record noticeledger.Record, generation string, request command) (closureCodeItem, error) {
	fingerprint, err := record.ReviewFingerprint(noticeledger.ActionKind(request.kind), request.targetID, generation, "")
	if err != nil {
		return closureCodeItem{}, operationFailure("closure_review_invalid")
	}
	_, notice := record.Identity()
	body, _ := json.Marshal(map[string]any{"kind": request.kind, "noticeId": notice, "reviewFingerprint": fingerprint,
		"routingGeneration": generation, "targetId": request.targetID, "version": 1})
	code := base64.StdEncoding.EncodeToString(body)
	if len(code) > 6000 {
		return closureCodeItem{}, operationFailure("closure_review_too_large")
	}
	return closureCodeItem{"CLOSURE", request.kind, request.targetID, code}, nil
}
