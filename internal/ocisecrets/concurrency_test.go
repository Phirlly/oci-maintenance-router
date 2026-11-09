package ocisecrets

import (
	"context"
	"net/http"
	"testing"
)

func TestConcurrentOperationsKeepIndependentCancellationAndVersions(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("stage") == "CURRENT" {
			close(entered)
			<-r.Context().Done()
			close(stopped)
			return
		}
		if r.URL.Query().Get("versionNumber") != "7" {
			t.Error("wrong pinned version")
		}
		_, _ = w.Write(bundleBody(t, testSecretID, 7, []byte("synthetic-key")))
	})
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan retrievalResult, 1)
	go func() { b, err := c.GetCurrent(ctx, testSecretID); done <- retrievalResult{b, err} }()
	await(t, entered)
	b, err := c.GetVersion(budget(t), testSecretID, 7)
	if err != nil || b.Version() != 7 || string(b.Bytes()) != "synthetic-key" {
		t.Fatalf("independent pinned request failed: %v", err)
	}
	cancel()
	wantError(t, takeError(t, done), Canceled, 0)
	await(t, stopped)
}
