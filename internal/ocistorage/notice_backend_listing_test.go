package ocistorage

import (
	"context"
	"testing"
)

func TestNoticeBackendListingValidatesAndOwnsPage(t *testing.T) {
	for _, tt := range []struct {
		name string
		page NoticePage
		code Code
	}{
		{"count", NoticePage{Names: make([]string, 1001)}, TooLarge},
		{"path", NoticePage{Names: []string{"configuration/router.json"}}, InvalidResponse},
		{"duplicate", NoticePage{Names: []string{listedName(1), listedName(1)}}, InvalidResponse},
		{"cursor", NoticePage{NextStartWith: "notices/x.json"}, InvalidResponse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &noticeBackend{}
			invoke := backendOperation(t, b, "list")
			b.list = func(context.Context, string) (NoticePage, error) { return tt.page, nil }
			wantError(t, invoke(budget(t)), tt.code, 200, false)
		})
	}
	r := noticeState(t)
	env, _ := r.Identity()
	page := NoticePage{[]string{listedName(1)}, listedName(2)}
	b := &noticeBackend{list: func(context.Context, string) (NoticePage, error) { return page, nil }}
	s, err := NewNoticeStore(b, env)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.ListPage(budget(t), "")
	if err != nil {
		t.Fatal(err)
	}
	first.Names[0] = listedName(3)
	if page.Names[0] != listedName(1) {
		t.Fatal("returned page can mutate backend state")
	}
	second, err := s.ListPage(budget(t), "")
	if err != nil {
		t.Fatal(err)
	}
	page.Names[0] = listedName(4)
	if second.Names[0] != listedName(1) {
		t.Fatal("backend can mutate confirmed page")
	}
}

func TestClientNoticeListingRejectsUnusableClient(t *testing.T) {
	for _, c := range []*Client{nil, {}} {
		page, err := c.ListNoticePage(budget(t), "")
		wantError(t, err, InvalidRequest, 0, false)
		emptyPage(t, page)
	}
}
