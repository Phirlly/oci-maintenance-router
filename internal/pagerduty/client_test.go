package pagerduty

import (
	"errors"
	"testing"
)

func TestFixedRegionalEndpoints(t *testing.T) {
	for region, host := range map[string]string{"US": "events.pagerduty.com", "EU": "events.eu.pagerduty.com"} {
		c, err := New(region)
		if err != nil || c.endpoint != "https://"+host+"/v2/enqueue" || c.transport == nil {
			t.Fatalf("region %s not configured: %v", region, err)
		}
	}
	for _, region := range []string{"", "us", "EU ", "https://example.invalid", "AP"} {
		c, err := New(region)
		var e *Error
		if c != nil || !errors.As(err, &e) || e.Code != InvalidConfiguration || e.MayHaveSent {
			t.Fatal("invalid region accepted")
		}
	}
}
