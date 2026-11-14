package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
	"testing"
)

func TestOperatorChallengeStoreRejectsInvalidConstructionAndRequests(t *testing.T) {
	s, objects, b := operatorStoreFixture(t)
	i := operatorIntentFixture(t)
	for _, tc := range []struct {
		objects      OperatorChallengeObjects
		env, install string
	}{{nil, i.EnvironmentID, i.InstallationID}, {(*operatorBackend)(nil), i.EnvironmentID, i.InstallationID}, {b, "wrong", i.InstallationID}, {b, i.EnvironmentID, "../other"}, {b, i.EnvironmentID, strings.Repeat("a", 129)}} {
		got, err := NewOperatorChallengeStore(tc.objects, tc.env, tc.install)
		if err == nil || got != nil {
			t.Fatal("invalid store constructed")
		}
	}
	before := len(objects.calls)
	for _, store := range []*OperatorChallengeStore{nil, {}, s} {
		for _, request := range []string{"", "../other", strings.Repeat("a", 129)} {
			got, err := store.Load(leaseBudget(t), request)
			operatorStoredFailure(t, got, err, "invalid_request", 0, false)
		}
		if store != s {
			got, err := store.Create(leaseBudget(t), operatorChallengeFixture(t))
			operatorStoredFailure(t, got, err, "invalid_request", 0, false)
		}
	}
	for _, field := range []string{"environment", "installation"} {
		other := i
		if field == "environment" {
			other.EnvironmentID = strings.Repeat("f", 64)
		} else {
			other.InstallationID = "other"
		}
		r, err := NewOperatorChallenge(other, operatorClaimFixture())
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Create(leaseBudget(t), r)
		operatorStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(objects.calls) != before {
		t.Fatal("invalid request reached backend")
	}
}

func TestOperatorChallengeStoreRejectsMalformedForeignAndOversizedReads(t *testing.T) {
	for _, kind := range []string{"corrupt", "oversize", "foreign", "bad-tag"} {
		t.Run(kind, func(t *testing.T) {
			s, objects, b := operatorStoreFixture(t)
			r := operatorChallengeFixture(t)
			if kind == "foreign" {
				i := operatorIntentFixture(t)
				i.RequestID = "other"
				r, _ = NewOperatorChallenge(i, operatorClaimFixture())
			}
			body, _ := r.Encode()
			tag := "v1"
			code := "invalid_record"
			switch kind {
			case "corrupt":
				body = []byte("private")
			case "oversize":
				body = make([]byte, operatorChallengeLimit+1)
			case "bad-tag":
				tag = "W/weak"
				code = "invalid_response"
			}
			b.get = func(context.Context, string) (ocistorage.Object, error) {
				return ocistorage.Object{Body: body, ETag: tag}, nil
			}
			b.history = func(context.Context, string) (bool, error) { t.Fatal("bad state treated as missing"); return true, nil }
			got, err := s.Load(leaseBudget(t), "operator-test-1")
			operatorStoredFailure(t, got, err, code, 200, false)
			got, err = s.Create(leaseBudget(t), operatorChallengeFixture(t))
			operatorStoredFailure(t, got, err, code, 200, false)
			if objects.version != 0 {
				t.Fatal("invalid current state overwritten")
			}
		})
	}
}

func TestOperatorChallengeMaximumKeyFitsBackendAndRemainsScoped(t *testing.T) {
	_, _, b := operatorStoreFixture(t)
	i := operatorIntentFixture(t)
	i.InstallationID = strings.Repeat("I", 128)
	i.RequestID = strings.Repeat("R", 128)
	s, err := NewOperatorChallengeStore(b, i.EnvironmentID, i.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewOperatorChallenge(i, operatorClaimFixture())
	if err != nil {
		t.Fatal(err)
	}
	var key string
	b.history = func(_ context.Context, n string) (bool, error) { key = n; return true, nil }
	got, err := s.Create(leaseBudget(t), r)
	if err != nil || len(key) > 1024 || key != "control/operator-challenges/"+i.InstallationID+"/"+i.RequestID+".json" || got.Record().Snapshot().Intent != i {
		t.Fatal("maximum valid identifiers failed", err)
	}
}
