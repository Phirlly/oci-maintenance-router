// Package ociownership reads current MySQL and Compute ownership metadata. It does not
// access SQL, change resources, cache metadata, retry reads or authorize delivery.
package ociownership

import (
	"net/http"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type Client struct {
	endpoint, region, regionKey, tenancy string
	resourceType, resourcePath           string
	signer                               common.HTTPRequestSigner
	transport                            *http.Transport
}

// BoundTo verifies a fixed reader's public identity without reading credentials.
func (c *Client) BoundTo(kind, region, regionKey, tenancy string) bool {
	return c != nil && c.resourceType == kind && c.region == region && c.regionKey == regionKey && c.tenancy == tenancy && c.signer != nil && c.transport != nil
}

// NewMySQL binds one regional OC1 GetDbSystem reader.
func NewMySQL(region, regionKey, tenancy string, provider common.KeyProvider) (*Client, error) {
	c, err := newClient(region, regionKey, tenancy, provider)
	if err != nil {
		return nil, err
	}
	c.endpoint = "https://mysql." + region + ".ocp.oraclecloud.com"
	c.resourceType, c.resourcePath = "mysqldbsystem", "/20190415/dbSystems/"
	return c, nil
}

// NewCompute binds one regional OC1 GetInstance reader.
func NewCompute(region, regionKey, tenancy string, provider common.KeyProvider) (*Client, error) {
	c, err := newClient(region, regionKey, tenancy, provider)
	if err != nil {
		return nil, err
	}
	c.endpoint = "https://iaas." + region + ".oraclecloud.com"
	c.resourceType, c.resourcePath = "instance", "/20160918/instances/"
	return c, nil
}

// Bootstrap verifies the catalog region/key and principal/tenancy association.
// The provider must support concurrent signing; construction never reads credentials.
func newClient(region, regionKey, tenancy string, provider common.KeyProvider) (*Client, error) {
	if provider == nil || len(region) > 63 || !regionLabel.MatchString(region) ||
		len(regionKey) > 63 || !regionKeyLabel.MatchString(regionKey) ||
		len(tenancy) > 4096 || !tenancyPattern.MatchString(tenancy) {
		return nil, &Error{Code: InvalidConfiguration}
	}
	return &Client{region: region, regionKey: regionKey, tenancy: tenancy,
		signer: common.DefaultRequestSigner(provider), transport: newTransport()}, nil
}
