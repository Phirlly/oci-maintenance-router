package ocimysql

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestFailedReadsCannotProduceReadyWork(t *testing.T) {
	key := generatedKey(t)
	p := testPolicy(t, "freeform", []string{compartment})
	for _, status := range []int{200, 401, 403, 404} {
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			io.WriteString(w, strings.Replace(minimal, dbID, dbID+"other", 1))
		})
		m, err := c.Get(budget(t), dbID)
		code := Service
		if status == 200 {
			code = InvalidResponse
		}
		wantError(t, err, code, status)
		wantEmpty(t, m)
		// Even accidental FOUND wrapping cannot turn a failed read into a recipient.
		result := p.Resolve(testNotice(t, dbID), false, map[string]routing.Lookup{dbID: {State: routing.Found, Metadata: m}})
		if len(result.Groups) != 0 || len(result.Decisions) != 1 || result.Decisions[0].Disposition != routing.Unresolved {
			t.Fatal("failed read produced ready work")
		}
	}
}
