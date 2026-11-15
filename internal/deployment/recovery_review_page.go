package deployment

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/reconciliation"
)

// RecoveryReviewPage distinguishes saved targets from current observed review.
// Only the final selected page may contain a code; the code grants no authority.
type RecoveryReviewPage struct {
	Evidence            string `json:"evidence"`
	View                string `json:"view"`
	SavedAt             string `json:"savedAt"`
	NoticeSchemaVersion int    `json:"noticeSchemaVersion"`
	TotalItems          int    `json:"totalItems"`
	Items               []any  `json:"items"`
	NextCursor          string `json:"nextCursor,omitempty"`
}

func recoveryReviewPage(saved noticeledger.Snapshot, items []any, binding string, request command) (*RecoveryReviewPage, error) {
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
	page := &RecoveryReviewPage{Evidence: "LAST_SAVED", View: "TARGETS", SavedAt: saved.UpdatedAt, NoticeSchemaVersion: saved.SchemaVersion, TotalItems: len(items)}
	if request.kind != "" {
		page.Evidence, page.View = "CURRENT_OBSERVED", "TARGET"
	}
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
	return nil, operationFailure("recovery_review_too_large")
}

func recoveryReviewCode(view reconciliation.RecoveryReview, request command) (any, error) {
	_, id := view.Saved.Identity()
	body, _ := json.Marshal(map[string]any{"componentFingerprint": view.Component.Fingerprint, "kind": request.kind, "noticeId": id, "reviewFingerprint": view.Fingerprint, "routingGeneration": view.Routing.RoutingGeneration, "targetId": request.targetID, "version": 1})
	code := base64.StdEncoding.EncodeToString(body)
	if len(code) > 6000 {
		return nil, operationFailure("recovery_review_too_large")
	}
	return map[string]any{"kind": "RECOVERY", "action": request.kind, "targetId": request.targetID, "code": code}, nil
}
