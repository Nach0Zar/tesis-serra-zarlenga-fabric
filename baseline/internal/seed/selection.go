package seed

import (
	"errors"
	"fmt"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
)

const selectionSchemaVersion = "1.0.0"

// Selection acota el seed general sin alterar el bundle compartido. Las
// secuencias excluidas quedan ausentes para que RegisterUnit pueda medirse.
type Selection struct {
	SchemaVersion     string `json:"schemaVersion"`
	DatasetSHA256     string `json:"datasetSHA256"`
	ExcludedSequences []int  `json:"excludedSequences"`
}

// LoadSelection valida que la seleccion corresponda exactamente al bundle que
// se va a sembrar y que use una lista ordenada, unica y dentro de rango.
func LoadSelection(path, datasetSHA256 string, totalUnits int) (Selection, error) {
	var selection Selection
	if err := decodeStrictJSON(path, &selection); err != nil {
		return Selection{}, fmt.Errorf("leer seleccion del snapshot: %w", err)
	}
	if selection.SchemaVersion != selectionSchemaVersion {
		return Selection{}, fmt.Errorf("schemaVersion de seleccion incompatible: %q", selection.SchemaVersion)
	}
	if selection.DatasetSHA256 != datasetSHA256 {
		return Selection{}, errors.New("la seleccion no corresponde al SHA-256 del dataset")
	}
	previous := 0
	for _, sequence := range selection.ExcludedSequences {
		if sequence < 1 || sequence > totalUnits {
			return Selection{}, fmt.Errorf("secuencia excluida fuera de rango: %d", sequence)
		}
		if sequence <= previous {
			return Selection{}, errors.New("excludedSequences debe estar ordenado y no contener duplicados")
		}
		previous = sequence
	}
	return selection, nil
}

// ApplySelection conserva el orden del bundle y no modifica el slice recibido.
func ApplySelection(registrations []core.SeedRegistration, selection Selection) []core.SeedRegistration {
	excluded := make(map[int]struct{}, len(selection.ExcludedSequences))
	for _, sequence := range selection.ExcludedSequences {
		excluded[sequence] = struct{}{}
	}
	filtered := make([]core.SeedRegistration, 0, len(registrations)-len(excluded))
	for index, registration := range registrations {
		if _, skip := excluded[index+1]; !skip {
			filtered = append(filtered, registration)
		}
	}
	return filtered
}
