// Package main siembra la baseline con el bundle del dataset compartido.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/seed"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	defaultDirectory := os.Getenv("SNT_BASELINE_DATASET_DIR")
	if defaultDirectory == "" {
		defaultDirectory = "/dataset"
	}
	datasetDirectory := flag.String("dataset-dir", defaultDirectory, "directorio del bundle generado por CLI-3")
	selectionFile := flag.String("selection-file", "", "seleccion opcional de secuencias excluidas del snapshot")
	flag.Parse()

	databaseURL := os.Getenv("SNT_BASELINE_DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "snt-seed: SNT_BASELINE_DATABASE_URL is required")
		os.Exit(1)
	}

	started := time.Now()
	bundle, err := seed.LoadBundle(*datasetDirectory)
	if err != nil {
		fmt.Fprintln(os.Stderr, "snt-seed:", err)
		os.Exit(1)
	}
	registrations := bundle.Registrations
	skipped := 0
	if *selectionFile != "" {
		selection, selectionErr := seed.LoadSelection(*selectionFile, bundle.Hash, len(bundle.Registrations))
		if selectionErr != nil {
			fmt.Fprintln(os.Stderr, "snt-seed:", selectionErr)
			os.Exit(1)
		}
		registrations = seed.ApplySelection(bundle.Registrations, selection)
		skipped = len(bundle.Registrations) - len(registrations)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "snt-seed: configuracion PostgreSQL invalida:", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "snt-seed: PostgreSQL no disponible:", err)
		os.Exit(1)
	}

	result, err := core.NewStore(pool, core.Credentials{}).SeedSnapshot(
		ctx,
		bundle.Organizations,
		registrations,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "snt-seed:", err)
		os.Exit(1)
	}
	fmt.Printf(
		"seed=%d sha256=%s organizations=%d units=%d skipped=%d duration=%s\n",
		bundle.Manifest.Seed,
		bundle.Hash,
		result.Organizations,
		result.Units,
		skipped,
		time.Since(started).Round(time.Millisecond),
	)
}
