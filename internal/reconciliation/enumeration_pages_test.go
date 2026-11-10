package reconciliation

import (
	"context"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestCyclesStopBeforeRepeatedRequest(t *testing.T) {
	for _, source := range []Source{Announcements, SavedNotices} {
		for _, cycleLength := range []int{1, 3} {
			t.Run(fmt.Sprintf("%s/%d", source, cycleLength), func(t *testing.T) {
				calls := 0
				a := announcementPages(emptyAnnouncements)
				s := savedPages(emptySaved)
				next := func() string { calls++; return objectName((calls-1)%cycleLength + 1) }
				if source == Announcements {
					a = func(context.Context, string) (ociannouncements.Page, error) {
						return ociannouncements.Page{IDs: []string{noticeID(1)}, NextPage: next()}, nil
					}
				} else {
					s = func(context.Context, string) (ocistorage.NoticePage, error) {
						return ocistorage.NoticePage{Names: []string{objectName(1)}, NextStartWith: next()}, nil
					}
				}
				got, err := enumerate(budget(t), a, s, noPause)
				wantFailure(t, got, err, CursorCycle, source, 0)
				if calls != cycleLength+1 {
					t.Fatal("reissued cursor or stopped too soon", calls)
				}
			})
		}
	}
}

func TestHundredthPageBoundaryForEachSource(t *testing.T) {
	for _, source := range []Source{Announcements, SavedNotices} {
		for _, terminal := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/terminal=%t", source, terminal), func(t *testing.T) {
				calls := 0
				next := func() string {
					calls++
					if terminal && calls == 100 {
						return ""
					}
					return objectName(calls)
				}
				a := announcementPages(emptyAnnouncements)
				s := savedPages(emptySaved)
				if source == Announcements {
					a = func(context.Context, string) (ociannouncements.Page, error) {
						return ociannouncements.Page{NextPage: next()}, nil
					}
				} else {
					s = func(context.Context, string) (ocistorage.NoticePage, error) {
						return ocistorage.NoticePage{NextStartWith: next()}, nil
					}
				}
				got, err := enumerate(budget(t), a, s, noPause)
				if terminal {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					wantFailure(t, got, err, PageLimit, source, 0)
				}
				if calls != 100 {
					t.Fatal("wrong page cap", calls)
				}
			})
		}
	}
}
