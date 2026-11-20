package reconciliation

import "testing"

func TestSetupPreviewPreparesWithoutTestFields(t *testing.T) {
	_, objects, lease := preparationFixture(t, nil)
	p := preparationFor(t, objects, setupRuntimeConfig(t, "PREVIEW", "NONE", 0), nil)
	if err := p.Initialize(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	if b := prepareSuccessfully(t, p, lease); b.ID() == "" {
		t.Fatal("missing baseline")
	}
}
