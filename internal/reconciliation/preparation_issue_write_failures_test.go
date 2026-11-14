package reconciliation

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationIssueStoreSanitizesRejectedAndMalformedWrites(t *testing.T) {
	for _, kind := range []string{"service", "unknown", "etag", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s, objects, b, f := preparationIssueFixture(t)
			previous, err := s.Load(leaseBudget(t), f.Notice())
			if err != nil {
				t.Fatal(err)
			}
			next, _ := previous.Record().Observe(f)
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			b.create = func(c context.Context, name string, body []byte) (string, error) {
				switch kind {
				case "service":
					return "", &ocistorage.Error{Code: ocistorage.Service, Status: 403, WriteUncertain: true}
				case "unknown":
					return "", errors.New("private provider data")
				}
				tag, err := objects.Create(c, name, body)
				if kind == "etag" {
					tag = "*"
				} else {
					cancel()
				}
				return tag, err
			}
			got, err := s.Save(ctx, previous, next)
			code, status, uncertain := "write_failed", 0, true
			switch kind {
			case "service":
				status, uncertain = 403, false
			case "etag":
				code, status = "write_uncertain", 200
			case "cancel":
				code, status = "canceled", 200
			}
			issueStorageFailed(t, got, err, code, status, uncertain)
		})
	}
}

func TestPreparationIssueStoreCanceledSaveDoesNotWrite(t *testing.T) {
	s, _, b, f := preparationIssueFixture(t)
	previous, err := s.Load(leaseBudget(t), f.Notice())
	if err != nil {
		t.Fatal(err)
	}
	next, _ := previous.Record().Observe(f)
	writes := 0
	b.create = func(context.Context, string, []byte) (string, error) { writes++; return "v1", nil }
	ctx, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	got, err := s.Save(ctx, previous, next)
	issueStorageFailed(t, got, err, "canceled", 0, false)
	if writes != 0 {
		t.Fatal("canceled write dispatched")
	}
}
