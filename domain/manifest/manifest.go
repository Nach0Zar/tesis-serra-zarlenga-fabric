// Package manifest expone el manifiesto fundacional de organizaciones que el
// chaincode embebe para resolver, en el bootstrap, la identidad de la
// organizacion regulatoria (ADR-010, punto 4).
//
// El manifiesto NO es una fuente de verdad de identidad en tiempo de ejecucion:
// tiene un unico punto de consumo (Init) y un unico momento de consumo (el
// bootstrap). Despues de Init, la resolucion de identidad es, sin excepcion,
// cid.GetMSPID() -> entrada del registro organizacion-establecimiento
// (ADR-003; ADR-010, "Por que esta forma de bootstrap no viola ADR-003").
package manifest

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// organizationsManifestJSON es una copia sincronizada de
// network/organizations-manifest.json, la fuente de verdad de despliegue que
// NET-2 fijo y que alimenta material criptografico, colecciones y bootstrap
// (ADR-006 punto 2; ADR-010 punto 4).
//
// La copia existe porque //go:embed no puede referenciar archivos fuera del
// directorio del paquete. No es una segunda fuente de verdad: sync_test.go
// falla si diverge del archivo canonico, de modo que la divergencia no puede
// llegar a merge. Para refrescarla: make sync-manifest (desde chaincode/).
//
//go:embed organizations-manifest.json
var organizationsManifestJSON []byte

// CanonicalPath es la ruta del archivo canonico, relativa a la raiz del
// repositorio. La copia embebida debe ser byte a byte identica.
const CanonicalPath = "network/organizations-manifest.json"

// Organization es una entrada del manifiesto fundacional. Solo se modelan los
// campos que el chaincode necesita; el esquema completo (hostnames, slugs) lo
// consumen los scripts de red.
type Organization struct {
	MSPID      string           `json:"mspId"`
	Slug       string           `json:"slug"`
	ID         string           `json:"id"`
	IDType     string           `json:"idType"`
	AgentType  domain.AgentType `json:"agentType"`
	Active     bool             `json:"active"`
	ClientRole string           `json:"clientRole"`
}

type foundationalManifest struct {
	SchemaVersion string         `json:"schemaVersion"`
	Organizations []Organization `json:"organizations"`
}

// ErrNoRegulator indica que el manifiesto embebido no declara ninguna
// organizacion con agentType REGULATOR. Es un error de construccion del
// paquete, no una condicion de negocio.
var ErrNoRegulator = errors.New("el manifiesto fundacional embebido no declara una organizacion REGULATOR")

// ErrMultipleRegulators indica que el manifiesto declara mas de un REGULATOR,
// lo que violaria la invariante de unicidad de ADR-010, punto 4.
var ErrMultipleRegulators = errors.New("el manifiesto fundacional embebido declara mas de una organizacion REGULATOR")

var (
	loadOnce sync.Once
	loaded   foundationalManifest
	loadErr  error
)

func load() (*foundationalManifest, error) {
	loadOnce.Do(func() {
		if err := json.Unmarshal(organizationsManifestJSON, &loaded); err != nil {
			loadErr = fmt.Errorf("manifiesto fundacional embebido invalido: %w", err)
		}
	})
	return &loaded, loadErr
}

// SchemaVersion devuelve la version de esquema del manifiesto embebido.
func SchemaVersion() (string, error) {
	m, err := load()
	if err != nil {
		return "", err
	}
	return m.SchemaVersion, nil
}

// Organizations devuelve las entradas declaradas por el manifiesto embebido.
func Organizations() ([]Organization, error) {
	m, err := load()
	if err != nil {
		return nil, err
	}
	out := make([]Organization, len(m.Organizations))
	copy(out, m.Organizations)
	return out, nil
}

// Regulator devuelve la unica organizacion declarada con agentType REGULATOR.
// Es el valor contra el que Init compara cid.GetMSPID() del invocador: el
// mspId regulatorio no viaja como argumento, porque aceptarlo dejaria la
// identidad del regulador a criterio de quien envia la propuesta (ADR-010,
// punto 4).
func Regulator() (Organization, error) {
	m, err := load()
	if err != nil {
		return Organization{}, err
	}

	var found Organization
	seen := 0
	for _, org := range m.Organizations {
		if org.AgentType == domain.AgentRegulator {
			found = org
			seen++
		}
	}

	switch seen {
	case 0:
		return Organization{}, ErrNoRegulator
	case 1:
		return found, nil
	default:
		return Organization{}, ErrMultipleRegulators
	}
}
