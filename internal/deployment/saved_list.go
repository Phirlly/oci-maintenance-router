package deployment

import (
	"context"
	"encoding/json"
	"slices"

	"oci-maintenance-router/internal/ocistorage"
)

// SavedNoticePage lists validated retained identities, not current health,
// source freshness, issue classifications or recovery authority.
type SavedNoticePage struct {
	Evidence   string   `json:"evidence"`
	NoticeIDs  []string `json:"noticeIds"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

func composeSavedList(s Settings, objects operationalObjects, p pinnedInputs) (operations, error) {
	notices, err := ocistorage.NewNoticeStore(objects, p.environment)
	if err != nil {
		return operations{}, err
	}
	body, _ := json.Marshal([]any{"oci-maintenance-router/saved-list", 1, p.environment,
		s.ConfigurationSHA256, s.CatalogSHA256, s.ReleaseSHA256})
	binding := digest(body)
	return operations{list: func(ctx context.Context, request command) (*SavedNoticePage, error) {
		return listSavedNotices(ctx, notices, binding, request.cursor)
	}}, nil
}

func listSavedNotices(ctx context.Context, notices *ocistorage.NoticeStore, binding, text string) (*SavedNoticePage, error) {
	cursor, err := parseListingCursor(text)
	if err != nil {
		return nil, err
	}
	if text == "" {
		cursor = listingCursor{Version: 1, Context: binding}
	} else if cursor.Context != binding {
		return nil, operationFailure("listing_changed")
	}
	page, err := notices.ListPage(ctx, cursor.Start)
	if err != nil {
		return nil, operationFailure("listing_read")
	}
	slices.Sort(page.Names) // ListPage supplies an owned copy; OCI ordering is not assumed.
	body, _ := json.Marshal([]any{binding, cursor.Start, page.Names, page.NextStartWith})
	pageDigest := digest(body)
	if cursor.Offset > 0 && cursor.PageDigest != pageDigest {
		return nil, operationFailure("listing_changed")
	}
	if cursor.Offset > 0 && cursor.Offset >= len(page.Names) {
		return nil, operationFailure("listing_cursor_invalid")
	}
	if page.NextStartWith != "" && page.NextStartWith == cursor.Start {
		return nil, operationFailure("listing_cycle")
	}
	result := &SavedNoticePage{Evidence: "RETAINED_IDS", NoticeIDs: []string{}}
	// Bound reads independently of output size. A byte-deferred item is reread
	// on the next call; its offset is never consumed by this response.
	for i := cursor.Offset; i < min(cursor.Offset+10, len(page.Names)); i++ {
		record, err := notices.LoadRecord(ctx, page.Names[i])
		if err != nil {
			return nil, operationFailure("listing_notice_read")
		}
		_, id := record.Identity()
		if len(id) > 4096 || !reviewNoticeID.MatchString(id) {
			return nil, operationFailure("listing_notice_invalid")
		}
		next, err := nextListingCursor(cursor, page, pageDigest, i+1)
		if err != nil {
			return nil, err
		}
		candidate := SavedNoticePage{Evidence: result.Evidence, NoticeIDs: append(result.NoticeIDs, id), NextCursor: next}
		body, _ := json.Marshal(candidate)
		if len(body) > 6500 {
			break
		}
		*result = candidate
	}
	if len(result.NoticeIDs) == 0 {
		if len(page.Names) != 0 {
			return nil, operationFailure("listing_too_large")
		}
		result.NextCursor, err = nextListingCursor(cursor, page, pageDigest, 0)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func nextListingCursor(c listingCursor, page ocistorage.NoticePage, pageDigest string, offset int) (string, error) {
	if offset < len(page.Names) {
		c.Offset, c.PageDigest = offset, pageDigest
		return encodeListingCursor(c), nil
	}
	if page.NextStartWith == "" {
		return "", nil
	}
	if c.Page == 99 {
		return "", operationFailure("listing_limit")
	}
	c.Start, c.Page, c.Offset, c.PageDigest = page.NextStartWith, c.Page+1, 0, ""
	return encodeListingCursor(c), nil
}
