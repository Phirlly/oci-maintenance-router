package ocisecrets

import (
	"testing"
	"time"
)

type retrievalResult struct {
	bundle Bundle
	err    error
}

func takeResult(t *testing.T, ch <-chan retrievalResult) retrievalResult {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not stop")
		return retrievalResult{}
	}
}

func takeError(t *testing.T, ch <-chan retrievalResult) error {
	t.Helper()
	result := takeResult(t, ch)
	wantEmpty(t, result.bundle)
	return result.err
}
