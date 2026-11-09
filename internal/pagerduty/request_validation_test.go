package pagerduty

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestInvalidCallsNeverDispatch(t *testing.T) {
	c, _ := tlsClient(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	for _, client := range []*Client{nil, {}, c} {
		ctx := budget(t)
		if client == c {
			ctx = nil
		}
		got, err := client.Send(ctx, []byte(savedEvent), []byte(syntheticKey))
		wantError(t, got, err, InvalidRequest, false)
	}
	got, err := c.Send(context.Background(), []byte(savedEvent), []byte(syntheticKey))
	wantError(t, got, err, DeadlineRequired, false)
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(budget(t))
		code := Canceled
		cause := context.Canceled
		if expired {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			code = DeadlineExceeded
			cause = context.DeadlineExceeded
		}
		cancel()
		got, err := c.Send(ctx, []byte(savedEvent), []byte(syntheticKey))
		wantError(t, got, err, code, false)
		if !errors.Is(err, cause) {
			t.Fatal("context identity lost")
		}
	}
	got, err = c.Send(budget(t), []byte("invalid"), []byte(syntheticKey))
	wantError(t, got, err, InvalidRequest, false)
	c.endpoint = ":invalid"
	got, err = c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
	wantError(t, got, err, InvalidRequest, false)
}
