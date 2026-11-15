package deployment

import (
	"encoding/json"
	"regexp"
	"strconv"

	"oci-maintenance-router/internal/noticeledger"
)

// SavedReviewPage is a whitelist of retained evidence, not a current health scan
// or an authorization to retry, close or approve work.
type SavedReviewPage struct {
	Evidence            string            `json:"evidence"`
	NoticeID            string            `json:"noticeId"`
	SavedAt             string            `json:"savedAt"`
	NoticeSchemaVersion int               `json:"noticeSchemaVersion"`
	SourceState         string            `json:"sourceState"`
	TotalItems          int               `json:"totalItems"`
	Items               []SavedReviewItem `json:"items"`
	NextCursor          string            `json:"nextCursor,omitempty"`
}

type SavedReviewItem struct {
	Kind           string `json:"kind"`
	Code           string `json:"code"`
	RevisionDigest string `json:"revisionDigest,omitempty"`
	BatchID        string `json:"batchId,omitempty"`
	ResourceIndex  int    `json:"resourceIndex"`
	Status         string `json:"status,omitempty"`
	LastSeenAt     string `json:"lastSeenAt,omitempty"`
	DestinationID  string `json:"destinationId,omitempty"`
}

var reviewCursorPattern = regexp.MustCompile(`^([0-9a-f]{64}):([1-9][0-9]{0,5})$`)

func parseReviewCursor(cursor string) (string, int, error) {
	if cursor == "" {
		return "", 0, nil
	}
	parts := reviewCursorPattern.FindStringSubmatch(cursor)
	if parts == nil {
		return "", 0, operationFailure("review_cursor_invalid")
	}
	offset, _ := strconv.Atoi(parts[2]) // At most six decimal digits.
	return parts[1], offset, nil
}

func savedReviewPage(saved noticeledger.Snapshot, view noticeledger.SavedReview, binding, cursor string) (*SavedReviewPage, error) {
	want, offset, err := parseReviewCursor(cursor)
	if err != nil {
		return nil, err
	}
	if want != "" && want != binding {
		return nil, operationFailure("review_changed")
	}
	items := savedReviewItems(view)
	if offset != 0 && offset >= len(items) {
		return nil, operationFailure("review_cursor_invalid")
	}
	page := &SavedReviewPage{Evidence: "LAST_SAVED", NoticeID: saved.NoticeID, SavedAt: saved.UpdatedAt,
		NoticeSchemaVersion: saved.SchemaVersion, SourceState: "UNKNOWN", TotalItems: len(items)}
	if saved.SourceOutcome != nil {
		page.SourceState = string(saved.SourceOutcome.State)
	}
	// Leave room for the fixed operational envelope and newline within 8192 bytes.
	for end := min(offset+32, len(items)); end >= offset; end-- {
		page.Items, page.NextCursor = items[offset:end], ""
		if end < len(items) {
			page.NextCursor = binding + ":" + strconv.Itoa(end)
		}
		body, _ := json.Marshal(page)
		if len(body) <= 6500 && (end > offset || len(items) == 0) {
			return page, nil
		}
	}
	return nil, operationFailure("review_too_large")
}
