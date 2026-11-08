package ocistorage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"

	"oci-maintenance-router/internal/noticeledger"
)

// NoticeStore binds complete notice records to one verified environment/location.
// Bootstrap owns association of the environment identity with this client.
type NoticeStore struct {
	client        *Client
	environmentID string
}

// StoredNotice is a confirmed read/write version, never send authorization.
// Conflict or uncertain writes return no handle: load and redecide before work.
type StoredNotice struct {
	store  *NoticeStore
	record noticeledger.Record
	etag   string
}

var environmentDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var storedNoticeID = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func NewNoticeStore(client *Client, environmentID string) (*NoticeStore, error) {
	if client == nil || client.signer == nil || client.transport == nil || !environmentDigest.MatchString(environmentID) {
		return nil, &Error{Code: InvalidConfiguration}
	}
	return &NoticeStore{client, environmentID}, nil
}
func (s StoredNotice) Record() noticeledger.Record { return s.record }
func (s *NoticeStore) usable() bool                { return s != nil && s.client != nil && s.environmentID != "" }
func noticeObjectName(id string) string {
	return fmt.Sprintf("notices/%x.json", sha256.Sum256([]byte(id)))
}

func (s *NoticeStore) Load(ctx context.Context, id string) (StoredNotice, error) {
	if !s.usable() || len(id) > 1<<20 || !storedNoticeID.MatchString(id) {
		return StoredNotice{}, &Error{Code: InvalidRequest}
	}
	obj, err := s.client.Get(ctx, noticeObjectName(id))
	if err != nil {
		return StoredNotice{}, err
	}
	r, err := noticeledger.Decode(obj.Body, s.environmentID, id)
	if err != nil {
		return StoredNotice{}, &Error{Code: InvalidResponse, Status: 200}
	}
	return StoredNotice{s, r, obj.ETag}, nil
}

func (s *NoticeStore) Create(ctx context.Context, r noticeledger.Record) (StoredNotice, error) {
	env, id := r.Identity()
	if !s.usable() || env != s.environmentID || id == "" {
		return StoredNotice{}, &Error{Code: InvalidRequest}
	}
	body, err := r.Encode()
	if err != nil {
		return StoredNotice{}, err
	}
	etag, err := s.client.Create(ctx, noticeObjectName(id), body)
	if err != nil {
		return StoredNotice{}, err
	}
	return StoredNotice{s, r, etag}, nil
}

// Save accepts exactly one transition of the record bound to previous's ETag.
// An older proposal cannot be combined with a more recently loaded version.
func (s *NoticeStore) Save(ctx context.Context, previous StoredNotice, next noticeledger.Record) (StoredNotice, error) {
	if !s.usable() || previous.store != s || !next.Follows(previous.record) {
		return StoredNotice{}, &Error{Code: InvalidRequest}
	}
	env, id := next.Identity()
	oldEnv, oldID := previous.record.Identity()
	if env != s.environmentID || env != oldEnv || id != oldID {
		return StoredNotice{}, &Error{Code: InvalidRequest}
	}
	body, err := next.Encode()
	if err != nil {
		return StoredNotice{}, err
	}
	etag, err := s.client.Replace(ctx, noticeObjectName(id), previous.etag, body)
	if err != nil {
		return StoredNotice{}, err
	}
	return StoredNotice{s, next, etag}, nil
}
