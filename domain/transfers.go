// Package domain es el paquete Go compartido por el chaincode y la baseline
// centralizada. Concentra las dos reglas de negocio que ADR-008 y ADR-012
// exigen que ambas implementaciones ejecuten literalmente con el mismo codigo:
// la decision de la matriz de transferencias autorizadas (DES-3) y la maquina
// de estados del medicamento (ADR-001).
//
// La paridad funcional queda garantizada por construccion y no por disciplina:
// no hay dos implementaciones de las reglas que mantener consistentes, hay una
// sola que ambos binarios importan (ADR-012, seccion 1).
package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// authorizedTransfersJSON embebe la matriz regulatoria en el binario en tiempo
// de compilacion (ADR-008, punto 1). Un binario dado evalua siempre la misma
// matriz, sin depender del filesystem del peer ni de estado externo.
//
//go:embed authorized-transfers.json
var authorizedTransfersJSON []byte

// AgentType es la categoria normativa de un establecimiento custodial.
// El catalogo lo fija DES-3 y ADR-010 lo deja explicitamente sin modificar:
// los agentType no custodiales (REGULATOR, FINANCIER) nunca son origen ni
// destino de una transferencia.
type AgentType string

// Catalogo custodial de DES-3 (domain/authorized-transfers.json, agentTypes).
const (
	AgentLaboratory        AgentType = "LABORATORY"
	AgentDistributor       AgentType = "DISTRIBUTOR"
	AgentLogisticsOperator AgentType = "LOGISTICS_OPERATOR"
	AgentDrugstore         AgentType = "DRUGSTORE"
	AgentPharmacy          AgentType = "PHARMACY"
	AgentHealthcare        AgentType = "HEALTHCARE_FACILITY"
)

// Catalogo no custodial de ADR-010, punto 1.
const (
	AgentRegulator AgentType = "REGULATOR"
	AgentFinancier AgentType = "FINANCIER"
)

// DefaultDenyReason es la razon de rechazo cuando el par no coincide ni con una
// regla autorizada ni con una prohibicion explicita (domain/README.md).
const DefaultDenyReason = "DEFAULT_DENY"

type agentTypeEntry struct {
	Code  AgentType `json:"code"`
	Label string    `json:"label"`
}

type authorizedTransfer struct {
	RuleID      string    `json:"id"`
	Origin      AgentType `json:"origin"`
	Destination AgentType `json:"destination"`
	Allowed     bool      `json:"allowed"`
	Rationale   string    `json:"rationale"`
}

type prohibitedTransfer struct {
	RuleID       string      `json:"id"`
	Origins      []AgentType `json:"origins"`
	Destinations []AgentType `json:"destinations"`
	Allowed      bool        `json:"allowed"`
	Reason       string      `json:"reason"`
}

type transferMatrix struct {
	SchemaVersion       string               `json:"schemaVersion"`
	RulesetID           string               `json:"rulesetId"`
	TransferScope       string               `json:"transferScope"`
	DefaultDecision     string               `json:"defaultDecision"`
	AgentTypes          []agentTypeEntry     `json:"agentTypes"`
	AuthorizedTransfers []authorizedTransfer `json:"authorizedTransfers"`
	ProhibitedTransfers []prohibitedTransfer `json:"prohibitedTransfers"`
}

// TransferDecision es el veredicto de la matriz para un par origen -> destino.
// Ante el mismo par, el chaincode y la baseline devuelven la misma decision, el
// mismo identificador de regla y la misma razon de rechazo (domain/README.md).
type TransferDecision struct {
	// Allowed indica si el par esta autorizado.
	Allowed bool
	// RuleID es el id de la regla que autorizo el par, o el id de la
	// prohibicion explicita que lo denego. Vacio cuando la denegacion es por
	// defaultDecision.
	RuleID string
	// Reason describe la causa del rechazo: el id de la prohibicion explicita
	// o DefaultDenyReason. Vacio cuando el par esta autorizado.
	Reason string
	// SchemaVersion es la version de la matriz que produjo la decision. Se
	// persiste junto al RuleID en el registro de operacion (ADR-008, punto 3)
	// y es lo que la recepcion contrasta contra su propia matriz embebida
	// (ADR-008, punto 5).
	SchemaVersion string
}

