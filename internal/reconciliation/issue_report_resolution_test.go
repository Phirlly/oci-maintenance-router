package reconciliation

import (
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestIssueReportMissingLegacySourceEvidenceDoesNotResolve(t *testing.T) {
	i := reportIssue{Kind: "SOURCE", Code: "source_unknown", ResourceIndex: -1}
	snapshot := noticeledger.Snapshot{Revisions: []noticeledger.Revision{{Resources: []noticeledger.Resource{{Disposition: "READY"}}}}}
	if reportIssueResolved(i, snapshot, noticeledger.SavedReview{}, true) {
		t.Fatal("missing source outcome was treated as positive resolution")
	}
}

func TestIssueReportBatchRequiresPositiveResolution(t *testing.T) {
	for _, state := range []delivery.State{delivery.Prepared, delivery.Sending, delivery.RetryWait, delivery.ReviewRequired, delivery.APIAccepted, delivery.Closed} {
		t.Run(string(state), func(t *testing.T) {
			batch := delivery.BatchSnapshot{State: state}
			batch.Message.BatchID = "batch"
			snapshot := noticeledger.Snapshot{Batches: []delivery.BatchSnapshot{batch}}
			got := reportIssueResolved(reportIssue{Kind: "BATCH", BatchID: "batch"}, snapshot, noticeledger.SavedReview{}, true)
			if want := state == delivery.APIAccepted || state == delivery.Closed; got != want {
				t.Fatalf("resolved=%v for %s; want %v", got, state, want)
			}
		})
	}
}
