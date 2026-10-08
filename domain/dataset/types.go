package dataset

const (
	// SchemaVersion versiona en conjunto el dataset y su manifiesto. La version
	// 2 cambia la forma de cada receta para declarar una preparacion ejecutable
	// comun a resultados exitosos y rechazos esperados.
	SchemaVersion = "2.0.0"
	// GeneratorVersion identifica la implementacion que produjo el bundle.
	GeneratorVersion = "2.0.0"
	// FixedSeed es la semilla unica exigida por CLI-3 y el protocolo de medicion.
	FixedSeed uint64 = 20260727
	// MinimumUnits es el piso experimental de docs/measurement-protocol.md §4.
	MinimumUnits = 50000

	// DatasetFileName is the stable dataset payload filename.
	DatasetFileName = "dataset.json"
	// ManifestFileName is the stable reproducibility manifest filename.
	ManifestFileName = "manifest.json"
	// HashFileName is the stable SHA-256 sidecar filename.
	HashFileName = "dataset.sha256"

	// DatasetSchemaID identifica el schema compatible con SchemaVersion.
	DatasetSchemaID = "urn:pfi-snt:synthetic-dataset:schema:2.0.0"
	// ManifestSchemaID identifica el schema del manifiesto compatible.
	ManifestSchemaID = "urn:pfi-snt:synthetic-dataset-manifest:schema:2.0.0"

	// RejectionUnauthorizedTransfer identifica rechazos derivados de la matriz.
	RejectionUnauthorizedTransfer = "UNAUTHORIZED_TRANSFER"
	// RejectionDuplicateIdentity identifica un segundo alta de GTIN+serie.
	RejectionDuplicateIdentity = "DUPLICATE_IDENTITY"
	// RejectionBlockingState identifica operaciones ordinarias bloqueadas por estado.
	RejectionBlockingState = "BLOCKING_STATE"
)

// OperationRequest contiene los campos publicos usados por las operaciones de
// una receta. El JSON Schema cierra, segun operation, la combinacion admisible.
type OperationRequest struct {
	GTIN             string `json:"gtin"`
	NumeroSerie      string `json:"numeroSerie"`
	Lote             string `json:"lote,omitempty"`
	FechaVencimiento string `json:"fechaVencimiento,omitempty"`
	Motivo           string `json:"motivo,omitempty"`
}

// DestinationPrivateData carries the declared transfer recipient.
type DestinationPrivateData struct {
	Destino string `json:"destino"`
}

// CommercialPrivateData carries the synthetic transfer documents.
type CommercialPrivateData struct {
	NumeroRemito  string `json:"numeroRemito"`
	NumeroFactura string `json:"numeroFactura"`
	Cantidad      int    `json:"cantidad"`
}

// DispatchPrivateData groups the transient fields required by DispatchTransfer.
type DispatchPrivateData struct {
	Destinatario DestinationPrivateData `json:"destinatario"`
	Commercial   CommercialPrivateData  `json:"commercial"`
}

// Invocation es una operacion concreta de la receta. RuleID y
// MatrixSchemaVersion solo aparecen en despachos de preparacion autorizados;
// TransitionID y StateMachineVersion solo aparecen en eventos de preparacion.
type Invocation struct {
	Operation           string               `json:"operation"`
	InvokerMSPID        string               `json:"invokerMspId"`
	Request             OperationRequest     `json:"request"`
	PrivateData         *DispatchPrivateData `json:"privateData,omitempty"`
	RuleID              string               `json:"ruleId,omitempty"`
	MatrixSchemaVersion string               `json:"matrixSchemaVersion,omitempty"`
	TransitionID        string               `json:"transitionId,omitempty"`
	StateMachineVersion string               `json:"stateMachineVersion,omitempty"`
}

// TransferDecision conserva la decision exacta de domain.DecideTransfer que
// fundamenta un rechazo ordinario de transferencia.
type TransferDecision struct {
	Kind                string `json:"kind"`
	RuleID              string `json:"ruleId,omitempty"`
	Reason              string `json:"reason"`
	MatrixSchemaVersion string `json:"matrixSchemaVersion"`
}

