package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestReadFailureDiscardsBothListsWithoutRetryOrRawError(t *testing.T) {
	private := errors.New("private provider URI and token")
	for _, source := range []Source{Announcements, SavedNotices} {
		for _, status := range []int{0, 99, 100, 403, 404, 429, 500, 599, 600} {
			calls := 0
			a := announcementPages(emptyAnnouncements)
			s := savedPages(emptySaved)
			var returned error = private
			if status != 0 {
				if source == Announcements {
					returned = fmt.Errorf("private wrapper: %w", &ociannouncements.Error{Code: "private", Status: status})
				} else {
					returned = fmt.Errorf("private wrapper: %w", &ocistorage.Error{Code: "private", Status: status})
				}
			}
			if source == Announcements {
				a = func(context.Context, string) (ociannouncements.Page, error) {
					calls++
					if calls == 1 {
						return ociannouncements.Page{IDs: []string{noticeID(1)}, NextPage: "second"}, nil
					}
					return ociannouncements.Page{IDs: []string{noticeID(2)}}, returned
				}
				s = func(context.Context, string) (ocistorage.NoticePage, error) {
					t.Fatal("continued after source failure")
					return ocistorage.NoticePage{}, nil
				}
			} else {
				a = func(context.Context, string) (ociannouncements.Page, error) {
					return ociannouncements.Page{IDs: []string{noticeID(1)}}, nil
				}
				s = func(context.Context, string) (ocistorage.NoticePage, error) {
					calls++
					if calls == 1 {
						return ocistorage.NoticePage{Names: []string{objectName(1)}, NextStartWith: objectName(2)}, nil
					}
					return ocistorage.NoticePage{Names: []string{objectName(2)}}, returned
				}
			}
			got, err := enumerate(budget(t), a, s, noPause)
			wantStatus := status
			if status < 100 || status > 599 {
				wantStatus = 0
			}
			wantFailure(t, got, err, ReadFailed, source, wantStatus)
			if calls != 2 || errors.Is(err, returned) || strings.Contains(fmt.Sprintf("%+v %#v", err, err), "private") {
				t.Fatal("retry or raw error exposure", err)
			}
		}
	}
}

func TestInvalidArgumentsAndTypedNilAdaptersDoNotPanic(t *testing.T) {
	for _, ctx := range []context.Context{nil, context.Background()} {
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		got, err := Enumerate(ctx, announcementPages(emptyAnnouncements), savedPages(emptySaved))
		wantFailure(t, got, err, code, "", 0)
	}
	got, err := Enumerate(budget(t), nil, savedPages(emptySaved))
	wantFailure(t, got, err, InvalidRequest, "", 0)
	got, err = Enumerate(budget(t), announcementPages(emptyAnnouncements), nil)
	wantFailure(t, got, err, InvalidRequest, "", 0)
	var a *ociannouncements.Client
	var s *ocistorage.NoticeStore
	got, err = enumerate(budget(t), a, savedPages(emptySaved), noPause)
	wantFailure(t, got, err, ReadFailed, Announcements, 0)
	got, err = enumerate(budget(t), announcementPages(emptyAnnouncements), s, noPause)
	wantFailure(t, got, err, ReadFailed, SavedNotices, 0)
}
