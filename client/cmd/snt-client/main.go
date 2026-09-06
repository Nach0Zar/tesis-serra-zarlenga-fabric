package main

import (
	"os"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}
