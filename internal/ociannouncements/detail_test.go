package ociannouncements

import (
	"strings"
	"testing"
)

const noticeID = "ocid1.announcement.oc1..example"
const minimal = `{"id":"ocid1.announcement.oc1..example","announcementType":"SCHEDULED_MAINTENANCE"}`
const detailed = `{
 "id":"ocid1.announcement.oc1..example","announcementType":"SCHEDULED_MAINTENANCE",
 "chainId":"example-chain","referenceTicketNumber":"EXAMPLE-001",
 "services":["Oracle HeatWave MySQL"],"platformType":"IAAS",
 "summary":"Synthetic scheduled maintenance","description":"Within the configured window.",
 "additionalInformation":"Illustrative only.",
 "timeOneType":"START_TIME","timeOneTitle":"Maintenance week starts","timeOneValue":"2026-01-16T00:00:00Z",
 "timeTwoType":"END_TIME","timeTwoTitle":"Maintenance week ends","timeTwoValue":"2026-01-22T23:59:59Z",
 "affectedResources":[{"resourceId":"ocid1.mysqldbsystem.oc1.iad.example","resourceName":"example-db",
 "region":"US East (Ashburn)","additionalProperties":[{"name":"label","value":"Region"},{"name":"value","value":"US East (Ashburn)"}]}],
 "timeCreated":"2026-01-01T10:00:00Z","timeUpdated":"2026-01-01T10:00:00.123456Z","lifecycleState":"INACTIVE"
}`

func TestDetailMapsWireToNormalizedNotice(t *testing.T) {
	n, err := decodeNotice([]byte(detailed), noticeID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Key().NoticeID != noticeID || n.Key().Digest != "da893badd39b21ff91f610032d735cbd596efeeaddcef222c316302a43eeeebf" {
		t.Fatalf("wrong normalized identity: %+v", n.Key())
	}
	r := n.Snapshot()
	if *r.TimeUpdated != "2026-01-01T10:00:00.123Z" || *r.LifecycleState != "INACTIVE" || len(r.Resources) != 1 {
		t.Fatal("context missing or unnormalized")
	}
}

func TestDetailPreservesUnknownLabelsAndOptionalPresence(t *testing.T) {
	body := strings.TrimSuffix(minimal, "}") + `,"announcementTypeFuture":"ignored","platformType":"NEW_PLATFORM","timeOneType":"NEW_TIME","lifecycleState":"NEW_STATE","description":"","summary":null,"services":null,"affectedResources":null,"future":{"number":1e999}}`
	n, err := decodeNotice([]byte(body), noticeID)
	if err != nil {
		t.Fatal(err)
	}
	r := n.Snapshot()
	if *r.Platform != "NEW_PLATFORM" || *r.Times[0].Type != "NEW_TIME" || *r.LifecycleState != "NEW_STATE" || r.Description == nil || *r.Description != "" || r.Summary != nil || len(r.Resources) != 0 {
		t.Fatal("presence or labels changed")
	}
	n, err = decodeNotice([]byte(strings.Replace(minimal, "SCHEDULED_MAINTENANCE", "NEW_TYPE", 1)), noticeID)
	if err != nil || n.Snapshot().AnnouncementType != "NEW_TYPE" {
		t.Fatal("unknown type lost")
	}
}

func TestDetailRejectsInvalidConsumedFieldsWithoutPartialNotice(t *testing.T) {
	cases := []string{
		"null", "[]", `{"id":"ocid1.announcement.oc1..other","announcementType":"X"}`,
		`{"id":"ocid1.announcement.oc1..example"}`,
		strings.Replace(minimal, `"SCHEDULED_MAINTENANCE"`, `null`, 1),
		strings.Replace(minimal, `"SCHEDULED_MAINTENANCE"`, `""`, 1),
		strings.Replace(minimal, `"id"`, `"ID"`, 1),
	}
	for _, field := range []string{"chainId", "referenceTicketNumber", "summary", "description", "additionalInformation", "timeOneType", "timeOneTitle", "timeOneValue", "timeTwoType", "timeTwoTitle", "timeTwoValue", "timeCreated", "timeUpdated", "lifecycleState", "platformType"} {
		cases = append(cases, strings.TrimSuffix(minimal, "}")+`,"`+field+`":false}`)
	}
	for _, suffix := range []string{
		`,"services":{}`, `,"services":[null]`, `,"services":[3]`, `,"affectedResources":{}`, `,"affectedResources":[null]`,
		`,"affectedResources":[{}]`, `,"affectedResources":[{"resourceId":""}]`, `,"affectedResources":[{"resourceId":false}]`,
		`,"affectedResources":[{"resourceId":"r","resourceName":1}]`, `,"affectedResources":[{"resourceId":"r","Region":"iad"}]`,
		`,"affectedResources":[{"resourceId":"r","additionalProperties":{}}]`, `,"affectedResources":[{"resourceId":"r","additionalProperties":[null]}]`,
		`,"affectedResources":[{"resourceId":"r","additionalProperties":[{}]}]`, `,"affectedResources":[{"resourceId":"r","additionalProperties":[{"name":"x"}]}]`,
		`,"affectedResources":[{"resourceId":"r","additionalProperties":[{"Name":"x","value":"v"}]}]`, `,"timeOneValue":"2026-01-01"`, `,"timeUpdated":"2026-01-01T00:00:00-00:00"`,
		`,"Summary":"wrong casing"`,
	} {
		cases = append(cases, strings.TrimSuffix(minimal, "}")+suffix+"}")
	}
	for _, body := range cases {
		n, err := decodeNotice([]byte(body), noticeID)
		if err == nil || n.Key().NoticeID != "" {
			t.Errorf("invalid detail accepted: %s", body)
		}
	}
}
