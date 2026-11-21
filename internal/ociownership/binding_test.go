package ociownership

import "testing"

func TestFixedReaderBindingDoesNotReadCredentials(t *testing.T) {
	for _, kind := range []string{"mysqldbsystem", "instance"} {
		constructor := NewMySQL
		if kind == "instance" {
			constructor = NewCompute
		}
		c, err := constructor("us-ashburn-1", "iad", "ocid1.tenancy.oc1..example", unusedProvider{})
		if err != nil || !c.BoundTo(kind, "us-ashburn-1", "iad", "ocid1.tenancy.oc1..example") {
			t.Fatal("matching binding rejected", err)
		}
		for _, args := range [][4]string{
			{"unknown", "us-ashburn-1", "iad", "ocid1.tenancy.oc1..example"},
			{kind, "us-phoenix-1", "iad", "ocid1.tenancy.oc1..example"},
			{kind, "us-ashburn-1", "phx", "ocid1.tenancy.oc1..example"},
			{kind, "us-ashburn-1", "iad", "ocid1.tenancy.oc1..other"},
		} {
			if c.BoundTo(args[0], args[1], args[2], args[3]) {
				t.Fatal("mismatched fixed binding accepted")
			}
		}
	}
	for _, c := range []*Client{nil, {}} {
		if c.BoundTo("", "", "", "") {
			t.Fatal("empty reader bound")
		}
	}
}
