package reconciliation

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/routing"
)

func savedPreparationIssues(t *testing.T, f *processingFixture, id string) noticeledger.PreparationIssues {
	t.Helper()
	r := f.saved(t, id)
	env, _ := r.Identity()
	s, err := NewPreparationIssueStore(f.live.objects, env)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(leaseBudget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	return got.Record()
}

func TestMaintenancePreparationSecretFailuresSurviveRestart(t *testing.T) {
	for _, code := range []string{"secret_unavailable", "secret_invalid"} {
		t.Run(code, func(t *testing.T) {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			current := f.p.current
			f.p.current = func(context.Context, string) (maintenanceCredential, error) {
				if code == "secret_invalid" {
					return diagnosticMaterial{7, func() []byte { return []byte("bad") }}, nil
				}
				return nil, nil
			}
			f.run(t)
			p := savedPreparationIssues(t, f, n.Key().NoticeID)
			if len(p.Snapshot().Observations) != 1 || p.Snapshot().Observations[0].Code != code {
				t.Fatal("preparation failure lost after restart")
			}
			f.p.current = current
			if s := f.run(t); s.Accepted != 1 {
				t.Fatal("restored dependency did not prepare/deliver")
			}
			p = savedPreparationIssues(t, f, n.Key().NoticeID)
			v, err := f.saved(t, n.Key().NoticeID).Review(p, fmt.Sprintf("%x", f.p.controls.configHash))
			if err != nil || len(v.Issues) != 0 || v.Preparation[0].Status != "HISTORICAL" || f.sent.Load() != 1 {
				t.Fatal("accepted issue was not historical", v, err)
			}
		})
	}
}

func TestMaintenancePreparationPayloadFailureRetainsIndexWithoutHugeID(t *testing.T) {
	large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
	n := processingNotice(t, 1, processingDB, large)
	f := processingFixtureFor(t, n)
	if s := f.run(t); s.Accepted != 1 {
		t.Fatal("fitting peer blocked")
	}
	p := savedPreparationIssues(t, f, n.Key().NoticeID)
	if len(p.Snapshot().Observations) != 1 {
		t.Fatal("payload blocker not durable")
	}
	x := p.Snapshot().Observations[0]
	if x.Code != "payload_too_large" || f.saved(t, n.Key().NoticeID).Snapshot().Revisions[0].Resources[x.ResourceIndex].ResourceID != large {
		t.Fatal("wrong resource correlated")
	}
	body, _ := p.Encode()
	if len(body) > 1024 || strings.Contains(string(body), large) {
		t.Fatal("duplicated source data")
	}
}

func TestHeldRecoveryApprovalPersistsSecretFailureWithoutClearingHold(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureWithBaseline(t, nil, n)
	metadata, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), processingDB)
	if err != nil {
		t.Fatal(err)
	}
	lookups := map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: metadata}}
	r, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, time.Now())
	if err == nil {
		r, err = r.Hold(delivery.ReasonOperator, time.Now())
	}
	if err != nil {
		t.Fatal(err)
	}
	body, _ := r.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	q := reviewedRecovery(t, r, noticeledger.ApproveRevision, n.Key().Digest)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	s := f.run(t)
	if s.Recovery == nil || s.Recovery.Reason != "secret_unavailable" || f.sent.Load() != 0 {
		t.Fatal("approval failure behavior changed", s)
	}
	p := savedPreparationIssues(t, f, n.Key().NoticeID)
	if len(p.Snapshot().Observations) != 1 || p.Snapshot().Observations[0].DestinationID != "payments" {
		t.Fatal("held approval failure not saved")
	}
	r = f.saved(t, n.Key().NoticeID)
	view, err := r.Review(p, fmt.Sprintf("%x", f.p.controls.configHash))
	if err != nil || r.Snapshot().HoldReason != delivery.ReasonOperator || view.Preparation[0].Status != "RECHECK_REQUIRED" {
		t.Fatal("issue evidence removed hold", err)
	}
}
