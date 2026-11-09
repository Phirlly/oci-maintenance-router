package ociannouncements

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/notices"
)

type retrievalResult struct {
	notice notices.Notice
	err    error
}

func wantEmpty(t *testing.T, n notices.Notice) {
	t.Helper()
	if n.Key() != (notices.RevisionKey{}) {
		t.Fatal("partial notice returned")
	}
}
func takeError(t *testing.T, ch <-chan retrievalResult) error {
	t.Helper()
	select {
	case result := <-ch:
		wantEmpty(t, result.notice)
		return result.err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not stop")
		return nil
	}
}
