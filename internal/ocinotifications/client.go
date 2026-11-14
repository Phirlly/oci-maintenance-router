// Package ocinotifications observes current operator-channel metadata and makes
// one bounded publication. It does not authorize, persist or retry sends.
package ocinotifications

import (
	"net/http"
	"reflect"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type Client struct {
	endpoint, region, regionKey, tenancy string
	signer                               common.HTTPRequestSigner
	transport                            *http.Transport
}

type ChannelSnapshot struct {
	TopicID, SubscriptionID, TopicCompartmentID, SubscriptionCompartmentID string
	Protocol, Endpoint                                                     string
}

// Channel retains observed metadata, not current freshness or send authority.
type Channel struct {
	client      *Client
	apiEndpoint string
	value       ChannelSnapshot
}

func (c Channel) Snapshot() ChannelSnapshot { return c.value }
func (c Channel) String() string            { return "operator channel (redacted)" }
func (c Channel) GoString() string          { return c.String() }

type Publication struct {
	Status      int
	MessageID   string
	MayHaveSent bool
}

// New fixes the OC1 control origin. Bootstrap must verify catalog/principal
// association; the provider must support concurrent signing. No credentials
// are read here and no endpoint override is exposed.
func New(region, regionKey, tenancy string, provider common.KeyProvider) (*Client, error) {
	if provider == nil || len(region) > 63 || !regionLabel.MatchString(region) || len(regionKey) > 63 || !regionKeyLabel.MatchString(regionKey) || len(tenancy) > 4096 || !tenancyPattern.MatchString(tenancy) {
		return nil, &Error{Code: InvalidConfiguration}
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil, &Error{Code: InvalidConfiguration}
		}
	}
	return &Client{endpoint: "https://notification." + region + ".oraclecloud.com", region: region, regionKey: regionKey, tenancy: tenancy, signer: common.DefaultRequestSigner(provider), transport: newTransport()}, nil
}
