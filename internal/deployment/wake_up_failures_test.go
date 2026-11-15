package deployment

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func TestNotificationRejectsUnboundedOrFailedBodyBeforeConfiguration(t *testing.T) {
	for _, kind := range []string{"oversized", "failed", "canceled", "missing-deadline"} {
		t.Run(kind, func(t *testing.T) {
			h, _, objects, _ := operationalFixture(t)
			ctx, cancel := context.WithCancel(operationalContext(t))
			defer cancel()
			input := &cancelInput{Reader: strings.NewReader("opaque")}
			if kind == "oversized" {
				input.Reader = strings.NewReader(strings.Repeat("x", 65537))
			}
			if kind == "canceled" {
				input.cancel = cancel
			}
			if kind == "missing-deadline" {
				ctx = context.Background()
			}
			var got OperationalResult
			if kind == "failed" {
				got = automaticResponse(t, h, ctx, intakeHeaders(), failedInput{})
			} else {
				got = automaticResponse(t, h, ctx, intakeHeaders(), input)
			}
			if got.Status != "not-ready" || objects.pinned.reads != 0 || objects.version != 0 {
				t.Fatalf("failed body reached work: %+v", got)
			}
		})
	}
}

func TestAutomaticOverlapKeepsOwnerAndDoesNotPublishCompletion(t *testing.T) {
	h, s, objects, _ := liveOperationFixture(t)
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	manager, err := reconciliation.NewLeaseManager(objects, p.environment)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := manager.Acquire(operationalContext(t))
	if err != nil || lease == nil {
		t.Fatal("fixture lease", err)
	}
	before := objects.version
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		t.Fatal("overlap reached processor")
		return operations{}, nil
	}
	got := automaticResponse(t, h, operationalContext(t), intakeHeaders(), strings.NewReader("opaque"))
	if got.Status != "coalesced" || got.Operation != "reconcile" || got.ScanCompleted || got.Telemetry != nil || objects.version != before || lease.Check(operationalContext(t), time.Second) != nil {
		t.Fatalf("overlap acquired or claimed work: %+v", got)
	}
}

func TestNotificationRequiredMetadataBounds(t *testing.T) {
	for _, key := range []string{"X-OCI-NS-TopicOcid", "X-OCI-NS-MessageId", "X-OCI-NS-MessageType"} {
		for _, kind := range []string{"multiple", "nil", "oversized", "space"} {
			headers := intakeHeaders()
			switch kind {
			case "multiple":
				headers.Add(key, "second")
			case "nil":
				headers[http.CanonicalHeaderKey(key)] = nil
			case "oversized":
				headers.Set(key, strings.Repeat("x", 1025))
			case "space":
				headers.Set(key, " ")
			}
			if _, _, err := notificationTopic(headers); err == nil {
				t.Fatal("invalid metadata accepted", key, kind)
			}
		}
	}
	headers := intakeHeaders()
	headers.Set("X-OCI-NS-MessageId", strings.Repeat("i", 1024))
	headers.Set("X-OCI-NS-MessageType", strings.Repeat("t", 128))
	headers.Set("X-OCI-NS-SigningCertURL", "https://example.invalid/do-not-fetch")
	headers.Set("Content-Type", "text/plain")
	if topic, present, err := notificationTopic(headers); err != nil || !present || topic != "ocid1.onstopic.oc1.iad.intake" {
		t.Fatal("valid bounded metadata refused", topic, present, err)
	}
}
