package ocinotifications

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func responseObject(body []byte, known ...string) (map[string]any, error) {
	value, err := strictJSON(body)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, &Error{Code: InvalidResponse}
	}
	for key := range object {
		for _, name := range known {
			if key != name && strings.EqualFold(key, name) {
				return nil, &Error{Code: InvalidResponse}
			}
		}
	}
	return object, nil
}
func field(object map[string]any, name string) string {
	value, _ := object[name].(string)
	return value
}

func (c *Client) decodeTopic(body []byte, id string) (Channel, error) {
	o, err := responseObject(body, "topicId", "compartmentId", "lifecycleState", "apiEndpoint")
	if err != nil {
		return Channel{}, err
	}
	origin, ok := c.topicOrigin(field(o, "apiEndpoint"))
	if field(o, "topicId") != id || !c.compartment(field(o, "compartmentId")) || field(o, "lifecycleState") == "" || !ok {
		return Channel{}, &Error{Code: InvalidResponse}
	}
	if field(o, "lifecycleState") != "ACTIVE" {
		return Channel{}, &Error{Code: InactiveChannel}
	}
	return Channel{client: c, apiEndpoint: origin, value: ChannelSnapshot{TopicID: id, TopicCompartmentID: field(o, "compartmentId")}}, nil
}
func (c *Client) decodeSubscription(body []byte, channel Channel, id string) (Channel, error) {
	o, err := responseObject(body, "id", "topicId", "compartmentId", "lifecycleState", "protocol", "endpoint")
	if err != nil {
		return Channel{}, err
	}
	endpoint := field(o, "endpoint")
	if field(o, "id") != id || field(o, "topicId") != channel.value.TopicID || !c.compartment(field(o, "compartmentId")) || field(o, "lifecycleState") == "" || field(o, "protocol") != "EMAIL" || !opaqueText(endpoint, 2048) || utf8.RuneCountInString(endpoint) > 512 {
		return Channel{}, &Error{Code: InvalidResponse}
	}
	if field(o, "lifecycleState") != "ACTIVE" {
		return Channel{}, &Error{Code: InactiveChannel}
	}
	channel.value.SubscriptionID = id
	channel.value.SubscriptionCompartmentID = field(o, "compartmentId")
	channel.value.Protocol = "EMAIL"
	channel.value.Endpoint = endpoint
	return channel, nil
}
func opaqueText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
