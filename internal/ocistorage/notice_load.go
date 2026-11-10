package ocistorage

import (
	"context"
	"encoding/json"
	"strings"

	"oci-maintenance-router/internal/noticeledger"
)

// LoadRecord reads a discovered object through the complete LoadObject binding
// checks, returning only its immutable record. It provides no CAS/save handle.
func (s *NoticeStore) LoadRecord(ctx context.Context, name string) (noticeledger.Record, error) {
	stored, err := s.LoadObject(ctx, name)
	if err != nil {
		return noticeledger.Record{}, err
	}
	return stored.Record(), nil
}

func validNoticeObjectName(name string) bool {
	return len(name) == 77 && strings.HasPrefix(name, "notices/") &&
		strings.HasSuffix(name, ".json") && environmentDigest.MatchString(name[8:72])
}

// LoadObject discovers identity inside a stored record, then verifies the entire
// canonical record, environment and hashed path. The current GET's ETag binds
// later Save operations. Saved data alone never establishes source freshness.
func (s *NoticeStore) LoadObject(ctx context.Context, name string) (StoredNotice, error) {
	if !s.usable() || !validNoticeObjectName(name) {
		return StoredNotice{}, &Error{Code: InvalidRequest}
	}
	obj, err := s.client.Get(ctx, name)
	if err != nil {
		return StoredNotice{}, err
	}
	var candidate struct {
		NoticeID string `json:"noticeId"`
	}
	if json.Unmarshal(obj.Body, &candidate) != nil || len(candidate.NoticeID) > objectLimit ||
		!storedNoticeID.MatchString(candidate.NoticeID) || noticeObjectName(candidate.NoticeID) != name {
		return StoredNotice{}, failure(ctx, InvalidResponse, 200, false)
	}
	record, err := noticeledger.Decode(obj.Body, s.environmentID, candidate.NoticeID)
	if ctx.Err() != nil {
		return StoredNotice{}, failure(ctx, Canceled, 200, false)
	}
	if err != nil {
		return StoredNotice{}, failure(ctx, InvalidResponse, 200, false)
	}
	return StoredNotice{s, record, obj.ETag}, nil
}
