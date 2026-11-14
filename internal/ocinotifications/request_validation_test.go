package ocinotifications

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestConstructorRejectsInvalidAndNilProviders(t *testing.T) {
	for _, v := range [][3]string{
		{"", "iad", tenancy}, {"us-x-1/path", "iad", tenancy},
		{"US-X-1", "iad", tenancy}, {strings.Repeat("a", 64), "iad", tenancy},
		{"us-x-1", "", tenancy}, {"us-x-1", "IAD", tenancy},
		{"us-x-1", "bad-key", tenancy}, {"us-x-1", strings.Repeat("a", 64), tenancy},
		{"us-x-1", "iad", ""}, {"us-x-1", "iad", compartment},
		{"us-x-1", "iad", "ocid1.tenancy.oc2..x"},
		{"us-x-1", "iad", tenancy + strings.Repeat("x", 4096)},
	} {
		c, err := New(v[0], v[1], v[2], &unusedProvider{})
		wantError(t, err, InvalidConfiguration, 0, false)
		if c != nil {
			t.Fatal("partial client")
		}
	}
	var absent *unusedProvider
	_, err := New("us-x-1", "iad", tenancy, absent)
	wantError(t, err, InvalidConfiguration, 0, false)
	_, err = New("us-x-1", "iad", tenancy, nil)
	wantError(t, err, InvalidConfiguration, 0, false)
}

func TestInvalidChannelReadNeverSigns(t *testing.T) {
	c, _ := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	for _, ids := range [][2]string{
		{"", subscriptionID}, {topicID, ""}, {subscriptionID, topicID},
		{strings.Replace(topicID, ".iad.", ".phx.", 1), subscriptionID},
		{topicID, strings.Replace(subscriptionID, ".iad.", ".us-phoenix-1.", 1)},
		{topicID + "/path", subscriptionID}, {topicID, subscriptionID + "?query"},
		{topicID + strings.Repeat("x", 4096), subscriptionID},
		{topicID, subscriptionID + strings.Repeat("x", 4096)},
	} {
		ch, err := c.GetChannel(budget(t), ids[0], ids[1])
		wantEmpty(t, ch)
		wantError(t, err, InvalidRequest, 0, false)
	}
	for _, client := range []*Client{nil, {}} {
		ch, err := client.GetChannel(budget(t), topicID, subscriptionID)
		wantEmpty(t, ch)
		wantError(t, err, InvalidRequest, 0, false)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		ch, err := c.GetChannel(ctx, topicID, subscriptionID)
		wantEmpty(t, ch)
		wantError(t, err, code, 0, false)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	ch, err := c.GetChannel(ctx, topicID, subscriptionID)
	wantEmpty(t, ch)
	wantError(t, err, DeadlineExceeded, 0, false)
	ctx, cancel = context.WithCancel(budget(t))
	cancel()
	ch, err = c.GetChannel(ctx, topicID, subscriptionID)
	wantEmpty(t, ch)
	wantError(t, err, Canceled, 0, false)
}
