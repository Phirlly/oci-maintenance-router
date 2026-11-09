// Package ociannouncements reads bounded OCI announcement pages and details.
// It does not classify notices, retry reads or grant scan or delivery authority.
package ociannouncements

import (
	"net/http"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type Client struct {
	endpoint  string
	tenancy   string
	signer    common.HTTPRequestSigner
	transport *http.Transport
}

// New fixes an OC1 region endpoint and tenancy-root scope. Bootstrap must verify
// region/principal/tenancy association. The provider must support concurrent
// signing; construction reads no credentials.
func New(region, tenancy string, provider common.KeyProvider) (*Client, error) {
	if provider == nil || len(region) > 63 || !regionLabel.MatchString(region) || !validID(tenancy, tenancyID) {
		return nil, &Error{Code: InvalidConfiguration}
	}
	return &Client{endpoint: "https://announcements." + region + ".oraclecloud.com", tenancy: tenancy,
		signer: common.DefaultRequestSigner(provider), transport: newTransport()}, nil
}
