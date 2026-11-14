package ocistorage

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestHistoryInvalidInputsNeverSignOrDispatch(t *testing.T) {
	c, _ := tlsClient(t, unusedProvider{}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	for _, name := range []string{"", ".", "..", "line\n", "\x7f", string([]byte{255}), strings.Repeat("n", 1025)} {
		empty, err := c.HistoryIsEmpty(budget(t), name)
		wantError(t, err, InvalidRequest, 0, false)
		if empty {
			t.Fatal("invalid request established absence")
		}
	}
	for _, client := range []*Client{nil, {}, {signer: c.signer}, {transport: c.transport}} {
		empty, err := client.HistoryIsEmpty(budget(t), "control/head.json")
		wantError(t, err, InvalidRequest, 0, false)
		if empty {
			t.Fatal("unusable client established absence")
		}
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		empty, err := c.HistoryIsEmpty(ctx, "control/head.json")
		wantError(t, err, code, 0, false)
		if empty {
			t.Fatal("unbounded call established absence")
		}
	}
	ctx, cancel := context.WithCancel(budget(t))
	cancel()
	empty, err := c.HistoryIsEmpty(ctx, "control/head.json")
	wantError(t, err, Canceled, 0, false)
	if empty || !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was not preserved")
	}
}
