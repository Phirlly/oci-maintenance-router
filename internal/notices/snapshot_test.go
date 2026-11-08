package notices

import (
	"reflect"
	"testing"
)

func overwrite(record *Record) {
	record.NoticeID = "changed"
	record.AnnouncementType = "changed"
	for _, field := range []*string{record.ChainID, record.Reference, record.Platform,
		record.Summary, record.Description, record.AdditionalInformation,
		record.TimeCreated, record.TimeUpdated, record.LifecycleState} {
		*field = "changed"
	}
	record.Services[0] = "changed"
	for i := range record.Times {
		*record.Times[i].Type = "changed"
		*record.Times[i].Title = "changed"
		*record.Times[i].Value = "changed"
	}
	for i := range record.Resources {
		r := &record.Resources[i]
		r.ResourceID = "changed"
		*r.ResourceName = "changed"
		*r.RegionDisplay = "changed"
		for j := range r.AdditionalProperties {
			*r.AdditionalProperties[j].Name = "changed"
			*r.AdditionalProperties[j].Value = "changed"
		}
		r.AdditionalProperties = append(r.AdditionalProperties, Property{text("new"), text("new")})
	}
	record.Resources = append(record.Resources, Resource{ResourceID: "new"})
}

func TestSnapshotAndSourceCannotMutateNotice(t *testing.T) {
	input := sample()
	got := normalized(t, input)
	want := normalized(t, sample())
	overwrite(&input)
	snapshot := got.Snapshot()
	overwrite(&snapshot)
	if !reflect.DeepEqual(got.Snapshot(), want.Snapshot()) || got.Key() != want.Key() {
		t.Fatal("caller mutation changed the owned snapshot or revision")
	}
	key := got.Key()
	key.Digest = "changed"
	if got.Key().Digest == key.Digest {
		t.Fatal("key accessor exposed mutable identity")
	}
}

func TestNormalizationIsIdempotent(t *testing.T) {
	got := normalized(t, sample())
	again := normalized(t, got.Snapshot())
	if !reflect.DeepEqual(got.Snapshot(), again.Snapshot()) || got.Key() != again.Key() {
		t.Fatal("normalized snapshot changed on repeated normalization")
	}
}
