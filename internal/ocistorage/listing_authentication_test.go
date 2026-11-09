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

func TestListingAuthenticationErrorsAndCancellationNeverDispatch(t *testing.T) {
	key := generatedKey(t)
	for _, failID := range []bool{false, true} {
		c, _ := tlsClient(t, failingKey{key: key, failID: failID}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
		p, err := noticeStore(t, c, noticeState(t)).ListPage(budget(t), "")
		wantError(t, err, Authentication, 0, false)
		emptyPage(t, p)
		if err.Error() != "object storage: authentication" || errors.Unwrap(err) != nil {
			t.Fatal("private credential detail escaped")
		}
	}
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	c, _ := tlsClient(t, syntheticKey{key: key, beforeKey: cancel}, func(http.ResponseWriter, *http.Request) { t.Error("late dispatch") })
	p, err := noticeStore(t, c, noticeState(t)).ListPage(ctx, "")
	wantError(t, err, Canceled, 0, false)
	emptyPage(t, p)
}

func TestListingRequiresBoundedOperationAndValidRequest(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	s := noticeStore(t, c, noticeState(t))
	start := time.Now()
	var calls atomic.Int32
	c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		calls.Add(1)
		end, ok := ctx.Deadline()
		if !ok || end.Before(start.Add(9*time.Second)) || end.After(start.Add(10100*time.Millisecond)) {
			t.Error("ten-second cap absent")
		}
		return nil, errors.New("private network detail")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	p, err := s.ListPage(ctx, "")
	wantError(t, err, Transport, 0, false)
	emptyPage(t, p)
	if calls.Load() != 1 || errors.Unwrap(err) != nil {
		t.Fatal("retry or private error")
	}
	c.endpoint = "://malformed"
	p, err = s.ListPage(budget(t), "")
	wantError(t, err, InvalidRequest, 0, false)
	emptyPage(t, p)
}
