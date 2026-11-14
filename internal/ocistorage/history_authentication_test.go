package ocistorage

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestHistoryAuthenticationFailuresNeverDispatch(t *testing.T) {
	key := generatedKey(t)
	for _, failID := range []bool{false, true} {
		c, _ := tlsClient(t, failingKey{key: key, failID: failID}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
		empty, err := c.HistoryIsEmpty(budget(t), "control/head.json")
		wantError(t, err, Authentication, 0, false)
		if empty || err.Error() != "object storage: authentication" || errors.Unwrap(err) != nil {
			t.Fatal("private failure escaped or established absence")
		}
	}
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	c, _ := tlsClient(t, syntheticKey{key: key, beforeKey: cancel}, func(http.ResponseWriter, *http.Request) { t.Error("late dispatch") })
	empty, err := c.HistoryIsEmpty(ctx, "control/head.json")
	wantError(t, err, Canceled, 0, false)
	if empty || !errors.Is(err, context.Canceled) {
		t.Fatal("signing cancellation was not preserved")
	}
}

func TestHistoryCapsOperationAndRejectsMalformedRequest(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	start := time.Now()
	var calls atomic.Int32
	c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		calls.Add(1)
		end, ok := ctx.Deadline()
		if !ok || end.Before(start.Add(9*time.Second)) || end.After(start.Add(10100*time.Millisecond)) {
			t.Error("ten-second operation cap absent")
		}
		return nil, errors.New("private network detail")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	empty, err := c.HistoryIsEmpty(ctx, "control/head.json")
	wantError(t, err, Transport, 0, false)
	if empty || calls.Load() != 1 || errors.Unwrap(err) != nil {
		t.Fatal("retry, absence or private error escaped")
	}
	c.endpoint = "://malformed"
	empty, err = c.HistoryIsEmpty(budget(t), "control/head.json")
	wantError(t, err, InvalidRequest, 0, false)
	if empty {
		t.Fatal("invalid endpoint established absence")
	}
}
