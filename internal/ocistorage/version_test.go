package ocistorage

import (
	"net/http"
	"strings"
	"testing"
)

func TestGetVersionPinsEscapedQueryAndValidatesResponse(t *testing.T) {
	const version = "opaque+/=?&version"
	for _, returned := range []string{version, "", "another-version", version + ", " + version} {
		t.Run(returned, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Query().Get("versionId") != version || len(r.URL.Query()) != 1 {
					t.Errorf("versioned request not pinned: %s", r.URL.RawQuery)
				}
				w.Header().Set("ETag", "stored")
				if returned != "" {
					w.Header().Set("version-id", returned)
				}
				_, _ = w.Write([]byte("pinned-configuration"))
			})
			got, err := c.GetVersion(budget(t), "config.json", version)
			if returned == version {
				if err != nil || string(got.Body) != "pinned-configuration" || got.ETag != "stored" {
					t.Fatalf("pinned read = %v, %v", got, err)
				}
			} else {
				wantError(t, err, InvalidResponse, 200, false)
				if len(got.Body) != 0 {
					t.Fatal("mismatched version returned usable data")
				}
			}
		})
	}
}

func TestGetVersionRejectsInvalidVersionBeforeDispatch(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) {
		t.Error("invalid version reached network")
	})
	for _, version := range []string{"", ".", "..", "line\nfeed", strings.Repeat("v", 1025), string([]byte{255})} {
		_, err := c.GetVersion(budget(t), "config.json", version)
		wantError(t, err, InvalidRequest, 0, false)
	}
}
