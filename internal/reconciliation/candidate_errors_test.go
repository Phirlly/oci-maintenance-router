package reconciliation

import (
	"errors"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func candidateError(t *testing.T, err error, want Code) *CandidateError {
	t.Helper()
	var got *CandidateError
	if !errors.As(err, &got) || got.Code != want || err.Error() != "candidate relevance: "+string(want) {
		t.Fatalf("candidate error=%v; want %s", err, want)
	}
	return got
}

func TestCandidateInvalidAdmissionIsPermanent(t *testing.T) {
	n := candidateNotice(t, 0, nil, true)
	r := candidateRecord(t, n)
	for _, tc := range []struct {
		name string
		code Code
		fail func(*Candidates) error
	}{
		{"notice", InvalidNotice, func(c *Candidates) error { return c.Observe(notices.Notice{}) }},
		{"record", InvalidRecord, func(c *Candidates) error { return c.Restore(noticeledger.Record{}) }},
		{"environment", EnvironmentMismatch, func(c *Candidates) error {
			other := candidateLocation()
			other.Bucket = "other"
			foreign, err := NewCandidates([]byte(candidateConfig), candidateCatalog(), other)
			if err != nil {
				t.Fatal(err)
			}
			*c = *foreign
			return c.Restore(r)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := candidates(t)
			observeCandidates(t, c, n)
			err := candidateError(t, tc.fail(c), tc.code)
			wantNoCandidateView(t, c, tc.code)
			for _, format := range []string{"%v", "%+v", "%#v"} {
				if len(fmt.Sprintf(format, err)) > 100 {
					t.Fatal("error contains uncontrolled data")
				}
			}
			err.Code = "caller-mutated"
			candidateError(t, c.Observe(n), tc.code)
			candidateError(t, c.Restore(r), tc.code)
			got, failure := c.Resolve()
			candidateError(t, failure, tc.code)
			if got != nil {
				t.Fatal("failed accumulator returned partial matches")
			}
		})
	}
}

func TestCandidatesRejectInvalidReceiversAndConfiguration(t *testing.T) {
	for _, c := range []*Candidates{nil, {}} {
		candidateError(t, c.Observe(notices.Notice{}), InvalidRequest)
		candidateError(t, c.Restore(noticeledger.Record{}), InvalidRequest)
		got, err := c.Resolve()
		candidateError(t, err, InvalidRequest)
		if got != nil {
			t.Fatal("invalid receiver returned matches")
		}
	}
	if c, err := NewCandidates([]byte(`{}`), candidateCatalog(), candidateLocation()); c != nil || err == nil {
		t.Fatal("accepted invalid configuration")
	}
	location := candidateLocation()
	location.Namespace = ""
	if c, err := NewCandidates([]byte(candidateConfig), candidateCatalog(), location); c != nil || err == nil {
		t.Fatal("accepted invalid ledger location")
	}
	catalog := candidateCatalog()
	catalog.Regions["us-ashburn-1"] = "wrong"
	if c, err := NewCandidates([]byte(candidateConfig), catalog, candidateLocation()); c != nil || err == nil {
		t.Fatal("accepted untrusted region configuration")
	}
}
