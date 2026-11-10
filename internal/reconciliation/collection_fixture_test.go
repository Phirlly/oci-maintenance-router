package reconciliation

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

var _ AnnouncementSource = (*ociannouncements.Client)(nil)
var _ SavedNoticeSource = (*ocistorage.NoticeStore)(nil)

type announcementDetails struct {
	page announcementPages
	get  func(context.Context, string) (notices.Notice, error)
}

func (a announcementDetails) ListPage(ctx context.Context, cursor string) (ociannouncements.Page, error) {
	if a.page == nil {
		return ociannouncements.Page{}, nil
	}
	return a.page(ctx, cursor)
}
func (a announcementDetails) Get(ctx context.Context, id string) (notices.Notice, error) {
	if a.get == nil {
		return notices.Notice{}, errors.New("unexpected Get")
	}
	return a.get(ctx, id)
}

type savedDetails struct {
	page savedPages
	load func(context.Context, string) (noticeledger.Record, error)
}

func (s savedDetails) ListPage(ctx context.Context, cursor string) (ocistorage.NoticePage, error) {
	if s.page == nil {
		return ocistorage.NoticePage{}, nil
	}
	return s.page(ctx, cursor)
}
func (s savedDetails) LoadRecord(ctx context.Context, name string) (noticeledger.Record, error) {
	if s.load == nil {
		return noticeledger.Record{}, errors.New("unexpected LoadRecord")
	}
	return s.load(ctx, name)
}
func recordName(r noticeledger.Record) string {
	_, id := r.Identity()
	return fmt.Sprintf("notices/%x.json", sha256.Sum256([]byte(id)))
}
func detailSources(t *testing.T, ns []notices.Notice, rs []noticeledger.Record) (announcementDetails, savedDetails) {
	t.Helper()
	ids, names := []string{}, []string{}
	for _, n := range ns {
		ids = append(ids, n.Key().NoticeID)
	}
	for _, r := range rs {
		names = append(names, recordName(r))
	}
	ai, si := 0, 0
	a := announcementDetails{page: func(context.Context, string) (ociannouncements.Page, error) {
		return ociannouncements.Page{IDs: ids}, nil
	}}
	a.get = func(_ context.Context, id string) (notices.Notice, error) {
		if ai >= len(ns) || id != ns[ai].Key().NoticeID {
			t.Fatal("wrong Get order or retry")
		}
		n := ns[ai]
		ai++
		return n, nil
	}
	s := savedDetails{page: func(context.Context, string) (ocistorage.NoticePage, error) {
		return ocistorage.NoticePage{Names: names}, nil
	}}
	s.load = func(_ context.Context, name string) (noticeledger.Record, error) {
		if si >= len(rs) || name != names[si] {
			t.Fatal("wrong saved read order or retry")
		}
		r := rs[si]
		si++
		return r, nil
	}
	return a, s
}
func collector(t *testing.T, a AnnouncementSource, s SavedNoticeSource) *Collector {
	t.Helper()
	c, err := NewCollector([]byte(candidateConfig), candidateCatalog(), candidateLocation(), a, s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func wantCollectionFailure(t *testing.T, got Collection, err error, code Code, source Source, status int) {
	t.Helper()
	var failure *CollectionError
	if !errors.As(err, &failure) || failure.Code != code || failure.Source != source || failure.HTTPStatus != status {
		t.Fatalf("collection error=%+v; want %s/%s/%d", err, code, source, status)
	}
	if got.Current != nil || got.Saved != nil || got.Observations != nil {
		t.Fatal("partial collection on failure")
	}
	if got.Candidates.EnvironmentID() != "" || got.Candidates.Current() != nil || got.Candidates.Components() != nil {
		t.Fatal("partial candidate view on failure")
	}
}
