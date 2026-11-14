package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

func issueStorageFailed(t *testing.T, got StoredPreparationIssues, err error, code string, status int, uncertain bool) {
	t.Helper()
	var e *PreparationIssueStorageError
	if !errors.As(err, &e) || e.Code != code || e.HTTPStatus != status || e.WriteUncertain != uncertain || got.store != nil {
		t.Fatalf("storage result=%+v error=%+v; want %s/%d/%t", got, err, code, status, uncertain)
	}
}

func TestPreparationIssueStoreRejectsUnreadableCurrentAndHistory(t *testing.T) {
	for _, backendError := range []error{errors.New("private backend text"), fmt.Errorf("wrapped: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404}), &ocistorage.Error{Code: ocistorage.Service, Status: 403}, (*ocistorage.Error)(nil)} {
		s, _, b, f := preparationIssueFixture(t)
		history := false
		b.history = func(context.Context, string) (bool, error) { history = true; return true, nil }
		b.get = func(context.Context, string) (ocistorage.Object, error) { return ocistorage.Object{}, backendError }
		got, err := s.Load(leaseBudget(t), f.Notice())
		issueStorageFailed(t, got, err, "read_failed", baselineStorageStatus(backendError), false)
		if history || strings.Contains(err.Error(), "private") {
			t.Fatal("error became absence or leaked data")
		}
	}
	s, _, b, f := preparationIssueFixture(t)
	b.history = func(context.Context, string) (bool, error) { return false, errors.New("private history") }
	got, err := s.Load(leaseBudget(t), f.Notice())
	issueStorageFailed(t, got, err, "history_read", 0, false)
}

func TestPreparationIssueStoreRejectsCorruptBodiesAndETags(t *testing.T) {
	for _, badTag := range []bool{false, true} {
		s, objects, b, f := preparationIssueFixture(t)
		p, _ := (noticeledger.PreparationIssues{}).Observe(f)
		body, _ := p.Encode()
		objects.seed(preparationIssueName(f.Notice()), body)
		b.get = func(ctx context.Context, name string) (ocistorage.Object, error) {
			o, e := objects.Get(ctx, name)
			if badTag {
				o.ETag = "*"
			} else {
				o.Body = []byte("private malformed record")
			}
			return o, e
		}
		got, err := s.Load(leaseBudget(t), f.Notice())
		code := "invalid_record"
		if badTag {
			code = "invalid_response"
		}
		issueStorageFailed(t, got, err, code, 200, false)
	}
}

func TestPreparationIssueStorePreservesUncertaintyWithoutAutomaticConfirmation(t *testing.T) {
	s, objects, b, f := preparationIssueFixture(t)
	previous, err := s.Load(leaseBudget(t), f.Notice())
	if err != nil {
		t.Fatal(err)
	}
	next, _ := previous.Record().Observe(f)
	reads := 0
	b.get = func(ctx context.Context, name string) (ocistorage.Object, error) {
		reads++
		return objects.Get(ctx, name)
	}
	b.create = func(ctx context.Context, name string, body []byte) (string, error) {
		_, err := objects.Create(ctx, name, body)
		if err != nil {
			return "", err
		}
		return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
	}
	got, err := s.Save(leaseBudget(t), previous, next)
	issueStorageFailed(t, got, err, "write_uncertain", 0, true)
	if reads != 0 {
		t.Fatal("store automatically confirmed without coordinator authority")
	}
	loaded, err := s.Load(leaseBudget(t), f.Notice())
	if err != nil || len(loaded.Record().Snapshot().Observations) != 1 {
		t.Fatal("lost response lost persisted bytes", err)
	}
}

func TestPreparationIssueStoreContextAndRequestFailures(t *testing.T) {
	s, objects, b, f := preparationIssueFixture(t)
	for _, backend := range []PreparationIssueObjects{nil, (*preparationIssueBackend)(nil)} {
		if _, err := NewPreparationIssueStore(backend, s.environment); err == nil {
			t.Fatal("nil backend accepted")
		}
	}
	if _, err := NewPreparationIssueStore(b, "invalid"); err == nil {
		t.Fatal("invalid environment accepted")
	}
	if got, err := s.Load(leaseBudget(t), noticeledger.Record{}); err == nil || got.store != nil {
		t.Fatal("invalid record accepted")
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if got, err := s.Load(ctx, f.Notice()); err == nil || got.store != nil {
			t.Fatal("unbounded context accepted")
		}
	}
	canceled, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	got, err := s.Load(canceled, f.Notice())
	issueStorageFailed(t, got, err, "canceled", 0, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	got, err = s.Load(expired, f.Notice())
	issueStorageFailed(t, got, err, "deadline_exceeded", 0, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline lost")
	}
	if errors.Is(&PreparationIssueStorageError{Code: "read_failed"}, context.Canceled) {
		t.Fatal("ordinary error is cancellation")
	}
	long, stopLong := context.WithTimeout(context.Background(), time.Minute)
	defer stopLong()
	b.get = func(ctx context.Context, name string) (ocistorage.Object, error) {
		d, _ := ctx.Deadline()
		if time.Until(d) > 10*time.Second {
			t.Error("storage bound extended")
		}
		return objects.Get(ctx, name)
	}
	if _, err = s.Load(long, f.Notice()); err != nil {
		t.Fatal(err)
	}
}
