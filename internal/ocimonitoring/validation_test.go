package ocimonitoring

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

func TestNewRequiresBoundedTrustedIdentity(t *testing.T) {
	for _, kind := range []string{"region", "region-long", "tenancy", "compartment", "foreign-root", "installation", "installation-long", "nil-provider", "typed-nil-provider"} {
		t.Run(kind, func(t *testing.T) {
			r, ten, comp, installation := "us-ashburn-1", tenancy, compartment, "installation-1"
			var p common.KeyProvider = &unusedProvider{}
			switch kind {
			case "region":
				r = "../host"
			case "region-long":
				r = strings.Repeat("x", 64)
			case "tenancy":
				ten = "not-tenancy"
			case "compartment":
				comp = "not-compartment"
			case "foreign-root":
				comp = "ocid1.tenancy.oc1..foreign"
			case "installation":
				installation = "customer/data"
			case "installation-long":
				installation = strings.Repeat("i", 129)
			case "nil-provider":
				p = nil
			case "typed-nil-provider":
				p = (*unusedProvider)(nil)
			}
			if client, err := New(r, ten, comp, installation, p); err == nil || client != nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	if _, err := New("us-ashburn-1", tenancy, tenancy, "installation-1", &unusedProvider{}); err != nil {
		t.Fatal("root compartment rejected", err)
	}
}

func TestInvalidObservationNeverReadsCredentials(t *testing.T) {
	negative, excessive := -1, 1001
	c, err := New("us-ashburn-1", tenancy, compartment, "installation-1", &unusedProvider{})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []Observation{
		{At: time.Now()}, {CompletedScan: true}, {At: time.Now(), ReviewIssues: &negative},
		{At: time.Now(), ReviewIssues: &excessive}, {At: time.Now().Add(-2 * time.Hour), CompletedScan: true},
		{At: time.Now().Add(11 * time.Minute), CompletedScan: true},
	} {
		if err := c.Publish(budget(t), value); err == nil {
			t.Fatal("invalid observation accepted")
		}
	}
	valid := Observation{At: time.Now(), CompletedScan: true}
	if err := c.Publish(context.Background(), valid); err == nil {
		t.Fatal("missing deadline accepted")
	}
	if err := c.Publish(nil, valid); err == nil {
		t.Fatal("nil context accepted")
	}
	if err := (*Client)(nil).Publish(budget(t), valid); err == nil {
		t.Fatal("nil client accepted")
	}
	ctx, cancel := context.WithCancel(budget(t))
	cancel()
	if err := c.Publish(ctx, valid); err == nil {
		t.Fatal("canceled work accepted")
	}
}
