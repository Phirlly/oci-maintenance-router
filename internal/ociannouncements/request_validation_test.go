package ociannouncements

import (
	"context"
	"crypto/rsa"
	"errors"
	"strings"
	"testing"
	"time"
)

type unusedProvider struct{}

func (unusedProvider) PrivateRSAKey() (*rsa.PrivateKey, error) { panic("unexpected credentials") }
func (unusedProvider) KeyID() (string, error)                  { panic("unexpected credentials") }

func wantError(t *testing.T, err error, code Code, status int) {
	t.Helper()
	var got *Error
	if !errors.As(err, &got) || got.Code != code || got.Status != status {
		t.Fatalf("error=%v, want %s/%d", err, code, status)
	}
	if err.Error() != "announcement read: "+string(code) {
		t.Fatal("uncontrolled error text")
	}
	switch code {
	case Canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("lost cancellation")
		}
	case DeadlineExceeded:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("lost deadline")
		}
	default:
		if errors.Unwrap(err) != nil {
			t.Fatal("uncontrolled error cause")
		}
	}
}

func TestConstructorFixesScopeWithoutReadingKeys(t *testing.T) {
	c, err := New("us-ashburn-1", "ocid1.tenancy.oc1..example", unusedProvider{})
	if err != nil || c.endpoint != "https://announcements.us-ashburn-1.oraclecloud.com" || c.tenancy != "ocid1.tenancy.oc1..example" {
		t.Fatal("incorrect endpoint/scope")
	}
	for _, pair := range [][2]string{{"", "ocid1.tenancy.oc1..x"}, {"us-x-1/path", "ocid1.tenancy.oc1..x"}, {"US-X-1", "ocid1.tenancy.oc1..x"}, {strings.Repeat("a", 64), "ocid1.tenancy.oc1..x"}, {"us-x-1", ""}, {"us-x-1", "ocid1.compartment.oc1..x"}, {"us-x-1", "ocid1.tenancy.oc2..x"}, {"us-x-1", "ocid1.tenancy.oc1.." + strings.Repeat("x", 4096)}} {
		c, err := New(pair[0], pair[1], unusedProvider{})
		wantError(t, err, InvalidConfiguration, 0)
		if c != nil {
			t.Fatal("partial client")
		}
	}
	if _, err := New("us-x-1", "ocid1.tenancy.oc1..x", nil); err == nil {
		t.Fatal("nil provider accepted")
	}
}

func TestInvalidReadsFailBeforeCredentials(t *testing.T) {
	c, _ := New("us-x-1", "ocid1.tenancy.oc1..x", unusedProvider{})
	for _, id := range []string{"", "ocid1.announcement.oc2..x", "ocid1.announcement.oc1.iad.x", noticeID + "/x", noticeID + "?x", noticeID + strings.Repeat("x", 4096)} {
		n, err := c.Get(budget(t), id)
		wantError(t, err, InvalidRequest, 0)
		if n.Key().NoticeID != "" {
			t.Fatal("partial notice")
		}
	}
	for _, token := range []string{"bad\n", string([]byte{0xff}), strings.Repeat("x", 4097)} {
		p, err := c.ListPage(budget(t), token)
		wantError(t, err, InvalidRequest, 0)
		if len(p.IDs) != 0 {
			t.Fatal("partial page")
		}
	}
	for _, client := range []*Client{nil, {}} {
		_, err := client.Get(budget(t), noticeID)
		wantError(t, err, InvalidRequest, 0)
		_, err = client.ListPage(budget(t), "")
		wantError(t, err, InvalidRequest, 0)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		_, err := c.Get(ctx, noticeID)
		wantError(t, err, code, 0)
		_, err = c.ListPage(ctx, "")
		wantError(t, err, code, 0)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := c.Get(ctx, noticeID)
	wantError(t, err, DeadlineExceeded, 0)
	_, err = c.ListPage(ctx, "")
	wantError(t, err, DeadlineExceeded, 0)
}
