package reconciliation

import (
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestIssueReportResolutionRequiresEvidenceForItsOwnSubject(t *testing.T) {
	for _, tc := range []struct {
		name, kind  string
		known, want bool
	}{
		{"notice-store", "STORE", false, true}, {"preparation-store", "STORE", false, false}, {"preparation-store", "STORE", true, true},
		{"missing-batch", "BATCH", true, false}, {"notice-present", "NOTICE", true, false}, {"notice-resolved", "NOTICE", true, true},
		{"assessment-present", "ASSESSMENT", true, false}, {"assessment-resolved", "ASSESSMENT", true, true},
		{"closed-revision", "RESOURCE", true, true}, {"other-revision", "RESOURCE", true, false}, {"other-row", "RESOURCE", true, false},
		{"source-available", "SOURCE", true, true}, {"source-empty-revision", "SOURCE", true, false}, {"source-closed-revision", "SOURCE", true, true},
		{"source-sending", "SOURCE", true, false}, {"source-accepted", "SOURCE", true, true}, {"source-empty-history", "SOURCE", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := reportIssue{Kind: tc.kind, Revision: "revision", ResourceIndex: 0, BatchID: "batch"}
			s := noticeledger.Snapshot{Revisions: []noticeledger.Revision{{Digest: "revision", Resources: []noticeledger.Resource{{Index: 0, Disposition: "UNRESOLVED", Reason: "lookup_missing"}}}}}
			v := noticeledger.SavedReview{}
			switch tc.name {
			case "notice-store":
				i.Code = "notice_unreadable"
			case "preparation-store":
				i.Code = "preparation_unreadable"
			case "notice-present":
				s.HoldReason = delivery.ReasonSourceUnavailable
				v.Issues = []noticeledger.SavedReviewIssue{{Kind: "NOTICE"}}
			case "assessment-present":
				v.Issues = []noticeledger.SavedReviewIssue{{Kind: "ASSESSMENT", RevisionDigest: "revision"}}
			case "closed-revision", "source-closed-revision":
				s.Recovery = &noticeledger.RecoverySnapshot{Actions: []noticeledger.RecoveryAction{{Status: noticeledger.RecoveryClosed, Request: noticeledger.RecoveryRequest{Kind: noticeledger.CloseRevision, TargetID: "revision"}}}}
			case "other-revision":
				s.Revisions[0].Digest = "other"
			case "other-row":
				s.Revisions[0].Resources[0].Index = 1
			case "source-available":
				s.SourceOutcome = &noticeledger.SourceOutcome{State: noticeledger.SourceAvailable}
			case "source-empty-revision":
				s.Revisions[0].Resources = nil
			case "source-sending", "source-accepted":
				s.Revisions[0].Resources[0].BatchID = "batch"
				b := delivery.BatchSnapshot{State: delivery.Sending}
				if tc.name == "source-accepted" {
					b.State = delivery.APIAccepted
				}
				s.Batches = []delivery.BatchSnapshot{b}
			case "source-empty-history":
				s = noticeledger.Snapshot{}
			}
			if got := reportIssueResolved(i, s, v, tc.known); got != tc.want {
				t.Fatalf("resolved=%v want %v", got, tc.want)
			}
		})
	}
}
