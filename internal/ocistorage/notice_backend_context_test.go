package ocistorage

import (
	"context"
	"strings"
	"testing"
	"time"
)

type valueNoticeBackend struct{ *noticeBackend }

func TestNoticeBackendConstruction(t *testing.T) {
	var typedNil *noticeBackend
	for _, b := range []NoticeObjects{nil, typedNil, (*Client)(nil), &Client{}} {
		s, err := NewNoticeStore(b, strings.Repeat("a", 64))
		wantError(t, err, InvalidConfiguration, 0, false)
		if s != nil {
			t.Fatal("invalid backend retained")
		}
	}
	for _, env := range []string{"", "x", strings.Repeat("A", 64), strings.Repeat("a", 65)} {
		_, err := NewNoticeStore(&noticeBackend{}, env)
		wantError(t, err, InvalidConfiguration, 0, false)
	}
	if _, err := NewNoticeStore(valueNoticeBackend{&noticeBackend{}}, strings.Repeat("a", 64)); err != nil {
		t.Fatal("non-pointer implementation rejected", err)
	}
}

func TestNoticeBackendRejectsInvalidLifetimeBeforeIO(t *testing.T) {
	for _, op := range noticeOperations {
		t.Run(op, func(t *testing.T) {
			b := &noticeBackend{}
			invoke := backendOperation(t, b, op)
			backendResponse(t, b, func(context.Context) error { t.Fatal("invalid context reached storage"); return nil })
			canceled, cancel := context.WithCancel(budget(t))
			cancel()
			expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer stop()
			for _, tt := range []struct {
				ctx  context.Context
				code Code
			}{
				{nil, InvalidRequest}, {context.Background(), DeadlineRequired}, {canceled, Canceled}, {expired, DeadlineExceeded},
			} {
				wantError(t, invoke(tt.ctx), tt.code, 0, false)
			}
		})
	}
}

func TestNoticeBackendBoundsIOAndRejectsLateSuccess(t *testing.T) {
	for _, op := range noticeOperations {
		for _, scenario := range []string{"cap", "shorter", "cancel", "expire"} {
			t.Run(op+"/"+scenario, func(t *testing.T) {
				b := &noticeBackend{}
				invoke := backendOperation(t, b, op)
				duration := time.Minute
				if scenario == "shorter" {
					duration = time.Second
				}
				if scenario == "expire" {
					duration = 20 * time.Millisecond
				}
				ctx, cancel := context.WithTimeout(context.Background(), duration)
				defer cancel()
				parentDeadline, _ := ctx.Deadline()
				calls := 0
				backendResponse(t, b, func(child context.Context) error {
					calls++
					deadline, ok := child.Deadline()
					if !ok || time.Until(deadline) > 10*time.Second || deadline.After(parentDeadline) {
						t.Fatal("lifetime extended")
					}
					if scenario == "shorter" && !deadline.Equal(parentDeadline) {
						t.Fatal("shorter deadline changed")
					}
					if scenario == "cancel" {
						cancel()
					}
					if scenario == "expire" {
						<-child.Done()
					}
					return nil
				})
				err := invoke(ctx)
				if scenario == "cancel" || scenario == "expire" {
					code := Canceled
					if scenario == "expire" {
						code = DeadlineExceeded
					}
					wantError(t, err, code, 200, op == "create" || op == "save")
				} else if err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Fatal("operation retried or skipped")
				}
			})
		}
	}
}
