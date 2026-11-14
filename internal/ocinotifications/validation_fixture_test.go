package ocinotifications

import (
	"context"
	"errors"
	"testing"
	"time"
)

func wantError(t *testing.T, err error, code Code, status int, sent bool) {
	t.Helper()
	var got *Error
	if !errors.As(err, &got) || got.Code != code || got.Status != status || got.MayHaveSent != sent {
		t.Fatalf("error=%v, want %s/%d/sent=%v", err, code, status, sent)
	}
	if err.Error() != "operator notification: "+string(code) {
		t.Fatal("uncontrolled diagnostic")
	}
	switch code {
	case Canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("lost cancellation")
		}
	case DeadlineExceeded:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("lost deadline")
		}
	default:
		if errors.Unwrap(err) != nil {
			t.Fatal("underlying error escaped")
		}
	}
}
func wantEmpty(t *testing.T, c Channel) {
	t.Helper()
	if c != (Channel{}) {
		t.Fatal("partial channel escaped")
	}
}

type retrievalResult struct {
	channel Channel
	err     error
}

func takeError(t *testing.T, ch <-chan retrievalResult) error {
	t.Helper()
	select {
	case r := <-ch:
		wantEmpty(t, r.channel)
		return r.err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not stop")
		return nil
	}
}
