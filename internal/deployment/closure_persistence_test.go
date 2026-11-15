package deployment

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

type closureFaultStore struct {
	*operationalStore
	writes  int
	lostAt  int
	persist bool
	parents map[string]bool
}

func (s *closureFaultStore) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	if !strings.HasPrefix(name, "notices/") {
		return s.operationalStore.Replace(ctx, name, tag, body)
	}
	s.writes++
	if s.parents[tag] {
		panic("reused notice predecessor")
	}
	s.parents[tag] = true
	if s.writes == s.lostAt && !s.persist {
		return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
	}
	next, err := s.operationalStore.Replace(ctx, name, tag, body)
	if err == nil && s.writes == s.lostAt {
		return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
	}
	return next, err
}

func TestClosureEntryPointConfirmsEachLegacyTransitionBeforeCompletion(t *testing.T) {
	for _, lostAt := range []int{1, 2, 3} {
		f := newSavedReviewFixture(t, "ocid1.announcement.oc1..legacy-persistence", 1, false)
		for name, obj := range f.objects.objects {
			if !strings.HasPrefix(name, "notices/") {
				continue
			}
			body := string(obj.Body)
			start := strings.Index(body, `,"sourceOutcome":`)
			end := strings.Index(body, `,"delivery":`)
			obj.Body = []byte(strings.Replace(body[:start]+body[end:], `"schemaVersion":2`, `"schemaVersion":1`, 1))
			f.objects.objects[name] = obj
		}
		intent := reviewedClosureIntent(t, f, "c1-persistence")
		applyClosureIntent(t, &f, intent)
		fault := &closureFaultStore{operationalStore: f.objects, lostAt: lostAt, persist: true, parents: map[string]bool{}}
		f.h.connect = func(Settings) (Store, error) { return fault, nil }
		got := invokeOperational(t, f.h, closeInvocation(f.s, "c1-persistence"))
		if got.Status != "closure-completed" || fault.writes != 3 {
			t.Fatal("migration/action confirmation failed", lostAt, got, fault.writes)
		}
		got = invokeOperational(t, f.h, closeInvocation(f.s, "c1-persistence"))
		if got.Status != "closure-completed" || fault.writes != 3 {
			t.Fatal("retry duplicated durable action", got)
		}
	}
}

func TestClosureEntryPointCannotPromoteUnconfirmedAction(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..unconfirmed", 1, false)
	intent := reviewedClosureIntent(t, f, "c1-persistence")
	applyClosureIntent(t, &f, intent)
	fault := &closureFaultStore{operationalStore: f.objects, lostAt: 2, persist: false, parents: map[string]bool{}}
	f.h.connect = func(Settings) (Store, error) { return fault, nil }
	got := invokeOperational(t, f.h, closeInvocation(f.s, "c1-persistence"))
	if got.Status != "not-ready" || got.Recovery != nil || fault.writes != 2 {
		t.Fatal("uncertain action promoted", got)
	}
}
