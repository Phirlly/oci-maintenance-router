package ocistorage

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInvalidRequestsNeverReachCredentialsOrNetwork(t *testing.T) {
	var sends atomic.Int32
	c, _ := tlsClient(t, unusedProvider{}, func(http.ResponseWriter, *http.Request) { sends.Add(1) })
	for _, name := range []string{"", ".", "..", "line\n", "\x7f", string([]byte{255}), strings.Repeat("n", 1025)} {
		obj, err := c.Get(budget(t), name)
		wantError(t, err, InvalidRequest, 0, false)
		if obj.Body != nil || obj.ETag != "" {
			t.Fatal("invalid read returned data")
		}
		tag, err := c.Create(budget(t), name, nil)
		wantError(t, err, InvalidRequest, 0, false)
		if tag != "" {
			t.Fatal("invalid write returned tag")
		}
	}
	for _, tag := range []string{"", "*", "W/\"tag\"", "one,two", "\"one\",\"two\"", "\"\"", "\"unterminated", "in\"ner", "back\\slash", "has space", "tab\t", "line\r\n", "雪", strings.Repeat("t", 1025)} {
		got, err := c.Replace(budget(t), "state", tag, nil)
		wantError(t, err, InvalidRequest, 0, false)
		if got != "" {
			t.Fatal("invalid condition returned tag")
		}
	}
	_, err := c.Create(budget(t), "state", make([]byte, (1<<20)+1))
	wantError(t, err, TooLarge, 0, false)
	_, err = c.Replace(budget(t), "state", "tag", make([]byte, (1<<20)+1))
	wantError(t, err, TooLarge, 0, false)
	if sends.Load() != 0 {
		t.Fatal("invalid input reached network")
	}
}

func TestRequestNeedsLiveCallerDeadlineAndUsableClient(t *testing.T) {
	c, _ := tlsClient(t, unusedProvider{}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	_, err := c.Get(context.Background(), "state")
	wantError(t, err, DeadlineRequired, 0, false)
	_, err = c.Get(nil, "state")
	wantError(t, err, InvalidRequest, 0, false)
	for _, client := range []*Client{nil, {}} {
		_, err = client.Get(budget(t), "state")
		wantError(t, err, InvalidRequest, 0, false)
	}
	ctx, cancel := context.WithCancel(budget(t))
	cancel()
	_, err = c.Create(ctx, "state", nil)
	wantError(t, err, Canceled, 0, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation identity missing")
	}
}
