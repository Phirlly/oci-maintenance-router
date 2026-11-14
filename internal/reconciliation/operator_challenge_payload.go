package reconciliation

import "encoding/json"

func operatorChallengePayload(code string) string {
	message := struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}{Title: "OCI maintenance router operator test", Body: "Confirm this operator notification in your Resource Manager stack. Confirmation code: " + code}
	body, _ := json.Marshal(message)
	return string(body)
}
