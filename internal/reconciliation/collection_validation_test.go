package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestCollectorRejectsInvalidConstruction(t *testing.T) {
	a, s := announcementDetails{}, savedDetails{}
	for _, invalid := range []string{"config", "location", "announcements", "saved"} {
		data, location := []byte(candidateConfig), candidateLocation()
		var ap AnnouncementSource = a
		var sp SavedNoticeSource = s
		switch invalid {
		case "config":
			data = []byte(`{}`)
		case "location":
			location.Namespace = ""
		case "announcements":
			ap = nil
		case "saved":
			sp = nil
		}
		if c, err := NewCollector(data, candidateCatalog(), location, ap, sp); c != nil || err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
}

func TestCollectorRejectsInvalidInvocationWithoutReads(t *testing.T) {
	a := announcementDetails{page: func(context.Context, string) (ociannouncements.Page, error) {
		t.Fatal("unexpected read")
		return ociannouncements.Page{}, nil
	}}
	c := collector(t, a, savedDetails{})
	for _, receiver := range []*Collector{nil, {}} {
		got, err := receiver.Collect(budget(t))
		wantCollectionFailure(t, got, err, InvalidRequest, "", 0)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		got, err := c.Collect(ctx)
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		wantCollectionFailure(t, got, err, code, "", 0)
	}
	ctx, cancel := context.WithCancel(budget(t))
	cancel()
	got, err := c.Collect(ctx)
	wantCollectionFailure(t, got, err, Canceled, "", 0)
}

func TestCollectorHandlesTypedNilRealAdapters(t *testing.T) {
	var a *ociannouncements.Client
	var s *ocistorage.NoticeStore
	got, err := collector(t, a, savedDetails{}).collect(budget(t), noPause)
	wantCollectionFailure(t, got, err, ReadFailed, Announcements, 0)
	got, err = collector(t, announcementDetails{}, s).collect(budget(t), noPause)
	wantCollectionFailure(t, got, err, ReadFailed, SavedNotices, 0)
}
