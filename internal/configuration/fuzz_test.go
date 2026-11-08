package configuration

import (
	"os"
	"testing"
)

func FuzzDecode(f *testing.F) {
	data, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"a":"\ud800"}`))
	f.Add([]byte(`{"a":1,"\u0061":2}`))
	f.Add([]byte(`[[[null]]]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := Decode(data, catalog())
		if err != nil {
			if _, ok := err.(*Error); !ok {
				t.Fatalf("uncontrolled error: %T", err)
			}
			if got.SchemaVersion != 0 {
				t.Fatal("rejected input returned partial configuration")
			}
		}
	})
}
