package ocisecrets

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestInvalidRequestsNeverReadCredentialsOrSend(t *testing.T) {
	c, _ := tlsClient(t, unusedProvider{}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
	for _, id := range []string{"", "ocid1.secret.oc1.exm.example", "ocid1.vaultsecret.oc2.exm.example", "ocid1.vaultsecret.oc1.other.example", "ocid1.vaultsecret.oc1..example", "ocid1.vaultsecret.oc1.exm.", testSecretID + ".extra.more", testSecretID + "/path", testSecretID + "?stage=CURRENT", testSecretID + "\n", strings.Repeat("a", 4097), "ocid1.vaultsecret.oc1.exm.bad!.example"} {
		b, err := c.GetCurrent(budget(t), id)
		wantError(t, err, InvalidRequest, 0)
		wantEmpty(t, b)
	}
	for _, version := range []int64{0, -1} {
		b, err := c.GetVersion(budget(t), testSecretID, version)
		wantError(t, err, InvalidRequest, 0)
		wantEmpty(t, b)
	}
}

func TestRetrievalRequiresLiveDeadlineAndUsableClient(t *testing.T) {
	c, _ := tlsClient(t, unusedProvider{}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
	for _, client := range []*Client{nil, {}} {
		b, err := client.GetCurrent(budget(t), testSecretID)
		wantError(t, err, InvalidRequest, 0)
		wantEmpty(t, b)
	}
	b, err := c.GetCurrent(nil, testSecretID)
	wantError(t, err, InvalidRequest, 0)
	wantEmpty(t, b)
	b, err = c.GetCurrent(context.Background(), testSecretID)
	wantError(t, err, DeadlineRequired, 0)
	wantEmpty(t, b)
	ctx, cancel := context.WithCancel(budget(t))
	cancel()
	b, err = c.GetCurrent(ctx, testSecretID)
	wantError(t, err, Canceled, 0)
	wantEmpty(t, b)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation identity")
	}
}