var (
	matrixOnce sync.Once
	matrixData transferMatrix
	matrixErr  error

	// authorizedIndex y custodialIndex se derivan una sola vez del JSON
	// embebido para que Decide sea O(1) sin recorrer la matriz por invocacion.
	authorizedIndex map[transferPair]authorizedTransfer
	custodialIndex  map[AgentType]struct{}
)

type transferPair struct {
	origin      AgentType
	destination AgentType
}

// loadMatrix parsea la matriz embebida una unica vez, conforme ADR-008 punto 1
// ("lo parsea una unica vez en la inicializacion del contrato").
func loadMatrix() (*transferMatrix, error) {
	matrixOnce.Do(func() {
		if err := json.Unmarshal(authorizedTransfersJSON, &matrixData); err != nil {
			matrixErr = fmt.Errorf("matriz de transferencias embebida invalida: %w", err)
			return
		}

		authorizedIndex = make(map[transferPair]authorizedTransfer, len(matrixData.AuthorizedTransfers))
		for _, rule := range matrixData.AuthorizedTransfers {
			if !rule.Allowed {
				continue
			}
			authorizedIndex[transferPair{rule.Origin, rule.Destination}] = rule
		}

		custodialIndex = make(map[AgentType]struct{}, len(matrixData.AgentTypes))
		for _, entry := range matrixData.AgentTypes {
			custodialIndex[entry.Code] = struct{}{}
		}
	})
	return &matrixData, matrixErr
}

// MatrixSchemaVersion devuelve la schemaVersion de la matriz embebida.
func MatrixSchemaVersion() (string, error) {
	m, err := loadMatrix()
	if err != nil {
		return "", err
	}
	return m.SchemaVersion, nil
}

// MatrixRulesetID devuelve el rulesetId de la matriz embebida.
func MatrixRulesetID() (string, error) {
	m, err := loadMatrix()
	if err != nil {
		return "", err
	}
	return m.RulesetID, nil
}

// IsCustodialAgentType indica si el agentType pertenece al catalogo custodial
// de DES-3. Los tipos de ADR-010 (REGULATOR, FINANCIER) devuelven false: nunca
// son origen ni destino de una transferencia ni pueden persistirse como
// custodio (ADR-010, punto 2).
func IsCustodialAgentType(t AgentType) (bool, error) {
	if _, err := loadMatrix(); err != nil {
		return false, err
	}
	_, ok := custodialIndex[t]
	return ok, nil
}

// DecideTransfer aplica el algoritmo de decision de domain/README.md sobre la
// matriz embebida:
//
//  1. coincidencia exacta en authorizedTransfers -> autorizar con el id de la regla;
//  2. coincidencia con una prohibicion explicita -> denegar con el id de la prohibicion;
//  3. en cualquier otro caso -> denegar por defaultDecision con razon DEFAULT_DENY.
//
// Es la unica funcion de decision del repositorio: ni el chaincode ni la
// baseline reimplementan pares mediante if o switch (domain/README.md,
// ADR-008 punto 2).
func DecideTransfer(origin, destination AgentType) (TransferDecision, error) {
	m, err := loadMatrix()
	if err != nil {
		return TransferDecision{}, err
	}

	if rule, ok := authorizedIndex[transferPair{origin, destination}]; ok {
		return TransferDecision{
			Allowed:       true,
			RuleID:        rule.RuleID,
			SchemaVersion: m.SchemaVersion,
		}, nil
	}

	for _, prohibition := range m.ProhibitedTransfers {
		if containsAgent(prohibition.Origins, origin) && containsAgent(prohibition.Destinations, destination) {
			return TransferDecision{
				Allowed:       false,
				RuleID:        prohibition.RuleID,
				Reason:        prohibition.RuleID,
				SchemaVersion: m.SchemaVersion,
			}, nil
		}
	}

	return TransferDecision{
		Allowed:       false,
		Reason:        DefaultDenyReason,
		SchemaVersion: m.SchemaVersion,
	}, nil
}

func containsAgent(list []AgentType, want AgentType) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
