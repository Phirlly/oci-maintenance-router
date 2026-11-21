package ociownership

import (
	"io"
	"net/http"
	"sync"
	"testing"
)

func TestConcurrentReadsHaveIndependentMetadata(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, tagged) })
	ctx := budget(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			m, err := c.Get(ctx, dbID)
			if err != nil {
				t.Error(err)
				return
			}
			if *m.ID != dbID || m.FreeformTags["Owner"] != "payments" || m.DefinedTags["Operations"]["Owner"] != "payments" {
				t.Error("shared metadata")
			}
			*m.ID = "caller change"
			*m.CompartmentID = "caller change"
			m.FreeformTags["Owner"] = "changed"
			m.DefinedTags["Operations"]["Owner"] = "changed"
			m.DefinedTags["Other"]["object"].(map[string]any)["a"].([]any)[0] = "changed"
		})
	}
	wg.Wait()
	m, err := c.Get(ctx, dbID)
	if err != nil || m.ID == nil || *m.ID != dbID || m.CompartmentID == nil || *m.CompartmentID != compartment || m.DefinedTags["Other"]["object"].(map[string]any)["a"].([]any)[0] == "changed" {
		t.Fatal("later read retained caller mutation")
	}
}
