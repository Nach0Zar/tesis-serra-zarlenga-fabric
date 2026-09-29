// Package main provides the CLI entry point for run metadata validation.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/runmeta"
)

func main() {
	schemaPath := flag.String("schema", runmeta.DefaultSchemaPath, "contrato de metadatos a aplicar")
	flag.Parse()

	if err := run(*schemaPath, flag.Args(), os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "runmeta:", err)
		os.Exit(1)
	}
}

func run(schemaPath string, paths []string, stdout, stderr io.Writer) error {
	if len(paths) == 0 {
		return fmt.Errorf("indicar al menos un metadata.json a validar")
	}

	validator, err := runmeta.NewValidator(schemaPath)
	if err != nil {
		return err
	}

	rejected := 0
	for _, path := range paths {
		findings, err := validator.ValidateFile(path)
		if err != nil {
			return err
		}

		if len(findings) == 0 {
			if _, err := fmt.Fprintf(stdout, "ok\t%s\n", path); err != nil {
				return fmt.Errorf("escribir el resultado de %s: %w", path, err)
			}

			continue
		}

		rejected++
		if err := reportFindings(stderr, path, findings); err != nil {
			return err
		}
	}

	if rejected > 0 {
		return fmt.Errorf("%d de %d documentos no cumplen el contrato", rejected, len(paths))
	}

	return nil
}

func reportFindings(stderr io.Writer, path string, findings []string) error {
	if _, err := fmt.Fprintf(stderr, "rechazado\t%s\n", path); err != nil {
		return fmt.Errorf("escribir el resultado de %s: %w", path, err)
	}

	for _, finding := range findings {
		if _, err := fmt.Fprintf(stderr, "\t%s\n", finding); err != nil {
			return fmt.Errorf("escribir el resultado de %s: %w", path, err)
		}
	}

	return nil
}
