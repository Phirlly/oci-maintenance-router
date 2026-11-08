package notices

import (
	"strings"
	"testing"
)

func TestInvalidUTF8CannotChangeIdentityThroughReplacement(t *testing.T) {
	bad := string([]byte{0xff})
	for field, change := range map[string]func(*Record){
		"noticeId":                             func(r *Record) { r.NoticeID = bad },
		"announcementType":                     func(r *Record) { r.AnnouncementType = bad },
		"chainId":                              func(r *Record) { r.ChainID = text(bad) },
		"reference":                            func(r *Record) { r.Reference = text(bad) },
		"platform":                             func(r *Record) { r.Platform = text(bad) },
		"summary":                              func(r *Record) { r.Summary = text(bad) },
		"description":                          func(r *Record) { r.Description = text(bad) },
		"additionalInformation":                func(r *Record) { r.AdditionalInformation = text(bad) },
		"timeCreated":                          func(r *Record) { r.TimeCreated = text(bad) },
		"timeUpdated":                          func(r *Record) { r.TimeUpdated = text(bad) },
		"lifecycleState":                       func(r *Record) { r.LifecycleState = text(bad) },
		"services":                             func(r *Record) { r.Services[0] = bad },
		"times[0].type":                        func(r *Record) { r.Times[0].Type = text(bad) },
		"times[0].title":                       func(r *Record) { r.Times[0].Title = text(bad) },
		"times[0].value":                       func(r *Record) { r.Times[0].Value = text(bad) },
		"times[1].type":                        func(r *Record) { r.Times[1].Type = text(bad) },
		"times[1].title":                       func(r *Record) { r.Times[1].Title = text(bad) },
		"times[1].value":                       func(r *Record) { r.Times[1].Value = text(bad) },
		"resources.resourceId":                 func(r *Record) { r.Resources[0].ResourceID = bad },
		"resources.resourceName":               func(r *Record) { r.Resources[0].ResourceName = text(bad) },
		"resources.regionDisplay":              func(r *Record) { r.Resources[0].RegionDisplay = text(bad) },
		"resources.additionalProperties.name":  func(r *Record) { r.Resources[0].AdditionalProperties[0].Name = text(bad) },
		"resources.additionalProperties.value": func(r *Record) { r.Resources[0].AdditionalProperties[0].Value = text(bad) },
	} {
		t.Run(field, func(t *testing.T) {
			input := sample()
			change(&input)
			wantError(t, input, field, "invalid_utf8")
		})
	}
}

func TestErrorsDoNotExposeSourceValues(t *testing.T) {
	input := sample()
	input.NoticeID = "sensitive-source"
	_, err := Normalize(input)
	if err == nil || strings.Contains(err.Error(), "sensitive") || err.Error() != "noticeId: invalid" {
		t.Fatalf("unsafe error: %v", err)
	}
}

func FuzzNoticeText(f *testing.F) {
	for _, value := range []string{"", "🐬 <>&", "e\u0301", "\x00", string([]byte{0xff})} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		input := minimal()
		input.Summary = text(value)
		got, err := Normalize(input)
		if err != nil {
			if _, ok := err.(*Error); !ok {
				t.Fatalf("uncontrolled error: %T", err)
			}
			return
		}
		if *got.Snapshot().Summary != value || normalized(t, got.Snapshot()).Key() != got.Key() {
			t.Fatal("text changed or revision is not stable")
		}
	})
}
