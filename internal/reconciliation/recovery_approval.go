package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
)

func (r *processingRun) approveRecovery(saved ocistorage.StoredNotice, n notices.Notice, lookups map[string]routing.Lookup) error {
	plan := r.owner.policy.Resolve(n, true, lookups)
	if plan.Assessment.State != routing.SupportedShape {
		return r.rejectRecovery(saved, "no_eligible_work")
	}
	excluded := map[string]bool{}
	for _, revision := range saved.Record().Snapshot().Revisions {
		if revision.Digest == n.Key().Digest {
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
	drafts, err := r.owner.builder.Build(n, true, lookups, selected)
	if err != nil {
		return &ProcessingError{Code: "preparation_failed"}
	}
	if len(drafts.Issues) != 0 || len(drafts.Drafts) == 0 {
		return r.rejectRecovery(saved, "no_eligible_work")
	}
	versions := map[string]int64{}
	values := []delivery.Preparation{}
	for _, draft := range drafts.Drafts {
		message := draft.Snapshot()
		if versions[message.DestinationID] == 0 {
			version, code, err := r.currentVersion(message.SecretID)
			if err != nil {
				return err
			}
			if code != "" {
				return r.rejectRecovery(saved, "secret_unavailable")
			}
			versions[message.DestinationID] = version
		}
		values = append(values, delivery.Preparation{Draft: draft, SecretVersion: versions[message.DestinationID]})
	}
	if err = r.recoveryReadiness(); err != nil {
		if missingRecoveryReadiness(err) {
			if failure := r.rejectRecovery(saved, "readiness_required"); failure != nil {
				return failure
			}
		}
		return err
	}
	next, _, err := r.owner.observer.ApproveRecovery(saved.Record(), r.recoveryRequest(), n, true, lookups, r.lease.manager.now(), values...)
	if err != nil {
		return &ProcessingError{Code: "recovery_failed"}
	}
	if _, err = r.save(saved, next); err != nil {
		return err
	}
	r.result.snapshot.Prepared += len(values)
	return nil
}
