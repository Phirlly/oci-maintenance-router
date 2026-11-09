// Package ocisecrets retrieves bounded, version-specific Vault secret contents.
// It does not persist credentials, retry requests or authorize notification sends.
package ocisecrets

import (
	"net/http"
	"regexp"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type Client struct {
	endpoint  string
	region    string
	regionKey string
	signer    common.HTTPRequestSigner
	transport *http.Transport
}

// New fixes the OC1 endpoint. Bootstrap must verify the region/key association
// and secret access first. The provider must be usable for concurrent signing;
// construction does not read credentials.
func New(region, regionKey string, provider common.KeyProvider) (*Client, error) {
	if provider == nil || len(region) > 63 || !regionLabel.MatchString(region) ||
		len(regionKey) > 63 || !keyLabel.MatchString(regionKey) {
		return nil, &Error{Code: InvalidConfiguration}
	}
	return &Client{
		endpoint: "https://secrets.vaults." + region + ".oci.oraclecloud.com",
		region:   region, regionKey: regionKey,
		signer: common.DefaultRequestSigner(provider), transport: newTransport(),
	}, nil
}

var regionLabel = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)
var keyLabel = regexp.MustCompile(`^[a-z0-9]+$`)
