package reconciliation

import (
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func (r *RecoveryReviewer) reviewEligibility(record noticeledger.Record, n notices.Notice, lookups map[string]routing.Lookup, plan routing.Plan, kind noticeledger.ActionKind, target string) string {
	if _, reason := recoveryTarget(record, noticeledger.RecoveryRequest{Kind: kind, TargetID: target}); reason != "" {
		return reason
	}
	if kind == noticeledger.RetryBatch {
		for _, b := range record.Snapshot().Batches {
			if b.Message.BatchID == target {
				return retryCompatibility(r.config, b, plan)
			}
		}
	}
	if plan.Assessment.State != routing.SupportedShape {
		return "no_eligible_work"
	}
	excluded := map[string]bool{}
	for _, revision := range record.Snapshot().Revisions {
		if revision.Digest == target {
			for _, row := range revision.Resources {
				excluded[row.ResourceID] = row.BatchID != "" || row.Disposition == "CLOSED"
			}
		}
	}
	selected := []string{}
	for _, d := range plan.Decisions {
		if d.Disposition == routing.Ready && !excluded[d.ResourceID] {
			selected = append(selected, d.ResourceID)
		}
	}
	drafts, err := r.builder.Build(n, true, lookups, selected)
	if err != nil || len(drafts.Issues) != 0 || len(drafts.Drafts) == 0 {
		return "no_eligible_work"
	}
	return ""
}
