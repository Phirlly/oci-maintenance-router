package pagerduty

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestConcurrentSendsKeepIndependentBodiesAndCancellation(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil {
			t.Error("invalid request")
			return
		}
		var dedup, key string
		_ = json.Unmarshal(fields["dedup_key"], &dedup)
		_ = json.Unmarshal(fields["routing_key"], &key)
		if dedup == "synthetic-batch" {
			if key != syntheticKey {
				t.Error("first key crossed requests")
			}
			close(entered)
			<-r.Context().Done()
			close(stopped)
			return
		}
		if dedup != "second-batch" || key != strings.Repeat("z", 32) {
			t.Error("second identity crossed requests")
		}
		w.WriteHeader(202)
	})
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan outcome, 1)
	go func() { r, e := c.Send(ctx, []byte(savedEvent), []byte(syntheticKey)); done <- outcome{r, e} }()
	await(t, entered)
	second := strings.Replace(savedEvent, "synthetic-batch", "second-batch", 1)
	got, err := c.Send(budget(t), []byte(second), []byte(strings.Repeat("z", 32)))
	if err != nil || got.Status != 202 {
		t.Fatal("independent request failed")
	}
	cancel()
	first := receive(t, done)
	wantError(t, first.response, first.err, Canceled, true)
	await(t, stopped)
}
