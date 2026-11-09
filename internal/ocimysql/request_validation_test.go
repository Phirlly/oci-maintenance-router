package ocimysql

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
	if err.Error() != "mysql metadata read: "+string(code) {
		t.Fatal("uncontrolled error")
	}
	switch code {
	case Canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost")
		}
	case DeadlineExceeded:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("deadline lost")
		}
	default:
		if errors.Unwrap(err) != nil {
			t.Fatal("raw error escaped")
		}
	}
}

func TestConstructorFixesEndpointWithoutCredentials(t *testing.T) {
	c, err := New("us-ashburn-1", "iad", tenancy, unusedProvider{})
	if err != nil || c.endpoint != "https://mysql.us-ashburn-1.ocp.oraclecloud.com" || c.region != "us-ashburn-1" || c.regionKey != "iad" || c.tenancy != tenancy {
		t.Fatal("incorrect regional client")
	}
	for _, v := range [][3]string{
		{"", "iad", tenancy}, {"us-x-1/path", "iad", tenancy}, {"US-X-1", "iad", tenancy}, {strings.Repeat("a", 64), "iad", tenancy},
		{"us-x-1", "", tenancy}, {"us-x-1", "IAD", tenancy}, {"us-x-1", "bad-key", tenancy}, {"us-x-1", strings.Repeat("a", 64), tenancy},
		{"us-x-1", "iad", ""}, {"us-x-1", "iad", "ocid1.compartment.oc1..x"}, {"us-x-1", "iad", "ocid1.tenancy.oc2..x"}, {"us-x-1", "iad", tenancy + strings.Repeat("x", 4096)},
	} {
		c, err := New(v[0], v[1], v[2], unusedProvider{})
		wantError(t, err, InvalidConfiguration, 0)
		if c != nil {
			t.Fatal("partial client")
		}
	}
	_, err = New("us-x-1", "iad", tenancy, nil)
	wantError(t, err, InvalidConfiguration, 0)
}

func TestInvalidReadNeverSigns(t *testing.T) {
	c, _ := New("us-ashburn-1", "iad", tenancy, unusedProvider{})
	for _, id := range []string{"", "ocid1.mysqlinstance.oc1.iad.x", "ocid1.mysqldbsystem.oc2.iad.x", "ocid1.mysqldbsystem.oc1.phx.x", "ocid1.mysqldbsystem.oc1.us-phoenix-1.x", "ocid1.mysqldbsystem.oc1..x", dbID + "/path", dbID + "?q", dbID + strings.Repeat("x", 4096)} {
		m, err := c.Get(budget(t), id)
		wantError(t, err, InvalidRequest, 0)
		wantEmpty(t, m)
	}
	for _, client := range []*Client{nil, {}} {
		m, err := client.Get(budget(t), dbID)
		wantError(t, err, InvalidRequest, 0)
		wantEmpty(t, m)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		m, err := c.Get(ctx, dbID)
		wantError(t, err, code, 0)
		wantEmpty(t, m)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	m, err := c.Get(ctx, dbID)
	wantError(t, err, DeadlineExceeded, 0)
	wantEmpty(t, m)
}