// ExpectedRejection declara la invocacion que debe fallar y la familia estable
// usada por EVAL-2 y EVAL-3 para seleccionar exactamente los mismos registros.
type ExpectedRejection struct {
	Category            string               `json:"category"`
	Operation           string               `json:"operation"`
	InvokerMSPID        string               `json:"invokerMspId"`
	Request             OperationRequest     `json:"request"`
	PrivateData         *DispatchPrivateData `json:"privateData,omitempty"`
	ExpectedErrorCode   string               `json:"expectedErrorCode"`
	TransferDecision    *TransferDecision    `json:"transferDecision,omitempty"`
	BlockingState       string               `json:"blockingState,omitempty"`
	StateMachineVersion string               `json:"stateMachineVersion,omitempty"`
}

// UnitScenario es una receta autocontenida. Preparation enumera exclusivamente
// las operaciones exitosas minimas previas; luego la receta declara un unico
// resultado esperado, exitoso o rechazado.
type UnitScenario struct {
	Sequence          int                `json:"sequence"`
	InitialState      string             `json:"initialState"`
	InitialCustodian  string             `json:"initialCustodian"`
	Preparation       []Invocation       `json:"preparation"`
	ExpectedSuccess   *Invocation        `json:"expectedSuccess,omitempty"`
	ExpectedRejection *ExpectedRejection `json:"expectedRejection,omitempty"`
}

// GeneratorMetadata identifies the implementation that produced the bundle.
type GeneratorMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Parameters records the reproducible generator inputs.
type Parameters struct {
	Units int `json:"units"`
}

// SourceMetadata records the versions of the embedded domain sources.
type SourceMetadata struct {
	TransferRulesetID                  string `json:"transferRulesetId"`
	TransferMatrixSchemaVersion        string `json:"transferMatrixSchemaVersion"`
	StateMachineVersion                string `json:"stateMachineVersion"`
	OrganizationsManifestSchemaVersion string `json:"organizationsManifestSchemaVersion"`
}

// Metadata summarizes the generated workload and its digest.
type Metadata struct {
	File                      string `json:"file"`
	HashFile                  string `json:"hashFile"`
	SHA256                    string `json:"sha256"`
	Units                     int    `json:"units"`
	HappyPathUnits            int    `json:"happyPathUnits"`
	RejectionUnits            int    `json:"rejectionUnits"`
	UnauthorizedTransferCases int    `json:"unauthorizedTransferCases"`
	DuplicateIdentityCases    int    `json:"duplicateIdentityCases"`
	BlockingStateCases        int    `json:"blockingStateCases"`
	ExplicitProhibitionCases  int    `json:"explicitProhibitionCases"`
	DefaultDenyCases          int    `json:"defaultDenyCases"`
}

// Organization identifies an active participant referenced by the workload.
type Organization struct {
	MSPID       string `json:"mspId"`
	CanonicalID string `json:"canonicalId"`
	AgentType   string `json:"agentType"`
	ClientRole  string `json:"clientRole"`
}

// Manifest registra todos los datos necesarios para reproducir y verificar el
// bundle sin incluir timestamps o paths absolutos que harian variar la salida.
type Manifest struct {
	Schema        string            `json:"$schema"`
	SchemaVersion string            `json:"schemaVersion"`
	Generator     GeneratorMetadata `json:"generator"`
	Seed          uint64            `json:"seed"`
	Parameters    Parameters        `json:"parameters"`
	Sources       SourceMetadata    `json:"sources"`
	Dataset       Metadata          `json:"dataset"`
	Organizations []Organization    `json:"organizations"`
}

// Result informa las tres rutas escritas y el manifiesto verificable.
type Result struct {
	DatasetPath  string
	ManifestPath string
	HashPath     string
	Manifest     Manifest
}
