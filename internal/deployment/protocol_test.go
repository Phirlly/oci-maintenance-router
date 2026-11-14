package deployment

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fdk "github.com/fnproject/fdk-go"
)

func protocolClient(t *testing.T, h *Handler) *http.Client {
	t.Helper()
	// A short private path also fits macOS's Unix socket path-length limit.
	dir, err := os.MkdirTemp("", "orm-fdk-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "function.sock")
	t.Setenv("FN_FORMAT", "http-stream")
	t.Setenv("FN_LISTENER", "unix:"+socket)
	t.Setenv("OCI_TRACING_ENABLED", "0")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		fdk.HandleContext(ctx, h)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("FDK server did not stop")
		}
	})
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("FDK socket not ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	return client
}

func TestFDKReturnsStructuredNonsendingStatus(t *testing.T) {
	h, settings, _ := handlerFixture(t)
	client := protocolClient(t, h)
	for _, tc := range []struct{ input, status, attempt string }{
		{invocation(settings), "deployment-verified", testAttemptID},
		{"LIVE", "not-ready", ""},
	} {
		t.Run(tc.status, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "http://socket/call", strings.NewReader(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Fn-Deadline", time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano))
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
			if err != nil {
				t.Fatal(err)
			}
			var got Result
			if json.Unmarshal(body, &got) != nil || got.Status != tc.status || got.SendingEnabled || got.CheckAttemptID != tc.attempt {
				t.Fatalf("protocol response: %q", body)
			}
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Fn-Fdk-Version") == "" {
				t.Fatalf("unexpected FDK transport: status=%d headers=%v", resp.StatusCode, resp.Header)
			}
		})
	}
}

func TestFDKReturnsOperationalVersionAndTruthfulCompletion(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	client := protocolClient(t, h)
	for _, tc := range []struct{ body, status, attempt string }{
		{invocation(s), "operational-verified", testAttemptID},
		{`{"operation":"reconcile"}`, "idle", ""},
	} {
		req, err := http.NewRequest(http.MethodPost, "http://socket/call", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Fn-Deadline", time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
		closeErr := resp.Body.Close()
		var got OperationalResult
		if err != nil || closeErr != nil || len(body) > 4096 || json.Unmarshal(body, &got) != nil || got.ResponseVersion != 2 || got.Status != tc.status || got.CheckAttemptID != tc.attempt || got.Mode != "PAUSED" || got.ScanCompleted || got.ConfigurationSHA256 != s.ConfigurationSHA256 || got.ReleaseSHA256 != s.ReleaseSHA256 || got.CleanupCode != "" {
			t.Fatalf("incorrect operational wire response: %s (%v/%v)", body, err, closeErr)
		}
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Fn-Fdk-Version") == "" {
			t.Fatal("missing actual FDK transport")
		}
	}
}
