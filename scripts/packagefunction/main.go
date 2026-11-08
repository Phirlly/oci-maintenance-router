// Command packagefunction builds a local Function archive, never a cloud deployment.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, func(dir string) ([]entry, error) { return buildFunction(dir, executeGo) }))
}
