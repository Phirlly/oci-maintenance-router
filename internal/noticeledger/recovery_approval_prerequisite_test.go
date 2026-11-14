package noticeledger

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
)

func TestRecoveryApprovalValidatesActualCurrentReadyWork(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db), db))
	q := approvalRequest(t, r, r.Snapshot().Revisions[0].Digest)
	n := normalized(t, source(db))
	values := preparations(t, source(db), db)
	for _, kind := range []string{"no_observer", "no_notice", "foreign_notice", "bad_time", "zero_draft", "zero_version", "duplicate", "too_many", "missing_owner", "wrong_owner"} {
		o := observer(t, nil)
		current := n
		found := lookups(db)
		at := clock()
		p := append([]delivery.Preparation{}, values...)
		switch kind {
		case "no_observer":
			o = nil
		case "no_notice":
			current = notices.Notice{}
		case "foreign_notice":
			s := source(db)
			s.NoticeID = "ocid1.announcement.oc1..foreign"
			current = normalized(t, s)
		case "bad_time":
			at = time.Time{}
		case "zero_draft":
			p[0].Draft = delivery.Draft{}
		case "zero_version":
			p[0].SecretVersion = 0
		case "duplicate":
			p = append(p, p[0])
		case "too_many":
			p = make([]delivery.Preparation, 257)
		case "missing_owner":
			found = nil
		case "wrong_owner":
			v := found[db]
			v.Metadata.FreeformTags = map[string]string{"owner": "video"}
			found[db] = v
		}
		bad, a, err := o.ApproveRecovery(r, q, current, false, found, at, p...)
		rejected(t, bad, err)
		if a.Status != "" {
			t.Fatal("invalid preparation consumed action", kind)
		}
	}
	for _, kind := range []string{"stale", "empty", "configuration"} {
		req := q
		o := observer(t, nil)
		p := values
		if kind == "stale" {
			req.ReviewFingerprint = strings.Repeat("c", 64)
		}
		if kind == "empty" {
			p = nil
		}
		if kind == "configuration" {
			o = observer(t, func(v map[string]any) { v["pagerDutyRegion"] = "EU" })
		}
		next, a, err := o.ApproveRecovery(r, req, n, false, lookups(db), clock(), p...)
		want := map[string]string{"stale": "stale_review", "empty": "no_eligible_work", "configuration": "configuration_changed"}[kind]
		if err != nil || a.Status != RecoveryRejected || a.Reason != want || len(next.Snapshot().Batches) != 0 {
			t.Fatal(kind, a, err)
		}
		reload(t, next)
	}
}

func TestRecoveryApprovalDoesNotReuseExistingOrAcceptedBatches(t *testing.T) {
	r, q, a := approvedRecord(t, true)
	id := a.BatchIDs[0]
	for _, accepted := range []bool{false, true} {
		current := r
		if accepted {
			started, ref, err := current.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(current, id, clock()))
			if err != nil {
				t.Fatal(err)
			}
			current, err = started.Finish(ref, 202, clock(), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
		}
		nextQ := approvalRequest(t, current, q.TargetID)
		nextQ.RequestID = "approve-2"
		before := encoded(t, current)
		bad, _, err := observer(t, nil).ApproveRecovery(current, nextQ, normalized(t, source(db)), false, lookups(db), clock(), preparations(t, source(db), db)...)
		rejected(t, bad, err)
		if !bytes.Equal(before, encoded(t, current)) {
			t.Fatal("failed preparation rewrote saved batch")
		}
	}
}
