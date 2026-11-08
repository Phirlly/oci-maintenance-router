package delivery_test

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestResourceCountSplitsAtFifty(t *testing.T) {
	for _, count := range []int{50, 51, 101} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ids := make([]string, count)
			m := map[string]routing.Lookup{}
			for i := range ids {
				ids[i] = fmt.Sprintf("%s%03d", db, i)
				m[ids[i]] = metadata(ids[i], "payments")
			}
			n, b := notice(t, record(ids...)), builder(t, nil)
			got := build(t, b, n, m, ids...)
			if len(got.Drafts) != (count+49)/50 || len(got.Issues) != 0 {
				t.Fatalf("wrong chunks: %+v", got)
			}
			for i, d := range got.Drafts {
				want := min(50, count-i*50)
				s := d.Snapshot()
				if len(s.Resources) != want || len(s.PayloadWithoutRoutingKey) > 32768 || s.Resources[0].ResourceID != ids[i*50] {
					t.Fatal("incorrect batch boundary")
				}
			}
			assertAccounting(t, got, ids)
			for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
				ids[i], ids[j] = ids[j], ids[i]
			}
			if !reflect.DeepEqual(got, build(t, b, n, m, ids...)) {
				t.Fatal("selection order changed chunks")
			}
		})
	}
}

func TestEnvelopeByteLimitIncludesEscapingAndUTF8(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		encodedBytes int
	}{
		{"ascii", "", 0},
		{"escaped", "<&\u2028\u2029", 24},
		{"multibyte", "🐬é", 6},
	} {
		for _, extra := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, extra), func(t *testing.T) {
				r := record(db)
				r.Reference = text(tc.prefix + strings.Repeat("x", referenceRoom(t)-tc.encodedBytes+extra))
				got := build(t, builder(t, nil), notice(t, r), map[string]routing.Lookup{db: metadata(db, "payments")}, db)
				if extra == 0 {
					if s := single(t, got); len(s.PayloadWithoutRoutingKey) != 32768 {
						t.Fatalf("boundary = %d", len(s.PayloadWithoutRoutingKey))
					}
				} else if len(got.Drafts) != 0 || !reflect.DeepEqual(got.Issues, []delivery.CapacityIssue{{ResourceID: db, Code: "payload_too_large"}}) {
					t.Fatalf("oversize silently accepted/dropped: %+v", got)
				}
				assertAccounting(t, got, []string{db})
			})
		}
	}
}

func TestByteLimitSplitsBeforeCountLimit(t *testing.T) {
	ids := []string{db, db + "other"}
	r := record(ids...)
	r.Reference = text(strings.Repeat("x", referenceRoom(t)-len("other")))
	got := build(t, builder(t, nil), notice(t, r), map[string]routing.Lookup{db: metadata(db, "payments"), ids[1]: metadata(ids[1], "payments")}, ids...)
	if len(got.Drafts) != 2 || len(got.Issues) != 0 {
		t.Fatalf("byte split failed: %+v", got)
	}
	for _, d := range got.Drafts {
		if len(d.Snapshot().Resources) != 1 || len(d.Snapshot().PayloadWithoutRoutingKey) > 32768 {
			t.Fatal("oversize chunk")
		}
	}
	assertAccounting(t, got, ids)
}

func TestOversizedSingletonDoesNotDiscardUsablePeers(t *testing.T) {
	ids := []string{db + "a", db + "b" + strings.Repeat("x", 32768), db + "c"}
	m := map[string]routing.Lookup{}
	for _, id := range ids {
		m[id] = metadata(id, "payments")
	}
	got := build(t, builder(t, nil), notice(t, record(ids...)), m, ids...)
	if len(got.Drafts) != 2 || !reflect.DeepEqual(got.Issues, []delivery.CapacityIssue{{ResourceID: ids[1], Code: "payload_too_large"}}) {
		t.Fatalf("peer preservation failed: drafts %d, issues %d", len(got.Drafts), len(got.Issues))
	}
	assertAccounting(t, got, ids)
}

func TestOversizedSharedContextReportsEverySelectedResource(t *testing.T) {
	ids := []string{db, db + "other"}
	r := record(ids...)
	r.Times[0].Title = text(strings.Repeat("x", 100000))
	got := build(t, builder(t, nil), notice(t, r), map[string]routing.Lookup{db: metadata(db, "payments"), ids[1]: metadata(ids[1], "video")}, ids...)
	if len(got.Drafts) != 0 || len(got.Issues) != 2 {
		t.Fatal("oversized context was silently lost")
	}
	for _, issue := range got.Issues {
		if issue.Code != "payload_too_large" {
			t.Fatal("unsafe capacity reason")
		}
	}
	assertAccounting(t, got, ids)
}

func referenceRoom(t *testing.T) int {
	t.Helper()
	golden, err := os.ReadFile("testdata/payload.json")
	if err != nil {
		t.Fatal(err)
	}
	// Independent fixed envelope: replacing null with a string uses two fewer
	// bytes before its content. Digests remain fixed-width after content changes.
	return 32768 - len(bytes.TrimSuffix(golden, []byte("\n"))) + 2
}

func assertAccounting(t *testing.T, got delivery.Result, ids []string) {
	t.Helper()
	counts := map[string]int{}
	for _, d := range got.Drafts {
		for _, r := range d.Snapshot().Resources {
			counts[r.ResourceID]++
		}
	}
	for _, issue := range got.Issues {
		counts[issue.ResourceID]++
	}
	if len(counts) != len(ids) {
		t.Fatal("unexpected/missing resource")
	}
	for _, id := range ids {
		if counts[id] != 1 {
			t.Fatal("resource missing or counted twice")
		}
	}
}
