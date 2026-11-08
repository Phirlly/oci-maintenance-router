// Command coverage runs the repository test suite and enforces its coverage floor.
// It is maintainer tooling and must not be included in the Function artifact.
package main

import (
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, runTests)) }
