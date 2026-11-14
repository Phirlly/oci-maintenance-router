package ocinotifications

import (
	"context"
	"net/http"
	"slices"
	"unicode/utf8"
)

const messageLimit = 64 << 10

// Publish broadcasts saved bytes once. A prior channel observation is not send
// authorization or current freshness: the coordinator must check intent, lease,
// controls and its persisted claim immediately before this call. Inputs must not
// be concurrently mutated. API acceptance is neither persistence nor human receipt.
func (c *Client) Publish(ctx context.Context, channel Channel, payload []byte) (Publication, error) {
	if c == nil || channel.client != c || channel.value.SubscriptionID == "" || channel.apiEndpoint == "" {
		return Publication{}, &Error{Code: InvalidRequest}
	}
	ctx, cancel, err := c.lifetime(ctx)
	if err != nil {
		return Publication{}, err
	}
	defer cancel()
	if len(payload) > messageLimit {
		return Publication{}, &Error{Code: TooLarge}
	}
	body := slices.Clone(payload)
	if !validMessage(body) {
		return Publication{}, &Error{Code: InvalidRequest}
	}
	response, status, err := c.exchange(ctx, http.MethodPost, channel.apiEndpoint+"/20181201/topics/"+channel.value.TopicID+"/messages", body)
	result := Publication{Status: status, MayHaveSent: status != 0}
	if err != nil {
		result.MayHaveSent = err.(*Error).MayHaveSent
		return result, err
	}
	o, err := responseObject(response, "messageId", "timeStamp")
	if ctx.Err() != nil {
		return result, failure(ctx, Canceled, status, true)
	}
	if err != nil || !opaqueText(field(o, "messageId"), 256) {
		return result, failure(ctx, InvalidResponse, status, true)
	}
	result.MessageID = field(o, "messageId")
	return result, nil
}

func validMessage(body []byte) bool {
	o, err := responseObject(body, "body", "title")
	if err != nil || len(o) != 2 {
		return false
	}
	text, bodyOK := o["body"].(string)
	title, titleOK := o["title"].(string)
	return bodyOK && text != "" && titleOK && utf8.RuneCountInString(title) <= 255
}
