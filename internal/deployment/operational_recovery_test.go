package deployment

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

func configureRecovery(t *testing.T, h *Handler, s Settings, objects *operationalStore, environment string, kind noticeledger.ActionKind) (Settings, *ocistorage.NoticeStore, string) {
	t.Helper()
	body, err := os.ReadFile("testdata/recovery-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var identity struct{ EnvironmentID, NoticeID string }
	if err := json.Unmarshal(body, &identity); err != nil {
		t.Fatal(err)
	}
	body = bytes.TrimSpace(bytes.ReplaceAll(body, []byte(identity.EnvironmentID), []byte(environment)))
	record, err := noticeledger.Decode(body, environment, identity.NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := ocistorage.NewNoticeStore(objects, environment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Create(operationalContext(t), record); err != nil {
		t.Fatal(err)
	}
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	target, component := record.Snapshot().Revisions[0].Digest, ""
	if kind == noticeledger.ApproveRevision {
		component = strings.Repeat("c", 64)
	}
	fingerprint, err := record.ReviewFingerprint(kind, target, p.generation, component)
	if err != nil {
		t.Fatal(err)
	}
	s = configureOperational(t, h, s, objects, func(v map[string]any) {
		q := map[string]any{"requestId": "recovery-1", "noticeId": identity.NoticeID, "kind": string(kind), "targetId": target, "reviewFingerprint": fingerprint, "routingGeneration": p.generation}
		if component != "" {
			q["componentFingerprint"] = component
		}
		v["recovery"] = q
	})
	return s, store, identity.NoticeID
}

func TestNonLiveReconcileExecutesRetainedRecoveryWithoutSending(t *testing.T) {
	for _, mode := range []string{"PAUSED", "TEST", "PREVIEW"} {
		for _, kind := range []noticeledger.ActionKind{noticeledger.CloseRevision, noticeledger.ApproveRevision} {
			t.Run(mode+"/"+string(kind), func(t *testing.T) {
				h, s, objects, environment, principal := retainedTestFixture(t, false)
				s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = mode })
				if mode == "PREVIEW" {
					bindActualPreparation(t, h, &emptyAnnouncementSource{})
					if got := invokeOperational(t, h, invocation(s)); got.Status != "operational-verified" {
						t.Fatalf("PREVIEW setup: %+v", got)
					}
					h.compose = composeOperations
				}
				_, store, notice := configureRecovery(t, h, s, objects, environment, kind)
				var prior []byte
				for i := 0; i < 2; i++ {
					got := invokeOperational(t, h, `{"operation":"reconcile"}`)
					status, action := "recovery-recorded", "CLOSED"
					if mode == "PREVIEW" {
						status = "prepared"
					}
					if kind == noticeledger.ApproveRevision {
						action = "REJECTED"
					}
					if got.Status != status || got.Recovery == nil || got.Recovery.Status != action || got.ScanCompleted || got.Diagnostic != nil || got.Operator != nil || principal.reads != 0 {
						t.Fatalf("recovery omitted, sent or reported a scan: %+v", got)
					}
					if action == "REJECTED" && got.Recovery.Reason != "readiness_required" {
						t.Fatalf("rejection not visible: %+v", got.Recovery)
					}
					saved, err := store.Load(operationalContext(t), notice)
					if err != nil {
						t.Fatal(err)
					}
					current, _ := saved.Record().Encode()
					if i == 1 && !bytes.Equal(prior, current) {
						t.Fatal("repeated recovery rewrote retained action")
					}
					prior = current
				}
			})
		}
	}
}
