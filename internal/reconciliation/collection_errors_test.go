package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestCollectionReadErrorsDiscardEarlierWorkWithoutRetry(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	for _, source := range []Source{Announcements, SavedNotices} {
		for _, status := range []int{0, 99, 100, 401, 403, 404, 429, 500, 599, 600} {
			a, s := detailSources(t, []notices.Notice{n, n}, []noticeledger.Record{r, r})
			calls := 0
			var fault error = errors.New("private provider detail")
			if status != 0 {
				if source == Announcements {
					fault = fmt.Errorf("private: %w", &ociannouncements.Error{Status: status})
				} else {
					fault = fmt.Errorf("private: %w", &ocistorage.Error{Status: status})
				}
			}
			if source == Announcements {
				a.get = func(context.Context, string) (notices.Notice, error) {
					calls++
					if calls == 1 {
						return n, nil
					}
					return n, fault
				}
				s.load = func(context.Context, string) (noticeledger.Record, error) {
					t.Fatal("continued after failed Get")
					return r, nil
				}
			} else {
				s.load = func(context.Context, string) (noticeledger.Record, error) {
					calls++
					if calls == 1 {
						return r, nil
					}
					return r, fault
				}
			}
			got, err := collector(t, a, s).collect(budget(t), noPause)
			wantStatus := status
			if status < 100 || status > 599 {
				wantStatus = 0
			}
			wantCollectionFailure(t, got, err, ReadFailed, source, wantStatus)
			if calls != 2 || errors.Is(err, fault) || strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), "private") {
				t.Fatal("retry or provider error leaked")
			}
			if err.Error() != "collection: read_failed" {
				t.Fatal("wrong error owner")
			}
		}
	}
}

func TestCollectionRejectsInvalidDetails(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	for _, wrong := range []notices.Notice{{}, candidateNotice(t, 2, nil, true)} {
		a, s := detailSources(t, []notices.Notice{n}, nil)
		a.get = func(context.Context, string) (notices.Notice, error) { return wrong, nil }
		got, err := collector(t, a, s).collect(budget(t), noPause)
		wantCollectionFailure(t, got, err, InvalidNotice, Announcements, 0)
	}
	a, s := detailSources(t, nil, []noticeledger.Record{r})
	s.load = func(context.Context, string) (noticeledger.Record, error) { return noticeledger.Record{}, nil }
	got, err := collector(t, a, s).collect(budget(t), noPause)
	wantCollectionFailure(t, got, err, InvalidRecord, SavedNotices, 0)
	a, s = detailSources(t, nil, []noticeledger.Record{r})
	location := candidateLocation()
	location.Bucket = "other"
	c, err := NewCollector([]byte(candidateConfig), candidateCatalog(), location, a, s)
	if err != nil {
		t.Fatal(err)
	}
	got, err = c.collect(budget(t), noPause)
	wantCollectionFailure(t, got, err, EnvironmentMismatch, SavedNotices, 0)
}

func TestCollectionPropagatesListingFailure(t *testing.T) {
	for _, source := range []Source{Announcements, SavedNotices} {
		a, s := announcementDetails{}, savedDetails{}
		if source == Announcements {
			a.page = func(context.Context, string) (ociannouncements.Page, error) {
				return ociannouncements.Page{NextPage: "loop"}, nil
			}
		} else {
			s.page = func(context.Context, string) (ocistorage.NoticePage, error) {
				return ocistorage.NoticePage{}, &ocistorage.Error{Status: 403}
			}
		}
		got, err := collector(t, a, s).collect(budget(t), noPause)
		code, status := CursorCycle, 0
		if source == SavedNotices {
			code, status = ReadFailed, 403
		}
		wantCollectionFailure(t, got, err, code, source, status)
	}
}
