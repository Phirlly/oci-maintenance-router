package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestDiscoveryReloadRejectsBothAddedAndLostMembership(t *testing.T) {
	a := candidateNotice(t, 1, chain("old"), true)
	src := a.Snapshot()
	src.Summary = chain("changed")
	b := normalizeCandidate(t, src)
	for _, loss := range []bool{false, true} {
		t.Run(map[bool]string{false: "added", true: "lost-hidden-in-union"}[loss], func(t *testing.T) {
			initial, replacement := candidateRecord(t, a), candidateRecord(t, a, b)
			if loss {
				initial, replacement = replacement, initial
			}
			d, s, lease := discoveryFixture(t, []notices.Notice{b}, []noticeledger.Record{initial})
			reads := 0
			s.before = func(_ context.Context, call discoveryCall) error {
				if call.method == "GET" && call.name == recordName(initial) {
					reads++
					if reads == 2 {
						body, err := replacement.Encode()
						if err != nil {
							t.Fatal(err)
						}
						s.seed(call.name, body)
					}
				}
				return nil
			}
			got, err := d.Run(leaseBudget(t), lease)
			discoveryFailure(t, got, err, "membership_changed")
			if len(s.noticeWrites()) != 0 {
				t.Fatal("changed saved membership was overwritten")
			}
		})
	}
}

func TestDiscoveryRejectsConflictingDuplicateSavedMembershipBeforeWrites(t *testing.T) {
	a := candidateNotice(t, 1, nil, true)
	src := a.Snapshot()
	src.Summary = chain("changed")
	b := normalizeCandidate(t, src)
	initial, replacement := candidateRecord(t, a), candidateRecord(t, a, b)
	d, s, lease := discoveryFixture(t, []notices.Notice{a}, []noticeledger.Record{initial})
	s.page = func(_ context.Context, start string) (ocistorage.NoticePage, error) {
		if start == "" {
			return ocistorage.NoticePage{Names: []string{recordName(initial)}, NextStartWith: recordName(initial)}, nil
		}
		return ocistorage.NoticePage{Names: []string{recordName(initial)}}, nil
	}
	reads := 0
	s.before = func(_ context.Context, call discoveryCall) error {
		if call.method == "GET" && call.name == recordName(initial) {
			reads++
			if reads == 2 {
				body, err := replacement.Encode()
				if err != nil {
					t.Fatal(err)
				}
				s.seed(call.name, body)
			}
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "membership_changed")
	if len(s.noticeWrites()) != 0 {
		t.Fatal("ambiguous collection wrote state")
	}
}
