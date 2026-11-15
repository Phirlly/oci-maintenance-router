package deployment

import (
	"encoding/json"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func TestActualClosureCodeSurvivesTerraformAndExecutesExactRequest(t *testing.T) {
	for _, size := range []int{32, 4096} {
		prefix := "ocid1.announcement.oc1.."
		id := prefix + strings.Repeat("a", size-len(prefix))
		f := newSavedReviewFixture(t, id, 1, false)
		rows := closurePages(t, f, "CLOSE_REVISION", f.notice.Key().Digest)
		body := renderProducedClosure(t, f, rows[len(rows)-1]["code"].(string))
		p, failure := loadPinned(operationalContext(t), f.s, f.objects)
		if failure.Status != "" {
			t.Fatal(failure)
		}
		c, err := configuration.Decode(body, p.catalog)
		if err != nil {
			t.Fatal(err)
		}
		if c.Recovery == nil || c.Recovery.NoticeID != id || c.Recovery.TargetID != f.notice.Key().Digest || c.Recovery.RoutingGeneration != p.generation {
			t.Fatal("renderer changed reviewed target", c.Recovery)
		}
		var rendered map[string]any
		if json.Unmarshal(body, &rendered) != nil {
			t.Fatal("invalid rendered config")
		}
		f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) {
			for k := range v {
				delete(v, k)
			}
			for k, value := range rendered {
				v[k] = value
			}
		})
		got := invokeOperational(t, f.h, closeInvocation(f.s, c.Recovery.RequestID))
		if got.Status != "closure-completed" || got.Recovery.RequestID != c.Recovery.RequestID {
			t.Fatal("actual produced intent failed closure", got)
		}
	}
}
