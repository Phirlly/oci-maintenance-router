// Package ocistorage provides bounded conditional writes and reads of whole
// objects. It does not interpret ledger state or authorize notification delivery.
package ocistorage

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/oracle/oci-go-sdk/v65/common"
	"oci-maintenance-router/internal/configuration"
)

type Client struct {
	endpoint  string
	path      string
	signer    common.HTTPRequestSigner
	transport *http.Transport
}

// New fixes the endpoint and location for this client. Bootstrap must verify the
// OC1 deployment region, tenancy association and permissions first. The provider
// must be usable and safe for concurrent signing; no credentials are read here.
func New(region string, location configuration.LedgerLocation, provider common.KeyProvider) (*Client, error) {
	if provider == nil || len(region) > 63 || !regionLabel.MatchString(region) ||
		!safeName(location.Namespace) || !bucketName.MatchString(location.Bucket) ||
		location.Bucket == "." || location.Bucket == ".." {
		return nil, &Error{Code: InvalidConfiguration}
	}
	return &Client{
		endpoint: "https://objectstorage." + region + ".oraclecloud.com",
		path:     "/n/" + url.PathEscape(location.Namespace) + "/b/" + url.PathEscape(location.Bucket) + "/o/",
		signer:   common.DefaultRequestSigner(provider), transport: newTransport(),
	}, nil
}

var regionLabel = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)
var bucketName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)

func safeName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 1024 &&
		utf8.ValidString(name) && !strings.ContainsFunc(name, unicode.IsControl)
}
