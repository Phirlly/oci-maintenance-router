package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

type announcementPages func(context.Context, string) (ociannouncements.Page, error)

func (f announcementPages) ListPage(ctx context.Context, cursor string) (ociannouncements.Page, error) {
	return f(ctx, cursor)
}

type savedPages func(context.Context, string) (ocistorage.NoticePage, error)

func (f savedPages) ListPage(ctx context.Context, cursor string) (ocistorage.NoticePage, error) {
	return f(ctx, cursor)
}

func emptyAnnouncements(context.Context, string) (ociannouncements.Page, error) {
	return ociannouncements.Page{}, nil
}
func emptySaved(context.Context, string) (ocistorage.NoticePage, error) {
	return ocistorage.NoticePage{}, nil
}
func noPause(context.Context, time.Duration) error { return nil }
func budget(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func noticeID(i int) string   { return fmt.Sprintf("ocid1.announcement.oc1..example%d", i) }
func objectName(i int) string { return fmt.Sprintf("notices/%064x.json", i) }

func wantFailure(t *testing.T, result Inventory, err error, code Code, source Source, status int) {
	t.Helper()
	var got *Error
	if !errors.As(err, &got) || got.Code != code || got.Source != source || got.HTTPStatus != status {
		t.Fatalf("wrong failure: %+v; want %s/%s/%d", err, code, source, status)
	}
	if result.AnnouncementIDs != nil || result.NoticeObjects != nil {
		t.Fatal("failure returned partial inventory")
	}
}
