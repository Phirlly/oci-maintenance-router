package deployment

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func configureReceipts(t *testing.T, h *Handler, s Settings, objects *operationalStore, diagnostic, operator string) Settings {
	t.Helper()
	return configureOperational(t, h, s, objects, func(v map[string]any) {
		v["diagnostic"].(map[string]any)["receiptCode"] = diagnostic
		if channel, ok := v["operatorTest"].(map[string]any); ok {
			channel["receiptCode"] = operator
		}
	})
}

func receiptVerified(t *testing.T, outcome *TestOutcome) bool {
	t.Helper()
	body, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	value, exists := fields["receiptVerified"]
	if exists && value != true {
		t.Fatal("receipt confirmation must be a true boolean or omitted")
	}
	return value == true
}

func retainedReceiptHistory(objects *operationalStore) map[string]string {
	result := map[string]string{}
	for name, object := range objects.objects {
		if strings.HasPrefix(name, "control/diagnostics/") || strings.HasPrefix(name, "control/operator-challenges/") {
			result[name] = object.ETag + ":" + digestFixture(object.Body)
		}
	}
	return result
}

func requireReceiptReadOnly(t *testing.T, objects *operationalStore, history map[string]string, versions int) {
	t.Helper()
	if objects.version != versions+2 || !reflect.DeepEqual(retainedReceiptHistory(objects), history) {
		t.Fatal("confirmation changed history beyond the acquired/released worker lease")
	}
}

func requireReceiptPrivacy(t *testing.T, got OperationalResult, codes ...string) {
	t.Helper()
	body, _ := json.Marshal(got)
	for _, private := range append(codes, "operator@example.invalid", "vaultsecret", "OwnerToken", "Payload") {
		if private != "" && strings.Contains(string(body), private) {
			t.Fatal("receipt result exposed private input or state")
		}
	}
}
