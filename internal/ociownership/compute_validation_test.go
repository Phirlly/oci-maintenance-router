package ociownership

import (
	"strings"
	"testing"
)

func TestComputeInvalidConfigurationReturnsNoClient(t *testing.T) {
	for _, v := range [][3]string{{"bad/region", "iad", tenancy}, {"us-ashburn-1", "IAD", tenancy}, {"us-ashburn-1", "iad", "ocid1.tenancy.oc2..foreign"}} {
		c, err := NewCompute(v[0], v[1], v[2], unusedProvider{})
		wantError(t, err, InvalidConfiguration, 0)
		if c != nil {
			t.Fatal("partial client")
		}
	}
	c, err := NewCompute("us-ashburn-1", "iad", tenancy, nil)
	wantError(t, err, InvalidConfiguration, 0)
	if c != nil {
		t.Fatal("nil provider accepted")
	}
}

func TestComputeInvalidReadNeverSigns(t *testing.T) {
	c, err := NewCompute("us-ashburn-1", "iad", tenancy, unusedProvider{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", dbID, "ocid1.instanceconsoleconnection.oc1.iad.x", "ocid1.instance.oc2.iad.x", "ocid1.instance.oc1.phx.x", "ocid1.instance.oc1.us-phoenix-1.x", "ocid1.instance.oc1..x", "ocid1.instance.oc1.IAD.x", instanceID + "/path", instanceID + "?q", instanceID + "#fragment", instanceID + strings.Repeat("x", 4096)} {
		m, err := c.Get(budget(t), id)
		wantError(t, err, InvalidRequest, 0)
		wantEmpty(t, m)
	}
	mysql, err := NewMySQL("us-ashburn-1", "iad", tenancy, unusedProvider{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := mysql.Get(budget(t), instanceID)
	wantError(t, err, InvalidRequest, 0)
	wantEmpty(t, m)
}
