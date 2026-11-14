package ocistorage

import (
	"bytes"
	"context"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

// Each operation must be explicitly provided; unexpected I/O fails immediately.
type noticeBackend struct {
	get     func(context.Context, string) (Object, error)
	create  func(context.Context, string, []byte) (string, error)
	replace func(context.Context, string, string, []byte) (string, error)
	list    func(context.Context, string) (NoticePage, error)
}

func (b *noticeBackend) Get(ctx context.Context, name string) (Object, error) {
	return b.get(ctx, name)
}
func (b *noticeBackend) Create(ctx context.Context, name string, body []byte) (string, error) {
	return b.create(ctx, name, body)
}
func (b *noticeBackend) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	return b.replace(ctx, name, tag, body)
}
func (b *noticeBackend) ListNoticePage(ctx context.Context, start string) (NoticePage, error) {
	return b.list(ctx, start)
}

func TestNoticeBackendPersistsRealHandles(t *testing.T) {
	r := noticeState(t)
	env, id := r.Identity()
	want, _ := r.Encode()
	name := noticeObjectName(id)
	b := &noticeBackend{}
	b.create = func(ctx context.Context, got string, body []byte) (string, error) {
		if got != name || !bytes.Equal(body, want) {
			t.Fatal("create did not preserve the exact path and canonical record")
		}
		return `"created"`, nil
	}
	b.get = func(context.Context, string) (Object, error) { return Object{want, `"loaded"`}, nil }
	s, err := NewNoticeStore(b, env)
	if err != nil {
		t.Fatal("a conditional object backend must support real stored handles", err)
	}
	created, err := s.Create(budget(t), r)
	if err != nil || created.store != s || created.etag != `"created"` {
		t.Fatal("create did not confirm its version", err)
	}
	for _, load := range []func() (StoredNotice, error){
		func() (StoredNotice, error) { return s.Load(budget(t), id) },
		func() (StoredNotice, error) { return s.LoadObject(budget(t), name) },
	} {
		loaded, err := load()
		if err != nil || loaded.store != s || loaded.etag != `"loaded"` {
			t.Fatal("load did not bind its own version", err)
		}
		next := holdState(t, loaded.Record(), delivery.ReasonOperator)
		nextBody, _ := next.Encode()
		b.replace = func(ctx context.Context, got, tag string, body []byte) (string, error) {
			if got != name || tag != `"loaded"` || !bytes.Equal(body, nextBody) {
				t.Fatal("replace lost exact path, precondition or immediate transition")
			}
			return "saved", nil
		}
		saved, err := s.Save(budget(t), loaded, next)
		if err != nil || saved.etag != "saved" || saved.Record().Snapshot().HoldReason != delivery.ReasonOperator {
			t.Fatal("conditional transition failed", err)
		}
	}
	b.list = func(ctx context.Context, start string) (NoticePage, error) {
		if start != name {
			t.Fatal("continuation changed")
		}
		return NoticePage{[]string{name}, name}, nil
	}
	page, err := s.ListPage(budget(t), name)
	if err != nil || len(page.Names) != 1 || page.Names[0] != name || page.NextStartWith != name {
		t.Fatal("listing failed", err)
	}
}
