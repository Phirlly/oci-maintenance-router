package reconciliation

import (
	"slices"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestCandidateChainsReachSeedsRegardlessOfOrder(t *testing.T) {
	ns := []notices.Notice{
		candidateNotice(t, 3, chain("second"), false),
		candidateNotice(t, 2, chain("second"), false),
		candidateNotice(t, 2, chain("first"), false),
		candidateNotice(t, 1, chain("first"), true),
	}
	for _, reverse := range []bool{false, true} {
		if reverse {
			slices.Reverse(ns)
		}
		c := candidates(t)
		observeCandidates(t, c, ns...)
		requireMatches(t, c, ns, true, true, true, true)
	}
}

func TestCandidateChainsKeepExactSeparateNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name         string
		seed, update *string
		want         bool
	}{
		{"nil", nil, nil, false},
		{"empty", chain(""), chain(""), false},
		{"nil-empty", nil, chain(""), false},
		{"same", chain("chain"), chain("chain"), true},
		{"case", chain("Chain"), chain("chain"), false},
		{"trailing-space", chain("chain "), chain("chain"), false},
		{"whitespace-is-exact", chain(" "), chain(" "), true},
		{"unicode", chain("é"), chain("e\u0301"), false},
		{"id-is-not-chain", nil, chain(noticeID(1)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := candidates(t)
			ns := []notices.Notice{candidateNotice(t, 1, tc.seed, true), candidateNotice(t, 2, tc.update, false)}
			observeCandidates(t, c, ns...)
			requireMatches(t, c, ns, true, tc.want)
		})
	}
}

func TestSameNoticeRemainsCandidateAcrossChangedChainAndLabels(t *testing.T) {
	c := candidates(t)
	ns := []notices.Notice{candidateNotice(t, 1, nil, false), candidateNotice(t, 1, chain("new"), true)}
	observeCandidates(t, c, ns...)
	requireMatches(t, c, ns, true, true)
}
