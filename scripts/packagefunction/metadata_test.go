package main

import (
	"runtime/debug"
	"testing"
)

func validBuild() *debug.BuildInfo {
	return &debug.BuildInfo{
		GoVersion: "go1.27.3", Path: "oci-maintenance-router/cmd/router",
		Main: debug.Module{Path: "oci-maintenance-router", Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "GOARCH", Value: "amd64"},
			{Key: "GOAMD64", Value: "v1"}, {Key: "CGO_ENABLED", Value: "0"}, {Key: "-trimpath", Value: "true"}, {Key: "-buildmode", Value: "exe"}, {Key: "-compiler", Value: "gc"}},
		Deps: []*debug.Module{
			{Path: "github.com/fnproject/fdk-go", Version: "v0.1.26", Sum: "h1:I7eVC+OIc2W8X5Ai80GNNYE9qzhEYCPmmQdH9t4A5/c="},
			{Path: "github.com/gofrs/flock", Version: "v0.10.0", Sum: "h1:SHMXenfaB03KbroETaCMtbBg3Yn29v4w1r+tgy4ff4k="},
			{Path: "github.com/oracle/oci-go-sdk/v65", Version: "v65.129.0", Sum: "h1:keTL99Tc9APm7Cm3QQLtiHZ3KU9ZRkiXJzzgjzjpKRk="},
			{Path: "github.com/sony/gobreaker/v2", Version: "v2.4.0", Sum: "h1:g2KJRW1Ubty3+ZOcSEUN7K+REQJdN6yo6XvaML+jptg="},
			{Path: "github.com/youmark/pkcs8", Version: "v0.0.0-20240726163527-a2c0da244d78", Sum: "h1:ilQV1hzziu+LLM3zUTJ0trRztfwgjqKnBWNtSRkbmwM="},
			{Path: "golang.org/x/crypto", Version: "v0.56.0", Sum: "h1:GUh5Ii4J5jtcseSMiRqr1jXCNHoxjeV9Fmekc2oLy6Y="},
		},
	}
}

func TestBuildMetadataAcceptsOnlyReviewedGraph(t *testing.T) {
	if err := validateBuild(validBuild()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*debug.BuildInfo){
		"compiler":          func(b *debug.BuildInfo) { b.GoVersion = "go1.28.0" },
		"entrypoint":        func(b *debug.BuildInfo) { b.Path = "oci-maintenance-router/scripts/coverage" },
		"main module":       func(b *debug.BuildInfo) { b.Main.Path = "example" },
		"replacement":       func(b *debug.BuildInfo) { b.Deps[0].Replace = &debug.Module{Path: "local"} },
		"main replacement":  func(b *debug.BuildInfo) { b.Main.Replace = &debug.Module{Path: "local"} },
		"version":           func(b *debug.BuildInfo) { b.Deps[0].Version = "v0.1.27" },
		"checksum":          func(b *debug.BuildInfo) { b.Deps[0].Sum = "h1:incorrect" },
		"missing":           func(b *debug.BuildInfo) { b.Deps = b.Deps[1:] },
		"unknown":           func(b *debug.BuildInfo) { b.Deps[0].Path = "example.org/unknown" },
		"duplicate":         func(b *debug.BuildInfo) { b.Deps[1] = b.Deps[0] },
		"nil module":        func(b *debug.BuildInfo) { b.Deps[0] = nil },
		"wrong target":      func(b *debug.BuildInfo) { b.Settings[0].Value = "darwin" },
		"missing setting":   func(b *debug.BuildInfo) { b.Settings = b.Settings[1:] },
		"duplicate setting": func(b *debug.BuildInfo) { b.Settings = append(b.Settings, b.Settings[0]) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			b := validBuild()
			change(b)
			if validateBuild(b) == nil {
				t.Fatal("accepted unreviewed build")
			}
		})
	}
	if validateBuild(nil) == nil {
		t.Fatal("accepted absent metadata")
	}
}
