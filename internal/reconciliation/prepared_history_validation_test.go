package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparedHistoryStoreRejectsInvalidBindingBeforeIO(t *testing.T) {
	_, objects, first, _ := preparedStoreFixture(t)
	var nilBackend *preparedBackend
	for _, backend := range []PreparedHistoryObjects{nil, nilBackend} {
		if s, err := NewPreparedHistoryStore(backend, first.EnvironmentID(), "installation-1"); err == nil || s != nil {
			t.Fatal("nil backend admitted")
		}
	}
	for _, identity := range [][2]string{{"", "id"}, {first.EnvironmentID(), ""}, {first.EnvironmentID(), "../id"}, {first.EnvironmentID(), strings.Repeat("a", 129)}} {
		if s, err := NewPreparedHistoryStore(objects, identity[0], identity[1]); err == nil || s != nil {
			t.Fatal("invalid identity admitted")
		}
	}
	for _, s := range []*PreparedHistoryStore{nil, {}} {
		got, err := s.Load(leaseBudget(t))
		preparedStoredFailure(t, got, err, "invalid_request", 0, false)
		got, err = s.Create(leaseBudget(t))
		preparedStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(objects.calls) != 0 {
		t.Fatal("invalid binding dispatched")
	}
	s := preparedStoreFor(t, &ocistorage.Client{}, first.EnvironmentID(), "installation-1")
	got, err := s.Create(leaseBudget(t))
	preparedStoredFailure(t, got, err, "write_failed", 0, false)
}

func TestPreparedHistoryStoreRejectsInvalidReadAndOwnsBytes(t *testing.T) {
	_, _, first, _ := preparedStoreFixture(t)
	h, _ := NewPreparedHistory(first.EnvironmentID(), "installation-1")
	body, _ := h.Encode()
	for _, tt := range []struct {
		tag  string
		body []byte
		code string
	}{
		{"", body, "invalid_response"}, {"W/tag", body, "invalid_response"}, {"*", body, "invalid_response"}, {"a,b", body, "invalid_response"}, {"a\r\nb", body, "invalid_response"}, {`""`, body, "invalid_response"}, {"é", body, "invalid_response"}, {strings.Repeat("x", 1025), body, "invalid_response"},
		{"tag", []byte("private"), "invalid_record"}, {"tag", make([]byte, 4097), "invalid_record"}, {"tag", []byte(strings.Replace(string(body), "installation-1", "other", 1)), "invalid_record"}, {"tag", []byte(strings.Replace(string(body), first.EnvironmentID(), strings.Repeat("f", 64), 1)), "invalid_record"},
	} {
		calls := 0
		backend := &preparedBackend{get: func(context.Context, string) (ocistorage.Object, error) {
			calls++
			return ocistorage.Object{Body: tt.body, ETag: tt.tag}, nil
		}}
		s := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1")
		got, err := s.Load(leaseBudget(t))
		preparedStoredFailure(t, got, err, tt.code, 200, false)
		if calls != 1 {
			t.Fatal("bad read retried")
		}
	}
	backend := &preparedBackend{get: func(context.Context, string) (ocistorage.Object, error) {
		return ocistorage.Object{Body: body, ETag: `"valid-tag"`}, nil
	}}
	got, err := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1").Load(leaseBudget(t))
	clear(body)
	if err != nil || got.History().EnvironmentID() != first.EnvironmentID() {
		t.Fatal("read did not own record", err)
	}
}
