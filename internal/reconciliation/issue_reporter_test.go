package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocinotifications"
)

func TestIssueReporterPublishesDurableProblemAndCoalesces(t *testing.T) {
	f := issueReporterFixtureFor(t)
	lease := f.processing.live.lease
	before, _ := f.processing.saved(t, noticeID(1)).Encode()
	got, err := f.reporter.Refresh(leaseBudget(t), lease, f.snapshot)
	if err != nil || !got.Dispatched || !got.Accepted || got.Issues != 1 || got.Pending || f.sends != 1 {
		t.Fatal("saved issue not reported", got, err)
	}
	if _, err = f.reporter.Retry(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	if _, err = f.reporter.Refresh(leaseBudget(t), lease, f.snapshot); err != nil || f.sends != 1 {
		t.Fatal("unchanged issue was sent again", err)
	}
	after, _ := f.processing.saved(t, noticeID(1)).Encode()
	if !bytes.Equal(before, after) || f.processing.sent.Load() != 0 {
		t.Fatal("reporting mutated or sent business work")
	}
}

func TestIssueReporterRetriesSavedUnknownPublicationWithoutSourceCalls(t *testing.T) {
	f := issueReporterFixtureFor(t)
	read := f.reporter.read
	f.reporter.read = func(ctx context.Context) (operatorObservation, error) {
		channel, err := read(ctx)
		publish := channel.publish
		channel.publish = func(ctx context.Context, body []byte) (ocinotifications.Publication, error) {
			_, _ = publish(ctx, body)
			return ocinotifications.Publication{MayHaveSent: true}, errors.New("private provider text")
		}
		return channel, err
	}
	lease := f.processing.live.lease
	got, err := f.reporter.Refresh(leaseBudget(t), lease, f.snapshot)
	if err == nil || !got.Dispatched || !got.Pending || got.Accepted || f.sends != 1 {
		t.Fatal("unknown publication reported accepted", got, err)
	}
	if _, err = f.reporter.Retry(leaseBudget(t), lease); err != nil || f.sends != 1 {
		t.Fatal("immediate retry ignored persisted cooldown", err)
	}
	// Canonical aged claim represents a prior invocation; no accepted outcome is seeded.
	s := f.saved(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	s.Pending.IssuedAt = now.Add(-6 * time.Minute).Format(leaseTimeFormat)
	s.Pending.Deadline = now.Add(-5 * time.Minute).Format(leaseTimeFormat)
	s.NotBefore = now.Add(-time.Minute).Format(leaseTimeFormat)
	body, err := encodeReport(s)
	if err != nil {
		t.Fatal(err)
	}
	f.processing.live.objects.seed(issueReportName, body)
	f.reporter.read = read
	listed, fetched, credentials := f.processing.listed.Load(), f.processing.fetched.Load(), f.processing.current.Load()
	got, err = f.reporter.Retry(leaseBudget(t), lease)
	if err != nil || !got.Accepted || f.sends != 2 || !bytes.Equal(f.payloads[0], f.payloads[1]) {
		t.Fatal("restart did not retry exact saved operator payload", got, err)
	}
	if f.processing.listed.Load() != listed || f.processing.fetched.Load() != fetched || f.processing.current.Load() != credentials {
		t.Fatal("pending operator report depended on source or credentials")
	}
}
