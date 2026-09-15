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
	repoRoot := filepath.Join("..", "..")

	// La ruta no viene de una entrada externa: CanonicalPath es una constante
	// del paquete y repoRoot es relativo al propio archivo de test.
	// #nosec G304 -- ruta constante, no derivada de entrada del usuario
	canonical, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(CanonicalPath)))
	if err != nil {
		t.Fatalf("no se pudo leer el manifiesto canonico %s: %v", CanonicalPath, err)
	}

	if !bytes.Equal(canonical, organizationsManifestJSON) {
		t.Fatalf("la copia embebida de %s difiere del archivo canonico; "+
			"regenerarla con `make sync-manifest` (ADR-010, punto 4)", CanonicalPath)
	}
}
