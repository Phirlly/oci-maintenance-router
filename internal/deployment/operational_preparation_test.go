package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/reconciliation"
)

type emptyAnnouncementSource struct{ pages int }

func (s *emptyAnnouncementSource) ListPage(ctx context.Context, _ string) (ociannouncements.Page, error) {
	s.pages++
	return ociannouncements.Page{IDs: []string{}}, ctx.Err()
}
func (*emptyAnnouncementSource) Get(context.Context, string) (notices.Notice, error) {
	return notices.Notice{}, errors.New("unexpected detail lookup")
}

func bindActualPreparation(t *testing.T, h *Handler, source *emptyAnnouncementSource) {
	t.Helper()
	h.compose = func(s Settings, store Store, p pinnedInputs, _ string) (operations, error) {
		preparation, err := reconciliation.NewBaselinePreparation(p.configBytes, p.catalogBytes,
			configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket}, source, store.(operationalObjects))
		if err != nil {
			return operations{}, err
		}
		return operations{initialize: preparation.Initialize, prepare: preparation.Prepare}, nil
	}
}

func invokeOperational(t *testing.T, h *Handler, body string) OperationalResult {
	t.Helper()
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(body), out)
	var got OperationalResult
	if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil || got.ResponseVersion != 2 || out.Body.Len() > 4096 {
		t.Fatalf("invalid operational result: %s (%v)", out.Body, err)
	}
	return got
}

func TestPreviewCheckInitializesAndReusesHistoricalBaseline(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	source := &emptyAnnouncementSource{}
	bindActualPreparation(t, h, source)
	for i := 0; i < 2; i++ {
		got := invokeOperational(t, h, invocation(s))
		if got.Status != "operational-verified" || !got.BaselinePrepared || got.ScanCompleted || got.BaselineSelected || got.CheckAttemptID != testAttemptID || got.ConfigurationSHA256 != s.ConfigurationSHA256 || got.ReleaseSHA256 != s.ReleaseSHA256 || got.Code != "" || got.CleanupCode != "" {
			t.Fatalf("PREVIEW verification did not complete: %+v", got)
		}
		if source.pages != 1 || len(objects.objects) != 3 {
			t.Fatalf("baseline was rescanned/reset: pages=%d objects=%d", source.pages, len(objects.objects))
		}
	}
}

func TestPreviewReconcileCannotInitializeMissingHistory(t *testing.T) {
	for _, operation := range []string{"reconcile", "deployment-check-without-intent"} {
		t.Run(operation, func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			source := &emptyAnnouncementSource{}
			bindActualPreparation(t, h, source)
			body := `{"operation":"reconcile"}`
			if operation != "reconcile" {
				s = configureOperational(t, h, s, objects, func(v map[string]any) { v["preparation"] = map[string]any{"initialize": false} })
				body = invocation(s)
			}
			got := invokeOperational(t, h, body)
			if got.Status != "not-ready" || got.Code != "read_failed" || got.BaselinePrepared || got.ScanCompleted || source.pages != 0 || len(objects.objects) != 1 {
				t.Fatalf("missing history was initialized or concealed: %+v; pages=%d objects=%d", got, source.pages, len(objects.objects))
			}
		})
	}
}

func TestPreviewFinalCurrentChangePreservesHistoryWithoutReportingVerification(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	bindActualPreparation(t, h, &emptyAnnouncementSource{})
	compose := h.compose
	h.compose = func(s Settings, store Store, p pinnedInputs, operation string) (operations, error) {
		ops, err := compose(s, store, p, operation)
		prepare := ops.prepare
		ops.prepare = func(ctx context.Context, lease *reconciliation.Lease) (reconciliation.Baseline, error) {
			baseline, err := prepare(ctx, lease)
			objects.pinned.config = []byte("changed current control")
			return baseline, err
		}
		return ops, err
	}
	got := invokeOperational(t, h, invocation(s))
	if got.Status != "not-ready" || got.Code != "configuration_changed" || !got.BaselinePrepared || got.ScanCompleted || len(objects.objects) != 3 {
		t.Fatalf("late change lost history or reported verification: %+v", got)
	}
}
