// Command packagestack assembles a local PREVIEW RM bundle; it never deploys.
package main

import "os"

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
