package reconciliation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ociownership"
)

func TestRecoveryReviewerRequiresLiveConfigurationAndReadBindings(t *testing.T) {
	f := processingFixtureFor(t)
	r := reviewFixture(t, f)
	for _, kind := range []string{"source", "saved", "typed-nil", "catalog", "config", "version", "paused", "location", "mysql"} {
		t.Run(kind, func(t *testing.T) {
			data, catalog := f.live.config, []byte(preparationCatalog)
			location := candidateLocation()
			var source AnnouncementSource = f.source
			var saved SavedNoticeSource = r.collector.saved
			mysql := processingMySQL(t)
			switch kind {
			case "source":
				source = nil
			case "saved":
				saved = nil
			case "typed-nil":
				var p *reviewSavedSource
				saved = p
			case "catalog":
				catalog = []byte("private")
			case "config":
				data = []byte("private")
			case "version", "paused":
				var v map[string]any
				_ = json.Unmarshal(data, &v)
				if kind == "version" {
					v["schemaVersion"] = 6
				} else {
					v["mode"] = "PAUSED"
				}
				data, _ = json.Marshal(v)
			case "location":
				location.Bucket = ""
			case "mysql":
				mysql = map[string]*ociownership.Client{}
			}
			got, err := NewRecoveryReviewer(data, catalog, location, source, saved, mysql)
			if got != nil || err == nil || err.Error() != "recovery review: invalid_request" {
				t.Fatal("invalid binding admitted", got, err)
			}
		})
	}
}

func TestRecoveryReviewRejectsUnboundedInvalidAndCanceledRequests(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	seedReviewRevision(t, f, n)
	r := reviewFixture(t, f)
	for _, kind := range []string{"nil-reviewer", "id", "target", "kind", "nil-context", "unbounded", "expired", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			reviewer := r
			id, target, action := n.Key().NoticeID, n.Key().Digest, noticeledger.ApproveRevision
			ctx := leaseBudget(t)
			switch kind {
			case "nil-reviewer":
				reviewer = nil
			case "id":
				id = strings.Repeat("x", 4097)
			case "target":
				target = "bad"
			case "kind":
				action = noticeledger.CloseRevision
			case "nil-context":
				ctx = nil
			case "unbounded":
				ctx = context.Background()
			case "expired":
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := f.listed.Load()
			view, err := reviewer.Review(ctx, id, action, target)
			if err == nil || view.Fingerprint != "" || f.listed.Load() != before {
				t.Fatal("invalid request performed discovery", err)
			}
		})
	}
}
