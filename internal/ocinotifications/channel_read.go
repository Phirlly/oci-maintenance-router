package ocinotifications

import (
	"context"
	"net/http"
)

// GetChannel reads both identities freshly under one budget. These sequential
// observations are not atomic. The coordinator owns freshness and intent checks.
func (c *Client) GetChannel(ctx context.Context, topicID, subscriptionID string) (Channel, error) {
	if c == nil || !c.regionalID(topicID, topicPattern) || !c.regionalID(subscriptionID, subscriptionPattern) {
		return Channel{}, &Error{Code: InvalidRequest}
	}
	ctx, cancel, err := c.lifetime(ctx)
	if err != nil {
		return Channel{}, err
	}
	defer cancel()
	body, _, err := c.exchange(ctx, http.MethodGet, c.endpoint+"/20181201/topics/"+topicID, nil)
	if err != nil {
		return Channel{}, err
	}
	channel, err := c.decodeTopic(body, topicID)
	if err != nil {
		return Channel{}, readFailure(ctx, err)
	}
	body, _, err = c.exchange(ctx, http.MethodGet, channel.apiEndpoint+"/20181201/subscriptions/"+subscriptionID, nil)
	if err != nil {
		return Channel{}, err
	}
	channel, err = c.decodeSubscription(body, channel, subscriptionID)
	if ctx.Err() != nil {
		return Channel{}, failure(ctx, Canceled, 200, false)
	}
	if err != nil {
		return Channel{}, readFailure(ctx, err)
	}
	return channel, nil
}

func readFailure(ctx context.Context, err error) error {
	return failure(ctx, err.(*Error).Code, 200, false)
}
