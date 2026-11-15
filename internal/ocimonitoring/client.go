// Package ocimonitoring publishes bounded historical router observations.
// It has no processing authority, persistence or retry policy.
package ocimonitoring

import (
	"net/http"
	"reflect"
	"regexp"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type Client struct {
	region, compartment, installation string
	signer                            common.HTTPRequestSigner
	transport                         *http.Transport
}

var regionPattern = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)
var tenancyPattern = regexp.MustCompile(`^ocid1\.tenancy\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var compartmentPattern = regexp.MustCompile(`^ocid1\.compartment\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var installationPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Observation contains only completed work and positively known saved state.
// Nil gauges mean unknown. A completed scan is never represented by zero.
type Observation struct {
	At                   time.Time
	CompletedScan        bool
	ReviewIssues         *int
	OperatorReportFailed *bool
}

func New(region, tenancy, compartment, installation string, provider common.KeyProvider) (*Client, error) {
	if len(region) > 63 || !regionPattern.MatchString(region) || len(tenancy) > 1024 || !tenancyPattern.MatchString(tenancy) || len(compartment) > 1024 || compartment != tenancy && !compartmentPattern.MatchString(compartment) || !installationPattern.MatchString(installation) || provider == nil {
		return nil, &Error{Code: "invalid_configuration"}
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil, &Error{Code: "invalid_configuration"}
		}
	}
	return &Client{region: region, compartment: compartment, installation: installation, signer: common.DefaultRequestSigner(provider), transport: newTransport()}, nil
}
