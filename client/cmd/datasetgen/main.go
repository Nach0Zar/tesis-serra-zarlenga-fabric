// Package main provides the CLI entry point for synthetic dataset generation.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/dataset"
)

func main() {
	units := flag.Int("units", dataset.MinimumUnits, "cantidad de unidades (minimo 50000)")
	outputDir := flag.String("output-dir", "../build/dataset", "directorio del bundle generado")
	flag.Parse()

	result, err := dataset.Generate(dataset.Config{Units: *units, OutputDir: *outputDir})
	if err != nil {
		fmt.Fprintln(os.Stderr, "datasetgen:", err)
		os.Exit(1)
	}

	if _, err := fmt.Fprintf(
		os.Stdout,
		"dataset=%s%cmanifest=%s%chash=%s%cunits=%d%csha256=%s%c",
		result.DatasetPath,
		byte(10),
		result.ManifestPath,
		byte(10),
		result.HashPath,
		byte(10),
		result.Manifest.Dataset.Units,
		byte(10),
		result.Manifest.Dataset.SHA256,
		byte(10),
	); err != nil {
		fmt.Fprintln(os.Stderr, "datasetgen: escribir resultado:", err)
		os.Exit(1)
	}
}
