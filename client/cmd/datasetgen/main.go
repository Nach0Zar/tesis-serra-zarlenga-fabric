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

	fmt.Fprintf(os.Stdout, "dataset=%s%c", result.DatasetPath, byte(10))
	fmt.Fprintf(os.Stdout, "manifest=%s%c", result.ManifestPath, byte(10))
	fmt.Fprintf(os.Stdout, "hash=%s%c", result.HashPath, byte(10))
	fmt.Fprintf(os.Stdout, "units=%d%c", result.Manifest.Dataset.Units, byte(10))
	fmt.Fprintf(os.Stdout, "sha256=%s%c", result.Manifest.Dataset.SHA256, byte(10))
}
