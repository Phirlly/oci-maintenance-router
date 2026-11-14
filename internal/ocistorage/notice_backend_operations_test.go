package ocistorage

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

var noticeOperations = []string{"load", "load-object", "create", "save", "list"}

// Setup uses a genuine confirmed handle before installing the failure under test.
func backendOperation(t *testing.T, b *noticeBackend, operation string) func(context.Context) error {
	t.Helper()
	r := noticeState(t)
	env, id := r.Identity()
	b.create = func(context.Context, string, []byte) (string, error) { return "initial", nil }
	s, err := NewNoticeStore(b, env)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.Create(budget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	next := holdState(t, old.Record(), delivery.ReasonOperator)
	return func(ctx context.Context) error {
		var got StoredNotice
		var err error
		switch operation {
		case "load":
			got, err = s.Load(ctx, id)
		case "load-object":
			got, err = s.LoadObject(ctx, noticeObjectName(id))
		case "create":
			got, err = s.Create(ctx, r)
		case "save":
			got, err = s.Save(ctx, old, next)
		case "list":
			var page NoticePage
			page, err = s.ListPage(ctx, "")
			if err != nil {
				emptyPage(t, page)
			}
		default:
			t.Fatal("unknown operation")
		}
		if err != nil && (got.store != nil || got.etag != "" || got.Record().Snapshot().NoticeID != "") {
			t.Fatal("failed operation returned a handle")
		}
		return err
	}
}

func backendResponse(t *testing.T, b *noticeBackend, callback func(context.Context) error) {
	t.Helper()
	body, _ := noticeState(t).Encode()
	b.get = func(ctx context.Context, _ string) (Object, error) { return Object{body, "current"}, callback(ctx) }
	b.create = func(ctx context.Context, _ string, _ []byte) (string, error) { return "current", callback(ctx) }
	b.replace = func(ctx context.Context, _, _ string, _ []byte) (string, error) { return "current", callback(ctx) }
	b.list = func(ctx context.Context, _ string) (NoticePage, error) {
		return NoticePage{[]string{listedName(1)}, listedName(2)}, callback(ctx)
	}
}
