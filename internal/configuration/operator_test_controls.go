package configuration

import (
	"regexp"
	"unicode"
	"unicode/utf8"
)

var operatorReceiptPattern = regexp.MustCompile(`^OMR-OP-[0-9a-f]{32}$`)

func (c *Config) readOperatorTest(r objectReader) {
	value, exists := r.values["operatorTest"]
	if !exists {
		return
	}
	o := readObject(value, "operatorTest", r.err, []string{"requestId", "subscriptionId", "endpoint"}, []string{"receiptCode"})
	c.OperatorTest = &OperatorTestRequest{RequestID: o.text("requestId"), SubscriptionID: o.text("subscriptionId"), Endpoint: o.text("endpoint")}
	if _, exists := o.values["receiptCode"]; exists {
		c.OperatorTest.ReceiptCode = o.text("receiptCode")
	}
}

func (c Config) validateOperatorTest(catalog Catalog) error {
	o := c.OperatorTest
	if o == nil {
		return nil
	}
	if !baselinePattern.MatchString(o.RequestID) {
		return &Error{"operatorTest.requestId", "invalid"}
	}
	_, regional := regionalOCID(o.SubscriptionID, "onssubscription", c.DeploymentRegion, catalog.Regions[c.DeploymentRegion])
	if len(o.SubscriptionID) > 4096 || !regional {
		return &Error{"operatorTest.subscriptionId", "invalid"}
	}
	if len(c.OperatorTopicID) > 4096 {
		return &Error{"operatorTopicId", "invalid"}
	}
	// strictJSON has already validated UTF8. Preserve exact contact spelling.
	if o.Endpoint == "" || len(o.Endpoint) > 2048 || utf8.RuneCountInString(o.Endpoint) > 512 {
		return &Error{"operatorTest.endpoint", "invalid"}
	}
	for _, r := range o.Endpoint {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return &Error{"operatorTest.endpoint", "invalid"}
		}
	}
	if o.ReceiptCode != "" && !operatorReceiptPattern.MatchString(o.ReceiptCode) {
		return &Error{"operatorTest.receiptCode", "invalid"}
	}
	return nil
}
