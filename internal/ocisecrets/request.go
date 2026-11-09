package ocisecrets

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GetCurrent selects a version for new work. The caller must persist its version
// with the immutable delivery before sending; this method does not cache it.
func (c *Client) GetCurrent(ctx context.Context, secretID string) (Bundle, error) {
	return c.get(ctx, secretID, 0)
}

// GetVersion reads only the saved version. An unavailable version never triggers
// a CURRENT fallback or an automatic retry.
func (c *Client) GetVersion(ctx context.Context, secretID string, version int64) (Bundle, error) {
	if version <= 0 {
		return Bundle{}, &Error{Code: InvalidRequest}
	}
	return c.get(ctx, secretID, version)
}

func (c *Client) get(ctx context.Context, secretID string, version int64) (Bundle, error) {
	if c == nil || c.signer == nil || c.transport == nil || ctx == nil || !c.validSecretID(secretID) {
		return Bundle{}, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return Bundle{}, failure(ctx, Canceled, 0)
	}
	if _, ok := ctx.Deadline(); !ok {
		return Bundle{}, &Error{Code: DeadlineRequired}
	}
	ctx, cancel := context.WithTimeout(ctx, operationLimit)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/20190301/secretbundles/"+url.PathEscape(secretID), nil)
	if err != nil {
		return Bundle{}, &Error{Code: InvalidRequest}
	}
	query := url.Values{"stage": {"CURRENT"}}
	if version != 0 {
		query = url.Values{"versionNumber": {strconv.FormatInt(version, 10)}}
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Accept", "application/json")
	// Key-provider I/O is synchronous and not preemptible. Never abandon signing
	// in a goroutine or dispatch after a delayed signer exhausts the budget.
	if err = c.signer.Sign(req); err != nil {
		return Bundle{}, failure(ctx, Authentication, 0)
	}
	if ctx.Err() != nil {
		return Bundle{}, failure(ctx, Canceled, 0)
	}
	client, closeConnections := c.operationHTTP(ctx)
	defer closeConnections()
	resp, err := client.Do(req)
	if err != nil {
		return Bundle{}, failure(ctx, Transport, 0)
	}
	defer resp.Body.Close()
	// Conservative application acceptance: no partial or unexpected 2xx body.
	if resp.StatusCode != http.StatusOK {
		return Bundle{}, failure(ctx, Service, resp.StatusCode)
	}
	return readBundle(ctx, resp, secretID, version)
}

var idComponent = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (c *Client) validSecretID(value string) bool {
	if len(value) > 4096 {
		return false
	}
	parts := strings.Split(value, ".")
	if (len(parts) != 5 && len(parts) != 6) || parts[0] != "ocid1" || parts[1] != "vaultsecret" ||
		parts[2] != "oc1" || (parts[3] != c.region && parts[3] != c.regionKey) ||
		!idComponent.MatchString(parts[len(parts)-1]) {
		return false
	}
	return len(parts) == 5 || parts[4] == "" || idComponent.MatchString(parts[4])
}
