package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type dependency struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Sum     string `json:"sum"`
}
type noticePolicy struct {
	dependency
	files []string
}

// This is the reviewed, linked Linux graph, not every module in go.sum.
// A graph change requires reviewing licenses/advisories and this allowlist.
func reviewedDependencies() []noticePolicy {
	return []noticePolicy{
		{dependency{"github.com/fnproject/fdk-go", "v0.1.26", "h1:I7eVC+OIc2W8X5Ai80GNNYE9qzhEYCPmmQdH9t4A5/c="}, []string{"LICENSE", "NOTICE.txt", "THIRD_PARTY_LICENSES.txt"}},
		{dependency{"github.com/gofrs/flock", "v0.10.0", "h1:SHMXenfaB03KbroETaCMtbBg3Yn29v4w1r+tgy4ff4k="}, []string{"LICENSE"}},
		{dependency{"github.com/oracle/oci-go-sdk/v65", "v65.129.0", "h1:keTL99Tc9APm7Cm3QQLtiHZ3KU9ZRkiXJzzgjzjpKRk="}, []string{"LICENSE.txt", "NOTICE.txt", "THIRD_PARTY_LICENSES.txt"}},
		{dependency{"github.com/sony/gobreaker/v2", "v2.4.0", "h1:g2KJRW1Ubty3+ZOcSEUN7K+REQJdN6yo6XvaML+jptg="}, []string{"LICENSE"}},
		{dependency{"github.com/youmark/pkcs8", "v0.0.0-20240726163527-a2c0da244d78", "h1:ilQV1hzziu+LLM3zUTJ0trRztfwgjqKnBWNtSRkbmwM="}, []string{"LICENSE"}},
		{dependency{"golang.org/x/crypto", "v0.56.0", "h1:GUh5Ii4J5jtcseSMiRqr1jXCNHoxjeV9Fmekc2oLy6Y="}, []string{"LICENSE"}},
	}
}

func collectNotices(env environment) ([]entry, error) {
	var entries []entry
	for _, name := range []string{"LICENSE", "NOTICE"} {
		data, err := os.ReadFile(name)
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("missing or empty project notice: %s", name)
		}
		entries = append(entries, entry{"resources/notices/oci-maintenance-router/" + name, data})
	}
	for _, module := range reviewedDependencies() {
		directory := module.Path + "@" + module.Version
		for _, name := range module.files {
			data, err := os.ReadFile(filepath.Join(env.GOMODCACHE, directory, name))
			if err != nil || len(data) == 0 {
				return nil, fmt.Errorf("missing or empty production notice: %s/%s", directory, name)
			}
			entries = append(entries, entry{"resources/notices/" + directory + "/" + name, data})
		}
	}
	data, err := os.ReadFile(filepath.Join(env.GOROOT, "LICENSE"))
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("missing or empty Go license")
	}
	return append(entries, entry{"resources/notices/" + compilerVersion + "/LICENSE", data}), nil
}
