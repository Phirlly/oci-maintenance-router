// Package ocimysql reads current DB-system ownership metadata. It does not
// access SQL, change resources, cache metadata, retry reads or authorize delivery.
package ocimysql

import (
	"net/http"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type Client struct {
	endpoint, region, regionKey, tenancy string
	signer                               common.HTTPRequestSigner
	transport                            *http.Transport
}

// New fixes the OC1 control-plane endpoint. Bootstrap must verify the catalog
// region/key and principal/tenancy association. The provider must support
// concurrent signing; construction does not read credentials.
func New(region, regionKey, tenancy string, provider common.KeyProvider) (*Client, error) {
	if provider == nil || len(region) > 63 || !regionLabel.MatchString(region) ||
		len(regionKey) > 63 || !regionKeyLabel.MatchString(regionKey) ||
		len(tenancy) > 4096 || !tenancyPattern.MatchString(tenancy) {
		return nil, &Error{Code: InvalidConfiguration}
	}
	return &Client{endpoint: "https://mysql." + region + ".ocp.oraclecloud.com",
		region: region, regionKey: regionKey, tenancy: tenancy,
		signer: common.DefaultRequestSigner(provider), transport: newTransport()}, nil
}
