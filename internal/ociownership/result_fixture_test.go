package ociownership

import (
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/routing"
)

type retrievalResult struct {
	metadata routing.Metadata
	err      error
}

func wantEmpty(t *testing.T, m routing.Metadata) {
	t.Helper()
	if !reflect.DeepEqual(m, routing.Metadata{}) {
		t.Fatal("partial metadata returned")
	}
}
func takeError(t *testing.T, ch <-chan retrievalResult) error {
	t.Helper()
	select {
	case result := <-ch:
		wantEmpty(t, result.metadata)
		return result.err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not stop")
		return nil
	}
}
