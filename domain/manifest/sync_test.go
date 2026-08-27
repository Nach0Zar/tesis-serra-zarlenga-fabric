package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestEmbeddedManifestMatchesCanonical es la garantia de que la copia embebida
// no es una segunda fuente de verdad. El archivo canonico lo fija NET-2 en
// network/organizations-manifest.json y ADR-006 (punto 2) y ADR-010 (punto 4)
// lo declaran fuente unica de verdad de despliegue para los tres consumidores:
// material criptografico, colecciones y bootstrap del registro.
//
// Si este test falla, la copia esta desactualizada: refrescarla con
// `make sync-manifest` desde chaincode/ y revisar el impacto sobre el
// packageID versionado (ADR-008 punto 5, ADR-010 punto 4).
func TestEmbeddedManifestMatchesCanonical(t *testing.T) {
	// La ruta es una constante del paquete compuesta con la raiz del
	// repositorio, no una entrada del usuario: no hay inclusion de archivo por
	// variable que gosec deba vigilar aca.
	//nolint:gosec // ruta constante, relativa al repositorio
	canonical, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(CanonicalPath)))
	if err != nil {
		t.Fatalf("no se pudo leer el manifiesto canonico %s: %v", CanonicalPath, err)
	}

	if !bytes.Equal(canonical, organizationsManifestJSON) {
		t.Fatalf("la copia embebida de %s difiere del archivo canonico; "+
			"regenerarla con `make sync-manifest` (ADR-010, punto 4)", CanonicalPath)
	}
}
