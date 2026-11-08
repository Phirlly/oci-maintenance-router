package main

import (
	"os"

	fdk "github.com/fnproject/fdk-go"
	"oci-maintenance-router/internal/deployment"
)

func main() {
	fdk.Handle(deployment.New(os.Getenv))
}
