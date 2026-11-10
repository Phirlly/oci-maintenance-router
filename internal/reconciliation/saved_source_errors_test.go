package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
)

func TestSavedSourceAllowedServiceFailuresPreserveUnavailableEvidence(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	facts := candidates(t)
	if e := facts.Restore(r); e != nil {
		t.Fatal(e)
	}
	membership := candidateView(t, facts).Components()
	for _, status := range []int{401, 403, 404, 408, 429, 500, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			a, s := detailSources(t, nil, []noticeledger.Record{r, r})
			calls := 0
			a.get = func(_ context.Context, id string) (notices.Notice, error) {
				calls++
				if id != n.Key().NoticeID {
					t.Fatal("wrong ID")
				}
				return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: status}
			}
			got, e := collector(t, a, s).collect(budget(t), noPause)
			want := []UnavailableNotice{{NoticeID: n.Key().NoticeID, HTTPStatus: status}}
			if e != nil || calls != 1 || got.Availability.EnvironmentID() != got.Candidates.EnvironmentID() || !reflect.DeepEqual(got.Availability.Unavailable(), want) || len(got.Current) != 0 || len(got.Observations) != 0 || !reflect.DeepEqual(membership, got.Candidates.Components()) {
				t.Fatal("failure fabricated source or lost bounded evidence", e)
			}
		})
	}
}

func TestSavedSourceRejectsUnclassifiedAndAmbiguousErrors(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	var typedNil *ociannouncements.Error
	cases := []struct {
		name    string
		err     error
		status  int
		partial bool
	}{
		{"unknown", errors.New("private detail"), 0, false}, {"typed nil", typedNil, 0, false},
		{"wrapped", fmt.Errorf("private: %w", &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}), 404, false},
		{"joined", errors.Join(&ociannouncements.Error{Code: ociannouncements.Service, Status: 404}, context.DeadlineExceeded), 404, false},
		{"partial", &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}, 404, true},
	}
	for _, code := range []ociannouncements.Code{ociannouncements.Authentication, ociannouncements.Transport, ociannouncements.DeadlineExceeded, ociannouncements.Canceled, ociannouncements.InvalidResponse, ociannouncements.TooLarge, ociannouncements.InvalidConfiguration, ociannouncements.InvalidRequest, ociannouncements.DeadlineRequired, "unknown"} {
		cases = append(cases, struct {
			name    string
			err     error
			status  int
			partial bool
		}{string(code), &ociannouncements.Error{Code: code, Status: 404}, 404, false})
	}
	for _, status := range []int{0, 99, 200, 302, 400, 409, 410, 418, 501, 505, 599, 600} {
		safe := status
		if status < 100 || status > 599 {
			safe = 0
		}
		cases = append(cases, struct {
			name    string
			err     error
			status  int
			partial bool
		}{fmt.Sprint(status), &ociannouncements.Error{Code: ociannouncements.Service, Status: status}, safe, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, s := detailSources(t, nil, []noticeledger.Record{r})
			a.get = func(context.Context, string) (notices.Notice, error) {
				if tc.partial {
					return n, tc.err
				}
				return notices.Notice{}, tc.err
			}
			got, e := collector(t, a, s).collect(budget(t), noPause)
			wantCollectionFailure(t, got, e, ReadFailed, Announcements, tc.status)
		})
	}
}

func TestSavedSourceLateInvalidIdentityDiscardsSuccessAndUnavailable(t *testing.T) {
	ns := []notices.Notice{candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true), candidateNotice(t, 3, nil, true)}
	for _, wrong := range []notices.Notice{{}, candidateNotice(t, 99, nil, true)} {
		rs := []noticeledger.Record{}
		for _, n := range ns {
			rs = append(rs, candidateRecord(t, n))
		}
		a, s := detailSources(t, nil, rs)
		calls := 0
		a.get = func(context.Context, string) (notices.Notice, error) {
			calls++
			switch calls {
			case 1:
				return ns[0], nil
			case 2:
				return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
			default:
				return wrong, nil
			}
		}
		got, e := collector(t, a, s).collect(budget(t), noPause)
		wantCollectionFailure(t, got, e, InvalidNotice, Announcements, 0)
		if calls != 3 {
			t.Fatal("wrong failure boundary")
		}
	}
}
