package delivery_test

import (
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestSelectionIsExplicitAndAtomic(t *testing.T) {
	b, n := builder(t, nil), notice(t, record(db))
	lookups := map[string]routing.Lookup{db: metadata(db, "payments")}
	got := build(t, b, n, lookups)
	if len(got.Drafts) != 0 || len(got.Issues) != 0 {
		t.Fatal("empty selection became all resources")
	}
	for _, tc := range []struct {
		ids  []string
		code string
	}{
		{[]string{db, db}, "duplicate"},
		{[]string{db, "private-unknown-id"}, "not_ready"},
		{[]string{""}, "not_ready"},
		{make([]string, 257), "too_large"},
		{[]string{strings.Repeat("x", (1<<20)+1)}, "too_large"},
	} {
		got, err := b.Build(n, false, lookups, tc.ids)
		wantError(t, got, err, "selection", tc.code)
	}
}

func TestSelectionCannotBypassScopeOrOwnership(t *testing.T) {
	for _, tc := range []struct {
		id     string
		lookup routing.Lookup
	}{
		{db, routing.Lookup{State: routing.NotFound}},
		{db, metadata(db, "unmapped")},
		{"ocid1.mysqldbsystem.oc1.phx.other", metadata("ocid1.mysqldbsystem.oc1.phx.other", "payments")},
		{"private-malformed", metadata("private-malformed", "payments")},
	} {
		got, err := builder(t, nil).Build(notice(t, record(tc.id)), false, map[string]routing.Lookup{tc.id: tc.lookup}, []string{tc.id})
		wantError(t, got, err, "selection", "not_ready")
	}
}

func TestZeroBuilderAndUnsupportedNoticeCannotBuild(t *testing.T) {
	n := notice(t, record(db))
	for _, b := range []*delivery.Builder{nil, {}} {
		got, err := b.Build(n, false, nil, nil)
		wantError(t, got, err, "builder", "invalid")
	}
	b := builder(t, nil)
	got, err := b.Build(notices.Notice{}, false, nil, nil)
	wantError(t, got, err, "notice", "unsupported")
	for _, change := range []func(*notices.Record){
		func(r *notices.Record) { r.AnnouncementType = "FUTURE_UPDATE" },
		func(r *notices.Record) { r.Platform = nil },
		func(r *notices.Record) { r.Services = nil },
		func(r *notices.Record) { r.Services = nil; r.Resources = nil },
	} {
		r := record(db)
		change(&r)
		for _, known := range []bool{false, true} {
			got, err := b.Build(notice(t, r), known, map[string]routing.Lookup{db: metadata(db, "payments")}, []string{db})
			wantError(t, got, err, "notice", "unsupported")
		}
	}
}

func wantError(t *testing.T, got delivery.Result, err error, field, code string) {
	t.Helper()
	var detail *delivery.Error
	if !errors.As(err, &detail) || detail.Field != field || detail.Code != code || err.Error() != field+": "+code || len(got.Drafts) != 0 || len(got.Issues) != 0 {
		t.Fatalf("expected controlled atomic failure %s/%s, got %+v %v", field, code, got, err)
	}
}
