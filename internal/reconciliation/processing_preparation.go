package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
)

func (r *processingRun) prepare(saved ocistorage.StoredNotice, n notices.Notice, lookups map[string]routing.Lookup) (ocistorage.StoredNotice, error) {
	plan := r.owner.policy.Resolve(n, true, lookups)
	if plan.Assessment.State != routing.SupportedShape {
		return saved, r.issue(ProcessingIssue{NoticeID: n.Key().NoticeID, RevisionDigest: n.Key().Digest, Code: string(plan.Assessment.Reason)})
	}
	assigned := map[string]bool{}
	for _, revision := range saved.Record().Snapshot().Revisions {
		if revision.Digest == n.Key().Digest {
			for _, row := range revision.Resources {
				if row.BatchID != "" || row.Disposition == "CLOSED" {
					assigned[row.ResourceID] = true
				}
			}
		}
	}
	selected := []string{}
	for _, d := range plan.Decisions {
		if assigned[d.ResourceID] {
			continue
		}
		if d.Disposition == routing.Ready {
			if r.suppressed(n.Key(), d, lookups) {
				r.result.snapshot.Suppressed++
				continue
			}
			selected = append(selected, d.ResourceID)
		} else if d.Disposition != routing.OutOfScope {
			if err := r.issue(ProcessingIssue{NoticeID: n.Key().NoticeID, RevisionDigest: n.Key().Digest, ResourceID: d.ResourceID, Code: string(d.Reason)}); err != nil {
				return saved, err
			}
		}
	}
	drafts, err := r.owner.builder.Build(n, true, lookups, selected)
	if err != nil {
		return saved, &ProcessingError{Code: "preparation_failed"}
	}
	for _, issue := range drafts.Issues {
		if err = r.recordPreparation(saved, n, lookups, issue.Code, issue.ResourceID); err != nil {
			return saved, err
		}
		if err = r.issue(ProcessingIssue{NoticeID: n.Key().NoticeID, RevisionDigest: n.Key().Digest, ResourceID: issue.ResourceID, Code: issue.Code}); err != nil {
			return saved, err
		}
	}
	versions, failures := map[string]int64{}, map[string]string{}
	for _, draft := range drafts.Drafts {
		message := draft.Snapshot()
		if versions[message.DestinationID] == 0 && failures[message.DestinationID] == "" {
			version, code, failure := r.currentVersion(message.SecretID)
			if failure != nil {
				return saved, failure
			}
			versions[message.DestinationID], failures[message.DestinationID] = version, code
		}
		if code := failures[message.DestinationID]; code != "" {
			if err = r.recordPreparation(saved, n, lookups, code, preparationMembers(message.Resources)...); err != nil {
				return saved, err
			}
			if err = r.issue(ProcessingIssue{NoticeID: n.Key().NoticeID, RevisionDigest: n.Key().Digest, BatchID: message.BatchID, Code: code}); err != nil {
				return saved, err
			}
			continue
		}
		next, prepareErr := saved.Record().Prepare(r.lease.manager.now(), delivery.Preparation{Draft: draft, SecretVersion: versions[message.DestinationID]})
		if prepareErr != nil {
			if e, ok := prepareErr.(*noticeledger.Error); !ok || e == nil || e.Code != "too_large" {
				return saved, &ProcessingError{Code: "preparation_failed"}
			}
			if err = r.recordPreparation(saved, n, lookups, "state_capacity", preparationMembers(message.Resources)...); err != nil {
				return saved, err
			}
			if err = r.issue(ProcessingIssue{NoticeID: n.Key().NoticeID, RevisionDigest: n.Key().Digest, BatchID: message.BatchID, Code: "state_capacity"}); err != nil {
				return saved, err
			}
			continue
		}
		saved, err = r.save(saved, next)
		if err != nil {
			return saved, err
		}
		r.result.snapshot.Prepared++
	}
	return saved, nil
}

func (r *processingRun) suppressed(revision notices.RevisionKey, d routing.Decision, lookups map[string]routing.Lookup) bool {
	metadata := lookups[d.ResourceID].Metadata
	return metadata.CompartmentID != nil && r.baseline.Suppresses("mysqldbsystem", d.Region, *metadata.CompartmentID, revision)
}

func (r *processingRun) currentVersion(id string) (int64, string, error) {
	if err := r.before(); err != nil {
		return 0, "", err
	}
	credential, err := r.owner.current(r.ctx, id)
	if r.ctx.Err() != nil {
		return 0, "", processingFailure(r.ctx, nil)
	}
	if err != nil || credential == nil {
		return 0, "secret_unavailable", nil
	}
	key := credential.Bytes()
	defer clear(key)
	if credential.Version() <= 0 || !pagerduty.ValidRoutingKey(key) {
		return 0, "secret_invalid", nil
	}
	return credential.Version(), "", nil
}
