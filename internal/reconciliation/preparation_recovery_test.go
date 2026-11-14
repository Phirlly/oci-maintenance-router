package reconciliation

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationReusesPersistedTargetAfterInterruptedHeadWrite(t *testing.T) {
	p, s, l := preparationFixture(t, []notices.Notice{candidateNotice(t, 1, nil, true)})
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	s.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "REPLACE" && c.name == preparedHistoryName {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 503}
		}
		return nil
	}
	b, err := p.Prepare(leaseBudget(t), l)
	preparationFailed(t, b, err, "write_failed")
	var target string
	var original []byte
	for name, obj := range s.objects {
		if strings.HasPrefix(name, "baselines/") {
			target = name
			original = bytes.Clone(obj.Body)
		}
	}
	if target == "" || preparationHead(t, s, l).BaselineID() != "" {
		t.Fatal("interrupted preparation lost immutable target or advanced head")
	}
	s.before = nil
	p = preparationFor(t, s, preparationConfig(t), []notices.Notice{candidateNotice(t, 2, nil, true)})
	before := preparationCalls(s, "LIST", "")
	got := prepareSuccessfully(t, p, l)
	actual, _ := got.Encode()
	if !bytes.Equal(original, actual) || preparationCalls(s, "LIST", "") != before || preparationCalls(s, "CREATE", target) != 1 {
		t.Fatal("retry rebuilt orphan with later source/time")
	}
}

func TestPreparationConfirmsLostBaselineAndHeadResponses(t *testing.T) {
	for _, stage := range []string{"initialize", "baseline", "head"} {
		t.Run(stage, func(t *testing.T) {
			p, s, l := preparationFixture(t, nil)
			s.after = func(_ context.Context, c discoveryCall) error {
				match := stage == "initialize" && c.method == "CREATE" && c.name == preparedHistoryName || stage == "baseline" && c.method == "CREATE" && strings.HasPrefix(c.name, "baselines/") || stage == "head" && c.method == "REPLACE" && c.name == preparedHistoryName
				if match {
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return nil
			}
			if err := p.Initialize(leaseBudget(t), l); err != nil {
				t.Fatal(err)
			}
			b := prepareSuccessfully(t, p, l)
			if preparationHead(t, s, l).BaselineID() != b.ID() {
				t.Fatal("lost response failed to retain confirmed progress")
			}
			if preparationCalls(s, "CREATE", preparedHistoryName) != 1 || preparationCalls(s, "REPLACE", preparedHistoryName) != 1 || preparationCalls(s, "CREATE", "baselines/"+b.ID()+".json") != 1 {
				t.Fatal("lost response caused a blind retry")
			}
		})
	}
}

func TestPreparationRestartAfterHeadWritePreservesCompletedHistory(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	s.after = func(_ context.Context, c discoveryCall) error {
		if c.method == "REPLACE" && c.name == preparedHistoryName {
			s.seed("configuration/router.json", []byte("changed after write"))
		}
		return nil
	}
	b, err := p.Prepare(leaseBudget(t), l)
	preparationFailed(t, b, err, "configuration_changed")
	s.after = nil
	h := preparationHead(t, s, l)
	if h.BaselineID() == "" {
		t.Fatal("confirmed head was lost")
	}
	original := bytes.Clone(s.objects["baselines/"+h.BaselineID()+".json"].Body)
	p = preparationFor(t, s, preparationConfig(t), []notices.Notice{candidateNotice(t, 7, nil, true)})
	before := preparationCalls(s, "LIST", "")
	b = prepareSuccessfully(t, p, l)
	body, _ := b.Encode()
	if !bytes.Equal(original, body) || preparationCalls(s, "LIST", "") != before {
		t.Fatal("restart reset completed baseline")
	}
}
