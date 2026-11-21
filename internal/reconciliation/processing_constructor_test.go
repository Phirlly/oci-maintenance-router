package reconciliation

import (
	"context"
	"github.com/oracle/oci-go-sdk/v65/common"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ociownership"
	"strings"
	"testing"
)

func processingMySQL(t *testing.T) OwnershipClients {
	t.Helper()
	c, err := ociownership.NewMySQL("us-ashburn-1", "iad", "ocid1.tenancy.oc1..example", common.NewRawConfigurationProvider("synthetic", "synthetic", "us-ashburn-1", "synthetic", "synthetic", nil))
	if err != nil {
		t.Fatal(err)
	}
	return OwnershipClients{MySQL: map[string]*ociownership.Client{"us-ashburn-1": c}}
}

func processingCompute(t *testing.T) OwnershipClients {
	t.Helper()
	c, err := ociownership.NewCompute("us-ashburn-1", "iad", "ocid1.tenancy.oc1..example", common.NewRawConfigurationProvider("synthetic", "synthetic", "us-ashburn-1", "synthetic", "synthetic", nil))
	if err != nil {
		t.Fatal(err)
	}
	return OwnershipClients{Compute: map[string]*ociownership.Client{"us-ashburn-1": c}}
}

func processingEmptySource() announcementDetails {
	return announcementDetails{page: func(context.Context, string) (ociannouncements.Page, error) { return ociannouncements.Page{}, nil }, get: func(context.Context, string) (notices.Notice, error) {
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
	}}
}

func TestMaintenanceConstructorOwnsActualSameInputComponents(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	clients := processingMySQL(t)
	p, err := NewMaintenanceProcessor(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), clients)
	if err != nil || p == nil {
		t.Fatal("valid processor rejected", err)
	}
	if p.discovery == nil || p.selector == nil || p.observer == nil || p.policy == nil || p.builder == nil || p.mysql["us-ashburn-1"] == nil || p.current == nil || p.version == nil || p.senders["US"] == nil || p.senders["EU"] == nil || p.wait == nil || p.jitter == nil || p.token == nil {
		t.Fatal("missing owned component")
	}
	delete(clients.MySQL, "us-ashburn-1")
	f.config[0] = 'x'
	if p.mysql["us-ashburn-1"] == nil || p.discovery.configHash != p.selector.diagnostic.input.current.configHash {
		t.Fatal("caller mutation or inconsistent pins")
	}
}

func TestMaintenanceConstructorRejectsNonLiveAndMissingClients(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		p, err := NewMaintenanceProcessor(liveSchemaConfig(t, mode), []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
		if p != nil || err == nil {
			t.Fatal("non-LIVE accepted")
		}
	}
	for _, clients := range []OwnershipClients{{}, {MySQL: map[string]*ociownership.Client{}}, {MySQL: map[string]*ociownership.Client{"us-ashburn-1": nil}}} {
		p, err := NewMaintenanceProcessor(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), clients)
		if p != nil || err == nil {
			t.Fatal("missing regional client accepted")
		}
	}
}
