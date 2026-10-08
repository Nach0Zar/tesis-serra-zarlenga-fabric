// Package main expone el ejecutable CLI del cliente Fabric Gateway.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(app.RunContext(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}
