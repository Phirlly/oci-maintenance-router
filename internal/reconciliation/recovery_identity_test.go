package reconciliation

import (
	"bytes"
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"strings"
	"testing"
	"time"
)

func TestRecoveryRetainedIntentPrecedesMutablePrerequisites(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	q := reviewedRecovery(t, old, noticeledger.CloseBatch, old.Snapshot().Batches[0].Message.BatchID)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	first, err := p.Run(leaseBudget(t), f.live.lease)
	if err != nil || first.Snapshot().Recovery == nil {
		t.Fatal(err)
	}
	view := first.Snapshot()
	view.Recovery.Status = "changed"
	if first.Snapshot().Recovery.Status != string(noticeledger.RecoveryClosed) {
		t.Fatal("caller mutated recovery result")
	}
	before, _ := f.saved(t, n.Key().NoticeID).Encode()
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	f.p.discovery.collector.announcements = announcementDetails{page: f.source.page, get: func(context.Context, string) (notices.Notice, error) {
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 503}
	}}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Status != string(noticeledger.RecoveryClosed) || f.sent.Load() != 0 {
		t.Fatal("later discovery failure hid confirmed recovery", err)
	}
	after, _ := f.saved(t, n.Key().NoticeID).Encode()
	if !bytes.Equal(before, after) {
		t.Fatal("retained closure was rewritten before failed discovery")
	}
	q["targetId"] = strings.Repeat("f", 64)
	p = recoveryOnlyProcessor(t, f, "PAUSED", q)
	r, err = p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Recovery != nil || len(recoveryActions(t, f, n.Key().NoticeID)) != 1 {
		t.Fatal("same ID admitted changed intent")
	}
}

func TestRecoveryRejectionsAreRetainedAndNeverReadCredentials(t *testing.T) {
	for _, fault := range []string{"stale-review", "generation", "component", "target", "terminal", "readiness"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		state := delivery.Prepared
		if fault == "terminal" {
			state = delivery.APIAccepted
		}
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), state, true)
		q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
		want := ""
		switch fault {
		case "stale-review":
			q["reviewFingerprint"], want = strings.Repeat("f", 64), "stale_review"
		case "generation":
			q["routingGeneration"], want = strings.Repeat("f", 64), "configuration_changed"
		case "component":
			q["componentFingerprint"], want = strings.Repeat("f", 64), "component_changed"
		case "target":
			q["targetId"], want = strings.Repeat("f", 64), "target_missing"
		case "terminal":
			want = "target_terminal"
		case "readiness":
			want = "readiness_required"
		}
		f.configure(t, func(v map[string]any) {
			v["schemaVersion"], v["recovery"] = 7, q
			if fault == "readiness" {
				v["diagnostic"].(map[string]any)["receiptCode"] = "OMR-00000000000000000000000000000000"
			}
		})
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if (err != nil) != (fault == "readiness") || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Status != string(noticeledger.RecoveryRejected) || r.Snapshot().Recovery.Reason != want || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 {
			t.Fatal("wrong durable prerequisite rejection", fault, err, r.Snapshot())
		}
		if len(recoveryActions(t, f, n.Key().NoticeID)) != 1 {
			t.Fatal("rejection history missing")
		}
	}
}
