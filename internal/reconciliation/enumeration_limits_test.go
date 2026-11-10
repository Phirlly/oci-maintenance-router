package reconciliation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func sourceSequence(ids []string) announcementPages {
	i := 0
	return func(context.Context, string) (ociannouncements.Page, error) {
		end := min(i+100, len(ids))
		p := ociannouncements.Page{IDs: ids[i:end]}
		i = end
		if i < len(ids) {
			p.NextPage = fmt.Sprintf("page%d", i)
		}
		return p, nil
	}
}

func TestOccurrenceCapCountsBothSourcesAndDuplicates(t *testing.T) {
	for _, extra := range []int{0, 1} {
		ids := make([]string, 9000)
		for i := range ids {
			ids[i] = noticeID(1)
		}
		calls := 0
		s := savedPages(func(context.Context, string) (ocistorage.NoticePage, error) {
			calls++
			if calls == 2 {
				return ocistorage.NoticePage{Names: []string{objectName(1)}}, nil
			}
			names := make([]string, 1000)
			for i := range names {
				names[i] = objectName(i)
			}
			p := ocistorage.NoticePage{Names: names}
			if extra > 0 {
				p.NextStartWith = objectName(1000)
			}
			return p, nil
		})
		got, err := enumerate(budget(t), sourceSequence(ids), s, noPause)
		if extra == 0 {
			if err != nil || len(got.AnnouncementIDs) != 9000 || len(got.NoticeObjects) != 1000 {
				t.Fatal("exact occurrence cap rejected", err)
			}
		} else {
			wantFailure(t, got, err, ItemLimit, SavedNotices, 0)
		}
		if calls != 1+extra {
			t.Fatal("unexpected extra call", calls)
		}
	}
}

func TestTextCapCountsIdentifiersAndUTF8CursorBytesAcrossSources(t *testing.T) {
	for _, extra := range []int{0, 1} {
		// 1024 maximum-length source IDs fill 4MiB. Make room for ten
		// distinct two-byte UTF8 cursors and one 77-byte stored object name.
		id := noticeID(1) + strings.Repeat("x", 4096-len(noticeID(1)))
		ids := make([]string, 1024)
		for i := range ids {
			ids[i] = id
		}
		ids[1023] = id[:4096-20-77+extra]
		i, page := 0, 0
		a := announcementPages(func(context.Context, string) (ociannouncements.Page, error) {
			end := min(i+100, len(ids))
			p := ociannouncements.Page{IDs: ids[i:end]}
			i, page = end, page+1
			if i < len(ids) {
				p.NextPage = string(rune('é' + page))
			}
			return p, nil
		})
		s := savedPages(func(context.Context, string) (ocistorage.NoticePage, error) {
			return ocistorage.NoticePage{Names: []string{objectName(1)}}, nil
		})
		got, err := enumerate(budget(t), a, s, noPause)
		if extra == 0 {
			if err != nil || len(got.AnnouncementIDs) != 1024 || len(got.NoticeObjects) != 1 {
				t.Fatal("exact text cap rejected", err)
			}
		} else {
			wantFailure(t, got, err, ByteLimit, SavedNotices, 0)
		}
	}
}

func TestInvalidPageStructureNeverReturnsEarlierWork(t *testing.T) {
	for _, source := range []Source{Announcements, SavedNotices} {
		cap, textCap := 100, 4096
		if source == SavedNotices {
			cap, textCap = 1000, 77
		}
		for _, kind := range []string{"entry_count", "empty_entry", "entry_bytes", "cursor_bytes"} {
			t.Run(string(source)+"/"+kind, func(t *testing.T) {
				ids, next := []string{noticeID(1)}, ""
				switch kind {
				case "entry_count":
					ids = make([]string, cap+1)
				case "empty_entry":
					ids = []string{""}
				case "entry_bytes":
					ids = []string{strings.Repeat("x", textCap+1)}
				case "cursor_bytes":
					next = strings.Repeat("x", textCap+1)
				}
				a := announcementPages(func(context.Context, string) (ociannouncements.Page, error) {
					return ociannouncements.Page{IDs: []string{noticeID(1)}}, nil
				})
				s := savedPages(emptySaved)
				if source == Announcements {
					a = func(context.Context, string) (ociannouncements.Page, error) {
						return ociannouncements.Page{IDs: ids, NextPage: next}, nil
					}
				} else {
					s = func(context.Context, string) (ocistorage.NoticePage, error) {
						return ocistorage.NoticePage{Names: ids, NextStartWith: next}, nil
					}
				}
				got, err := enumerate(budget(t), a, s, noPause)
				wantFailure(t, got, err, InvalidPage, source, 0)
			})
		}
	}
}
