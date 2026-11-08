package ocistorage

import (
	"context"
	"crypto/rsa"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancellationDuringSigningPreventsLateDispatch(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var sends atomic.Int32
	provider := syntheticKey{key: generatedKey(t), beforeKey: func() { close(entered); <-release }}
	c, _ := tlsClient(t, provider, func(http.ResponseWriter, *http.Request) { sends.Add(1) })
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Create(ctx, "state", []byte("{}")); done <- err }()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	await(t, entered)
	cancel()
	select {
	case <-done:
		t.Fatal("signer was abandoned")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	released = true
	wantError(t, takeError(t, done), Canceled, 0, false)
	if sends.Load() != 0 {
		t.Fatal("late request sent")
	}
}

type failingKey struct {
	key    *rsa.PrivateKey
	failID bool
}

func (p failingKey) PrivateRSAKey() (*rsa.PrivateKey, error) {
	if p.failID {
		return p.key, nil
	}
	return nil, errors.New("synthetic-private-credential")
}
func (p failingKey) KeyID() (string, error) { return "", errors.New("synthetic-private-identity") }

func TestCredentialErrorsAreControlledAndNeverDispatched(t *testing.T) {
	key := generatedKey(t)
	for _, failID := range []bool{false, true} {
		c, _ := tlsClient(t, failingKey{key: key, failID: failID}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
		_, err := c.Create(budget(t), "state", nil)
		wantError(t, err, Authentication, 0, false)
		if err.Error() != "object storage: authentication" || errors.Unwrap(err) != nil {
			t.Fatal("credential error escaped")
		}
	}
}
