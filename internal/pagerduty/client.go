// Package pagerduty sends a single bounded Events v2 request. It does not
// authorize sends, retrieve credentials, persist outcomes or schedule retries.
package pagerduty

import (
	"net/http"
	"time"
)

type Client struct {
	endpoint  string
	transport *http.Transport
}

// Response contains only the observed status and parsed retry hint. HTTP 202
// means API acceptance, not durable recording or human receipt. Overflow means
// the server delay exceeds year 9999; the coordinator must hold it for review.
type Response struct {
	Status             int
	RetryAfter         time.Time
	RetryAfterOverflow bool
}

// New fixes the Events endpoint for the saved batch's PagerDuty region.
// Clients hold no credentials and may be used concurrently.
func New(region string) (*Client, error) {
	var host string
	switch region {
	case "US":
		host = "events.pagerduty.com"
	case "EU":
		host = "events.eu.pagerduty.com"
	default:
		return nil, &Error{Code: InvalidConfiguration}
	}
	return &Client{endpoint: "https://" + host + "/v2/enqueue", transport: newTransport()}, nil
}
