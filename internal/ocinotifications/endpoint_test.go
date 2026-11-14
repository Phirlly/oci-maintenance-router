package ocinotifications

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReturnedOriginAllowsOnlyConfiguredRegionalService(t *testing.T) {
	c, _ := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	for _, origin := range []string{controlOrigin, dataOrigin, "https://notification.us-ashburn-1.oci.oraclecloud.com", "https://another.cell.notification.us-ashburn-1.oci.oraclecloud.com"} {
		for _, slash := range []string{"", "/"} {
			got, ok := c.topicOrigin(origin + slash)
			if !ok || got != origin {
				t.Fatal("valid returned service origin rejected")
			}
		}
	}
	for _, origin := range []string{
		"", strings.Repeat("x", 2049),
		"http://" + strings.TrimPrefix(dataOrigin, "https://"),
		dataOrigin + "/20181201", dataOrigin + "/x",
		dataOrigin + "?", dataOrigin + "?x", dataOrigin + "#", dataOrigin + "#x",
		dataOrigin + ":443", dataOrigin + ":", dataOrigin + "/../", dataOrigin + "/%2f",
		strings.ToUpper(dataOrigin),
		strings.Replace(dataOrigin, "https://", "https://user@", 1),
		strings.Replace(dataOrigin, "us-ashburn-1", "us-phoenix-1", 1),
		dataOrigin + ".evil.invalid", dataOrigin + ".",
		"https://evilnotification.us-ashburn-1.oci.oraclecloud.com",
		"https://.notification.us-ashburn-1.oci.oraclecloud.com",
		"https://-bad.notification.us-ashburn-1.oci.oraclecloud.com",
		"https://bad_.notification.us-ashburn-1.oci.oraclecloud.com",
		"https://" + strings.Repeat("a", 64) + ".notification.us-ashburn-1.oci.oraclecloud.com",
		"https://" + strings.Repeat(strings.Repeat("a", 60)+".", 4) + "notification.us-ashburn-1.oci.oraclecloud.com",
		"https://%invalid",
	} {
		if value, ok := c.topicOrigin(origin); ok || value != "" {
			t.Errorf("unsafe origin accepted: %q", origin)
		}
	}
}

func TestUntrustedReturnedOriginStopsBeforeSecondSigning(t *testing.T) {
	var signs, calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t), beforeKey: func() { signs.Add(1) }}, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, strings.Replace(topicJSON, dataOrigin, "https://untrusted.example.invalid", 1))
	})
	ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
	wantEmpty(t, ch)
	wantError(t, err, InvalidResponse, 200, false)
	if calls.Load() != 1 || signs.Load() != 1 {
		t.Fatal("untrusted origin was signed or dispatched")
	}
}
