package reconciliation

import "encoding/json"

func diagnosticPayload(dedup, code string) string {
	message := struct {
		Action  string `json:"event_action"`
		Dedup   string `json:"dedup_key"`
		Payload struct {
			Summary  string `json:"summary"`
			Source   string `json:"source"`
			Severity string `json:"severity"`
			Details  struct {
				Purpose string `json:"purpose"`
				Code    string `json:"confirmation_code"`
			} `json:"custom_details"`
		} `json:"payload"`
	}{Action: "trigger", Dedup: dedup}
	message.Payload.Summary = "OCI maintenance router test notification"
	message.Payload.Source = "oci-maintenance-router"
	message.Payload.Severity = "info"
	message.Payload.Details.Purpose = "Confirm this test notification in your Resource Manager stack."
	message.Payload.Details.Code = code
	body, _ := json.Marshal(message)
	return string(body)
}
