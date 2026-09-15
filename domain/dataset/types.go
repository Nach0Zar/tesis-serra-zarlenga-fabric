package dataset

const (
	// SchemaVersion versiona en conjunto el dataset y su manifiesto.
	SchemaVersion = "1.0.0"
	// GeneratorVersion identifica la implementacion que produjo el bundle.
	GeneratorVersion = "1.0.0"
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

	datasetSchemaID  = "urn:pfi-snt:synthetic-dataset:schema:1.0.0"
	manifestSchemaID = "urn:pfi-snt:synthetic-dataset-manifest:schema:1.0.0"
)

// UnitRef identifica una unidad de la misma forma que el contrato DES-5.
type UnitRef struct {
	GTIN        string `json:"gtin"`
	NumeroSerie string `json:"numeroSerie"`
}

// RegisterUnitRequest replica el request publico de RegisterUnit.
type RegisterUnitRequest struct {
	GTIN             string `json:"gtin"`
	NumeroSerie      string `json:"numeroSerie"`
	Lote             string `json:"lote"`
	FechaVencimiento string `json:"fechaVencimiento"`
}

// Registration describes the initial RegisterUnit operation.
type Registration struct {
	Operation    string              `json:"operation"`
	InvokerMSPID string              `json:"invokerMspId"`
	Request      RegisterUnitRequest `json:"request"`
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

// Dispatch describes one DispatchTransfer invocation.
type Dispatch struct {
	Operation    string              `json:"operation"`
	InvokerMSPID string              `json:"invokerMspId"`
	Request      UnitRef             `json:"request"`
	PrivateData  DispatchPrivateData `json:"privateData"`
}

// Receive describes the matching ReceiveTransfer invocation.
type Receive struct {
	Operation    string  `json:"operation"`
	InvokerMSPID string  `json:"invokerMspId"`
	Request      UnitRef `json:"request"`
}

// Dispense describes the terminal Dispense invocation.
type Dispense struct {
	Operation    string  `json:"operation"`
	InvokerMSPID string  `json:"invokerMspId"`
	Request      UnitRef `json:"request"`
}

// ValidTransfer conserva un despacho y su recepcion como una unica unidad
// conceptual de carga, aunque sean dos transacciones write (ADR-004; protocolo
// §3.4). RuleID y MatrixSchemaVersion provienen de domain.DecideTransfer.
type ValidTransfer struct {
	RuleID              string   `json:"ruleId"`
	MatrixSchemaVersion string   `json:"matrixSchemaVersion"`
	Dispatch            Dispatch `json:"dispatch"`
	Receive             Receive  `json:"receive"`
}

// ExpectedRejection describes a transfer that the domain matrix must reject.
type ExpectedRejection struct {
	DecisionKind      string   `json:"decisionKind"`
	RuleID            string   `json:"ruleId,omitempty"`
	Reason            string   `json:"reason"`
	ExpectedErrorCode string   `json:"expectedErrorCode"`
	Dispatch          Dispatch `json:"dispatch"`
}

// UnitScenario es una receta autocontenida. Los caminos felices terminan en
// Dispense; los de rechazo terminan en un Dispatch que la matriz debe denegar.
type UnitScenario struct {
	Sequence          int                `json:"sequence"`
	InitialState      string             `json:"initialState"`
	InitialCustodian  string             `json:"initialCustodian"`
	Registration      Registration       `json:"registration"`
	ValidTransfers    []ValidTransfer    `json:"validTransfers"`
	Dispense          *Dispense          `json:"dispense,omitempty"`
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
	OrganizationsManifestSchemaVersion string `json:"organizationsManifestSchemaVersion"`
}

// Metadata summarizes the generated workload and its digest.
type Metadata struct {
	File                     string `json:"file"`
	HashFile                 string `json:"hashFile"`
	SHA256                   string `json:"sha256"`
	Units                    int    `json:"units"`
	HappyPathUnits           int    `json:"happyPathUnits"`
	RejectionUnits           int    `json:"rejectionUnits"`
	ExplicitProhibitionCases int    `json:"explicitProhibitionCases"`
	DefaultDenyCases         int    `json:"defaultDenyCases"`
}

// Organization identifies an active custodial participant used by the workload.
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
