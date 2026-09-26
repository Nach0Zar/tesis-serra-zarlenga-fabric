// Package dataset defines and generates the deterministic synthetic workload shared by both backends.
package dataset

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	foundational "github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/manifest"
)

const (
	initialState                = "EN_LABORATORIO"
	explicitProhibitionDecision = "EXPLICIT_PROHIBITION"
	defaultDenyDecision         = "DEFAULT_DENY"
	expectedTransferError       = "TRANSFER_NOT_AUTHORIZED"
	expectedDuplicateError      = "UNIT_ALREADY_EXISTS"
	expectedBlockingError       = "INVALID_STATE_TRANSITION"
	generatorName               = "cli-3-dataset-generator"
)

// Config defines the requested bundle size and destination directory.
type Config struct {
	Units     int
	OutputDir string
}

func (c Config) validate() error {
	if c.Units < MinimumUnits {
		return fmt.Errorf("units debe ser al menos %d", MinimumUnits)
	}
	if strings.TrimSpace(c.OutputDir) == "" {
		return errors.New("output-dir es obligatorio")
	}
	return nil
}

type organization struct {
	MSPID      string
	ID         string
	IDType     string
	AgentType  domain.AgentType
	ClientRole string
}

func (o organization) canonicalID() string {
	return o.IDType + ":" + o.ID
}

type deniedCase struct {
	Origin      organization
	Destination organization
	Decision    domain.TransferDecision
	SetupPath   []organization
}

type ordinaryOperation struct {
	Name  string
	Event domain.Event
}

type blockingCase struct {
	State                 domain.State
	PreparationTransition domain.Transition
	PreparationOperation  string
	PreparationInvoker    organization
	RejectedOperation     ordinaryOperation
	SetupPath             []organization
	DispatchDestination   organization
}

type generationPlan struct {
	organizations       []organization
	participants        []organization
	happyPaths          [][]organization
	deniedCases         []deniedCase
	blockingCases       []blockingCase
	explicitDenials     int
	defaultDenials      int
	matrixSchemaVersion string
	matrixRulesetID     string
	manifestVersion     string
	stateMachineVersion string
	duplicateCaseCount  int
}

// Generate produce un unico bundle consumible por Fabric y baseline. La seed
// no es configurable: permitir dos valores reabriria la divergencia que CLI-3
// y el protocolo cierran con FixedSeed.
func Generate(config Config) (Result, error) {
	if err := config.validate(); err != nil {
		return Result{}, err
	}
	return generateBundle(config.OutputDir, config.Units)
}

func generateBundle(outputDir string, units int) (Result, error) {
	plan, err := newGenerationPlan()
	if err != nil {
		return Result{}, err
	}
	rejections := len(plan.deniedCases) + plan.duplicateCaseCount + len(plan.blockingCases)
	if units < rejections {
		return Result{}, fmt.Errorf("units=%d no alcanza para los %d casos de rechazo derivados", units, rejections)
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return Result{}, fmt.Errorf("crear output-dir: %w", err)
	}

	datasetPath := filepath.Join(outputDir, DatasetFileName)
	datasetHash, err := writeDataset(datasetPath, units, plan)
	if err != nil {
		return Result{}, err
	}

	manifest := buildManifest(units, datasetHash, plan)
	manifestPath := filepath.Join(outputDir, ManifestFileName)
	if err := writeJSON(manifestPath, manifest); err != nil {
		return Result{}, fmt.Errorf("escribir manifiesto: %w", err)
	}

	hashPath := filepath.Join(outputDir, HashFileName)
	hashLine := fmt.Sprintf("%s  %s%c", datasetHash, DatasetFileName, byte(10))
	if err := os.WriteFile(hashPath, []byte(hashLine), 0o600); err != nil {
		return Result{}, fmt.Errorf("escribir archivo SHA-256: %w", err)
	}

	return Result{
		DatasetPath:  datasetPath,
		ManifestPath: manifestPath,
		HashPath:     hashPath,
		Manifest:     manifest,
	}, nil
}

func newGenerationPlan() (generationPlan, error) {
	organizations, err := loadCustodialOrganizations()
	if err != nil {
		return generationPlan{}, err
	}
	graph, err := buildAuthorizedGraph(organizations)
	if err != nil {
		return generationPlan{}, err
	}

	labs := organizationsByType(organizations, domain.AgentLaboratory)
	if len(labs) == 0 {
		return generationPlan{}, errors.New("el manifiesto no declara un laboratorio custodial activo")
	}

	happyPaths := collectHappyPaths(labs, graph, len(organizations))
	if len(happyPaths) == 0 {
		return generationPlan{}, errors.New("la matriz y el manifiesto no permiten ningun camino laboratorio -> agente dispensador")
	}

	deniedCases, explicit, defaults, err := collectDeniedCases(organizations, labs, graph)
	if err != nil {
		return generationPlan{}, err
	}
	if explicit == 0 || defaults == 0 {
		return generationPlan{}, fmt.Errorf(
			"los rechazos derivados deben incluir prohibiciones explicitas y default deny (explicit=%d, default=%d)",
			explicit, defaults)
	}

	regulator, err := loadRegulator()
	if err != nil {
		return generationPlan{}, err
	}
	blockingCases, err := collectBlockingCases(organizations, labs, graph, regulator)
	if err != nil {
		return generationPlan{}, err
	}
	if len(blockingCases) == 0 {
		return generationPlan{}, errors.New("la maquina de estados no produjo casos bloqueantes")
	}

	matrixVersion, err := domain.MatrixSchemaVersion()
	if err != nil {
		return generationPlan{}, fmt.Errorf("leer schemaVersion de la matriz: %w", err)
	}
	rulesetID, err := domain.MatrixRulesetID()
	if err != nil {
		return generationPlan{}, fmt.Errorf("leer rulesetId de la matriz: %w", err)
	}
	manifestVersion, err := foundational.SchemaVersion()
	if err != nil {
		return generationPlan{}, fmt.Errorf("leer schemaVersion del manifiesto de organizaciones: %w", err)
	}

	participants := append(append([]organization(nil), organizations...), regulator)
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].MSPID < participants[j].MSPID
	})

	return generationPlan{
		organizations:       organizations,
		participants:        participants,
		happyPaths:          happyPaths,
		deniedCases:         deniedCases,
		blockingCases:       blockingCases,
		explicitDenials:     explicit,
		defaultDenials:      defaults,
		matrixSchemaVersion: matrixVersion,
		matrixRulesetID:     rulesetID,
		manifestVersion:     manifestVersion,
		stateMachineVersion: domain.StateMachineVersion,
		duplicateCaseCount:  1,
	}, nil
}

func loadCustodialOrganizations() ([]organization, error) {
	entries, err := foundational.Organizations()
	if err != nil {
		return nil, fmt.Errorf("leer manifiesto fundacional: %w", err)
	}
	organizations := make([]organization, 0, len(entries))
	for _, entry := range entries {
		custodial, err := domain.IsCustodialAgentType(entry.AgentType)
		if err != nil {
			return nil, fmt.Errorf("clasificar agentType %s: %w", entry.AgentType, err)
		}
		if !entry.Active || !custodial {
			continue
		}
		organizations = append(organizations, organizationFromManifest(entry))
	}
	sort.Slice(organizations, func(i, j int) bool {
		return organizations[i].MSPID < organizations[j].MSPID
	})
	return organizations, nil
}

func loadRegulator() (organization, error) {
	entry, err := foundational.Regulator()
	if err != nil {
		return organization{}, fmt.Errorf("leer regulador del manifiesto fundacional: %w", err)
	}
	if !entry.Active {
		return organization{}, errors.New("el regulador fundacional no esta activo")
	}
	return organizationFromManifest(entry), nil
}

func organizationFromManifest(entry foundational.Organization) organization {
	return organization{
		MSPID: entry.MSPID, ID: entry.ID, IDType: entry.IDType,
		AgentType: entry.AgentType, ClientRole: entry.ClientRole,
	}
}

func organizationsByType(all []organization, agentType domain.AgentType) []organization {
	var out []organization
	for _, org := range all {
		if org.AgentType == agentType {
			out = append(out, org)
		}
	}
	return out
}

func buildAuthorizedGraph(organizations []organization) (map[string][]organization, error) {
	graph := make(map[string][]organization, len(organizations))
	for _, origin := range organizations {
		for _, destination := range organizations {
			if origin.MSPID == destination.MSPID {
				continue
			}
			decision, err := domain.DecideTransfer(origin.AgentType, destination.AgentType)
			if err != nil {
				return nil, fmt.Errorf("evaluar %s -> %s: %w", origin.AgentType, destination.AgentType, err)
			}
			if decision.Allowed {
				graph[origin.MSPID] = append(graph[origin.MSPID], destination)
			}
		}
		sort.Slice(graph[origin.MSPID], func(i, j int) bool {
			return graph[origin.MSPID][i].MSPID < graph[origin.MSPID][j].MSPID
		})
	}
	return graph, nil
}

func collectHappyPaths(labs []organization, graph map[string][]organization, limit int) [][]organization {
	var paths [][]organization
	for _, lab := range labs {
		visited := map[string]bool{lab.MSPID: true}
		collectPathsDFS(lab, []organization{lab}, visited, graph, limit, &paths)
	}
	sort.Slice(paths, func(i, j int) bool {
		return pathKey(paths[i]) < pathKey(paths[j])
	})
	return paths
}

func collectPathsDFS(
	current organization,
	path []organization,
	visited map[string]bool,
	graph map[string][]organization,
	limit int,
	out *[][]organization,
) {
	if len(path) > 1 && isDispensingAgent(current.AgentType) {
		copyPath := append([]organization(nil), path...)
		*out = append(*out, copyPath)
	}
	if len(path) >= limit {
		return
	}
	for _, next := range graph[current.MSPID] {
		if visited[next.MSPID] {
			continue
		}
		visited[next.MSPID] = true
		collectPathsDFS(next, append(path, next), visited, graph, limit, out)
		delete(visited, next.MSPID)
	}
}

func isDispensingAgent(agentType domain.AgentType) bool {
	return agentType == domain.AgentPharmacy || agentType == domain.AgentHealthcare
}

func pathKey(path []organization) string {
	parts := make([]string, len(path))
	for i, org := range path {
		parts[i] = org.MSPID
	}
	return strings.Join(parts, "->")
}

func collectDeniedCases(
	organizations []organization,
	labs []organization,
	graph map[string][]organization,
) ([]deniedCase, int, int, error) {
	var cases []deniedCase
	explicit := 0
	defaults := 0
	for _, origin := range organizations {
		setupPath, found := shortestPathFromAny(labs, origin, graph)
		if !found {
			continue
		}
		for _, destination := range organizations {
			if origin.MSPID == destination.MSPID {
				continue
			}
			decision, err := domain.DecideTransfer(origin.AgentType, destination.AgentType)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("evaluar rechazo %s -> %s: %w", origin.AgentType, destination.AgentType, err)
			}
			if decision.Allowed {
				continue
			}
			if decision.RuleID == "" {
				defaults++
			} else {
				explicit++
			}
			cases = append(cases, deniedCase{
				Origin: origin, Destination: destination, Decision: decision,
				SetupPath: append([]organization(nil), setupPath...),
			})
		}
	}
	return cases, explicit, defaults, nil
}

func collectBlockingCases(
	organizations []organization,
	labs []organization,
	graph map[string][]organization,
	regulator organization,
) ([]blockingCase, error) {
	pharmacies := organizationsByType(organizations, domain.AgentPharmacy)
	if len(pharmacies) == 0 {
		return nil, errors.New("el manifiesto no declara una farmacia activa")
	}

	var pharmacy organization
	var setupPath []organization
	var dispatchDestination organization
	for _, candidate := range pharmacies {
		path, found := shortestPathFromAny(labs, candidate, graph)
		if !found || len(graph[candidate.MSPID]) == 0 {
			continue
		}
		pharmacy = candidate
		setupPath = path
		dispatchDestination = graph[candidate.MSPID][0]
		break
	}
	if pharmacy.MSPID == "" {
		return nil, errors.New("no hay una farmacia alcanzable con destino autorizado para los casos bloqueantes")
	}

	ordinary := []ordinaryOperation{
		{Name: "DispatchTransfer", Event: domain.EventDistribuirEslabonPosterior},
		{Name: "Dispense", Event: domain.EventDispensarPaciente},
	}
	transitions := domain.Transitions()
	var cases []blockingCase
	for _, state := range domain.States() {
		if !domain.IsBlockingState(state) {
			continue
		}
		transition, err := transitionFromCustodyTo(state, transitions)
		if err != nil {
			return nil, err
		}
		operation, invoker, err := preparationOperation(transition, pharmacy, regulator)
		if err != nil {
			return nil, err
		}
		for _, rejected := range ordinary {
			if _, declared := domain.LookupTransition(state, rejected.Event); declared {
				continue
			}
			cases = append(cases, blockingCase{
				State: state, PreparationTransition: transition,
				PreparationOperation: operation, PreparationInvoker: invoker,
				RejectedOperation: rejected, SetupPath: append([]organization(nil), setupPath...),
				DispatchDestination: dispatchDestination,
			})
		}
	}
	return cases, nil
}

func transitionFromCustodyTo(state domain.State, transitions []domain.Transition) (domain.Transition, error) {
	var found []domain.Transition
	for _, transition := range transitions {
		if transition.To != state {
			continue
		}
		for _, from := range transition.From {
			if from == domain.StateEnCustodia {
				found = append(found, transition)
				break
			}
		}
	}
	if len(found) != 1 {
		return domain.Transition{}, fmt.Errorf(
			"el estado bloqueante %s debe tener una unica preparacion directa desde EN_CUSTODIA; obtuvo %d",
			state, len(found))
	}
	return found[0], nil
}

// preparationOperation traduce eventos de la maquina compartida al nombre del
// contrato publico. No decide estados ni compatibilidades: ambas decisiones ya
// vienen de domain.Transitions y domain.LookupTransition.
func preparationOperation(
	transition domain.Transition,
	custodian organization,
	regulator organization,
) (string, organization, error) {
	var operation string
	var invoker organization
	var actor domain.Actor
	switch transition.Event {
	case domain.EventPonerEnCuarentena:
		operation, invoker, actor = "Quarantine", custodian, domain.ActorCurrentCustodian
	case domain.EventInformarVencimiento:
		operation, invoker, actor = "ReportExpired", custodian, domain.ActorCurrentCustodian
	case domain.EventInformarDeterioro:
		operation, invoker, actor = "ReportDamaged", custodian, domain.ActorCurrentCustodian
	case domain.EventRetirarMercado:
		operation, invoker, actor = "WithdrawFromMarket", regulator, domain.ActorANMAT
	case domain.EventProhibirProducto:
		operation, invoker, actor = "ProhibitProduct", regulator, domain.ActorANMAT
	case domain.EventDevolverProducto:
		operation, invoker, actor = "ReturnProduct", custodian, domain.ActorCurrentCustodian
	default:
		return "", organization{}, fmt.Errorf(
			"el evento %s que prepara %s no tiene operacion publica mapeada",
			transition.Event, transition.To)
	}
	if !transition.AllowsActor(actor) {
		return "", organization{}, fmt.Errorf(
			"la transicion %s no habilita al actor %s elegido para %s",
			transition.ID, actor, operation)
	}
	return operation, invoker, nil
}

func shortestPathFromAny(
	starts []organization,
	target organization,
	graph map[string][]organization,
) ([]organization, bool) {
	var candidates [][]organization
	for _, start := range starts {
		if path, found := shortestPath(start, target, graph); found {
			candidates = append(candidates, path)
		}
	}
	if len(candidates) == 0 {
		return nil, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i]) != len(candidates[j]) {
			return len(candidates[i]) < len(candidates[j])
		}
		return pathKey(candidates[i]) < pathKey(candidates[j])
	})
	return candidates[0], true
}

func shortestPath(start, target organization, graph map[string][]organization) ([]organization, bool) {
	if start.MSPID == target.MSPID {
		return []organization{start}, true
	}
	queue := [][]organization{{start}}
	visited := map[string]bool{start.MSPID: true}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		current := path[len(path)-1]
		for _, next := range graph[current.MSPID] {
			if visited[next.MSPID] {
				continue
			}
			nextPath := append(append([]organization(nil), path...), next)
			if next.MSPID == target.MSPID {
				return nextPath, true
			}
			visited[next.MSPID] = true
			queue = append(queue, nextPath)
		}
	}
	return nil, false
}

func writeDataset(path string, units int, plan generationPlan) (string, error) {
	// #nosec G304 -- path is the output location explicitly selected by the CLI caller.
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("crear dataset: %w", err)
	}
	hasher := sha256.New()
	writer := bufio.NewWriterSize(io.MultiWriter(file, hasher), 256*1024)

	writeLine := func(value string) error {
		_, err := fmt.Fprintln(writer, value)
		return err
	}
	if err := writeLine("{"); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("escribir cabecera del dataset: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "  %q: %q,%c", "$schema", DatasetSchemaID, byte(10)); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("escribir schema del dataset: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "  %q: %q,%c", "schemaVersion", SchemaVersion, byte(10)); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("escribir version del dataset: %w", err)
	}
	if err := writeLine("  \"units\": ["); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("escribir lista del dataset: %w", err)
	}

	for index := 0; index < units; index++ {
		unit, err := buildUnitScenario(index, plan)
		if err != nil {
			_ = file.Close()
			return "", err
		}
		encoded, err := json.Marshal(unit)
		if err != nil {
			_ = file.Close()
			return "", fmt.Errorf("serializar unidad %d: %w", index+1, err)
		}
		if index > 0 {
			if err := writer.WriteByte(','); err != nil {
				_ = file.Close()
				return "", fmt.Errorf("escribir separador de unidad: %w", err)
			}
			if err := writer.WriteByte(byte(10)); err != nil {
				_ = file.Close()
				return "", fmt.Errorf("escribir separador de unidad: %w", err)
			}
		}
		if _, err := writer.WriteString("    "); err != nil {
			_ = file.Close()
			return "", fmt.Errorf("escribir unidad: %w", err)
		}
		if _, err := writer.Write(encoded); err != nil {
			_ = file.Close()
			return "", fmt.Errorf("escribir unidad %d: %w", index+1, err)
		}
	}
	if err := writer.WriteByte(byte(10)); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("cerrar lista del dataset: %w", err)
	}
	if err := writeLine("  ]"); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("cerrar lista del dataset: %w", err)
	}
	if err := writeLine("}"); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("cerrar dataset: %w", err)
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("flush del dataset: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("cerrar archivo del dataset: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func buildUnitScenario(index int, plan generationPlan) (UnitScenario, error) {
	sequence := index + 1
	gtin, serial, lot, expiration := identifiersFor(sequence)
	registerRequest := OperationRequest{
		GTIN: gtin, NumeroSerie: serial, Lote: lot, FechaVencimiento: expiration,
	}
	ref := OperationRequest{GTIN: gtin, NumeroSerie: serial}

	deniedEnd := len(plan.deniedCases)
	duplicateEnd := deniedEnd + plan.duplicateCaseCount
	blockingEnd := duplicateEnd + len(plan.blockingCases)

	switch {
	case index < deniedEnd:
		return buildTransferRejection(sequence, registerRequest, ref, plan.deniedCases[index])
	case index < duplicateEnd:
		return buildDuplicateRejection(sequence, registerRequest, plan)
	case index < blockingEnd:
		return buildBlockingRejection(sequence, registerRequest, ref, plan.blockingCases[index-duplicateEnd])
	default:
		// #nosec G115 -- sequence is index+1 and therefore strictly positive.
		sequence64 := uint64(sequence)
		rng := splitMix64{state: FixedSeed + sequence64*0x9e3779b97f4a7c15}
		path := plan.happyPaths[rng.intn(len(plan.happyPaths))]
		preparation, err := makePreparation(sequence, registerRequest, path)
		if err != nil {
			return UnitScenario{}, err
		}
		final := path[len(path)-1]
		return UnitScenario{
			Sequence: sequence, InitialState: initialState, InitialCustodian: path[0].canonicalID(),
			Preparation: preparation,
			ExpectedSuccess: &Invocation{
				Operation: "Dispense", InvokerMSPID: final.MSPID, Request: ref,
			},
		}, nil
	}
}

func buildTransferRejection(
	sequence int,
	registerRequest OperationRequest,
	ref OperationRequest,
	rejected deniedCase,
) (UnitScenario, error) {
	preparation, err := makePreparation(sequence, registerRequest, rejected.SetupPath)
	if err != nil {
		return UnitScenario{}, err
	}
	kind := explicitProhibitionDecision
	if rejected.Decision.RuleID == "" {
		kind = defaultDenyDecision
	}
	return UnitScenario{
		Sequence: sequence, InitialState: initialState,
		InitialCustodian: rejected.SetupPath[0].canonicalID(), Preparation: preparation,
		ExpectedRejection: &ExpectedRejection{
			Category: RejectionUnauthorizedTransfer, Operation: "DispatchTransfer",
			InvokerMSPID: rejected.Origin.MSPID, Request: ref,
			PrivateData:       makeDispatchPrivateData(sequence, len(rejected.SetupPath), rejected.Destination),
			ExpectedErrorCode: expectedTransferError,
			TransferDecision: &TransferDecision{
				Kind: kind, RuleID: rejected.Decision.RuleID, Reason: rejected.Decision.Reason,
				MatrixSchemaVersion: rejected.Decision.SchemaVersion,
			},
		},
	}, nil
}

func buildDuplicateRejection(
	sequence int,
	registerRequest OperationRequest,
	plan generationPlan,
) (UnitScenario, error) {
	labs := organizationsByType(plan.organizations, domain.AgentLaboratory)
	if len(labs) == 0 {
		return UnitScenario{}, errors.New("no hay laboratorio para preparar el duplicado")
	}
	lab := labs[0]
	preparation := []Invocation{{
		Operation: "RegisterUnit", InvokerMSPID: lab.MSPID, Request: registerRequest,
	}}
	return UnitScenario{
		Sequence: sequence, InitialState: initialState,
		InitialCustodian: lab.canonicalID(), Preparation: preparation,
		ExpectedRejection: &ExpectedRejection{
			Category: RejectionDuplicateIdentity, Operation: "RegisterUnit",
			InvokerMSPID: lab.MSPID, Request: registerRequest,
			ExpectedErrorCode: expectedDuplicateError,
		},
	}, nil
}

func buildBlockingRejection(
	sequence int,
	registerRequest OperationRequest,
	ref OperationRequest,
	blocked blockingCase,
) (UnitScenario, error) {
	preparation, err := makePreparation(sequence, registerRequest, blocked.SetupPath)
	if err != nil {
		return UnitScenario{}, err
	}
	preparation = append(preparation, Invocation{
		Operation: blocked.PreparationOperation, InvokerMSPID: blocked.PreparationInvoker.MSPID,
		Request: OperationRequest{
			GTIN: ref.GTIN, NumeroSerie: ref.NumeroSerie,
			Motivo: fmt.Sprintf("Preparacion deterministica del estado %s.", blocked.State),
		},
		TransitionID:        blocked.PreparationTransition.ID,
		StateMachineVersion: domain.StateMachineVersion,
	})

	rejection := &ExpectedRejection{
		Category: RejectionBlockingState, Operation: blocked.RejectedOperation.Name,
		InvokerMSPID: blocked.SetupPath[len(blocked.SetupPath)-1].MSPID, Request: ref,
		ExpectedErrorCode: expectedBlockingError, BlockingState: string(blocked.State),
		StateMachineVersion: domain.StateMachineVersion,
	}
	if blocked.RejectedOperation.Name == "DispatchTransfer" {
		rejection.PrivateData = makeDispatchPrivateData(
			sequence, len(blocked.SetupPath)+1, blocked.DispatchDestination)
	}

	return UnitScenario{
		Sequence: sequence, InitialState: initialState,
		InitialCustodian: blocked.SetupPath[0].canonicalID(), Preparation: preparation,
		ExpectedRejection: rejection,
	}, nil
}

func makePreparation(
	unitSequence int,
	registerRequest OperationRequest,
	path []organization,
) ([]Invocation, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("la unidad %d no tiene laboratorio de origen", unitSequence)
	}
	preparation := []Invocation{{
		Operation: "RegisterUnit", InvokerMSPID: path[0].MSPID, Request: registerRequest,
	}}
	ref := OperationRequest{GTIN: registerRequest.GTIN, NumeroSerie: registerRequest.NumeroSerie}
	for edge := 0; edge+1 < len(path); edge++ {
		transfer, err := makeValidTransferInvocations(
			unitSequence, edge+1, ref, path[edge], path[edge+1])
		if err != nil {
			return nil, err
		}
		preparation = append(preparation, transfer...)
	}
	return preparation, nil
}

func makeValidTransferInvocations(
	unitSequence int,
	transferSequence int,
	ref OperationRequest,
	origin organization,
	destination organization,
) ([]Invocation, error) {
	decision, err := domain.DecideTransfer(origin.AgentType, destination.AgentType)
	if err != nil {
		return nil, fmt.Errorf("evaluar transferencia valida: %w", err)
	}
	if !decision.Allowed {
		return nil, fmt.Errorf("el plan produjo un par no autorizado %s -> %s", origin.AgentType, destination.AgentType)
	}
	return []Invocation{
		{
			Operation: "DispatchTransfer", InvokerMSPID: origin.MSPID, Request: ref,
			PrivateData: makeDispatchPrivateData(unitSequence, transferSequence, destination),
			RuleID:      decision.RuleID, MatrixSchemaVersion: decision.SchemaVersion,
		},
		{Operation: "ReceiveTransfer", InvokerMSPID: destination.MSPID, Request: ref},
	}, nil
}

func makeDispatchPrivateData(
	unitSequence int,
	transferSequence int,
	destination organization,
) *DispatchPrivateData {
	return &DispatchPrivateData{
		Destinatario: DestinationPrivateData{Destino: destination.canonicalID()},
		Commercial: CommercialPrivateData{
			NumeroRemito:  fmt.Sprintf("R-%08d-%02d", unitSequence, transferSequence),
			NumeroFactura: fmt.Sprintf("F-%08d-%02d", unitSequence, transferSequence),
			Cantidad:      1,
		},
	}
}

func identifiersFor(sequence int) (gtin, serial, lot, expiration string) {
	zeroBased := sequence - 1
	// #nosec G115 -- callers provide a strictly positive sequence.
	zeroBased64 := uint64(zeroBased)
	// #nosec G115 -- callers provide a strictly positive sequence.
	sequence64 := uint64(sequence)

	product := (FixedSeed + zeroBased64/100) % 1_000_000_000
	gtinData := fmt.Sprintf("0779%09d", product)
	gtin = gtinData + string(rune('0'+gs1CheckDigit(gtinData)))
	serial = fmt.Sprintf("SN%016X", sequence64)
	lot = fmt.Sprintf("L%08X", zeroBased64/1000+1)

	rng := splitMix64{state: FixedSeed ^ (sequence64 * 0xd2b74407b1ce6e93)}
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	expiration = base.AddDate(0, 0, rng.intn(1095)).Format("2006-01-02")
	return gtin, serial, lot, expiration
}

func gs1CheckDigit(data string) byte {
	sum := 0
	weight := 3
	for index := len(data) - 1; index >= 0; index-- {
		sum += int(data[index]-'0') * weight
		if weight == 3 {
			weight = 1
		} else {
			weight = 3
		}
	}
	return byte((10 - sum%10) % 10)
}

type splitMix64 struct {
	state uint64
}

func (r *splitMix64) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *splitMix64) intn(n int) int {
	// #nosec G115 -- private callers guarantee a strictly positive bound.
	return int(r.next() % uint64(n))
}

func buildManifest(units int, hash string, plan generationPlan) Manifest {
	organizations := make([]Organization, 0, len(plan.participants))
	for _, org := range plan.participants {
		organizations = append(organizations, Organization{
			MSPID: org.MSPID, CanonicalID: org.canonicalID(),
			AgentType: string(org.AgentType), ClientRole: org.ClientRole,
		})
	}
	rejections := len(plan.deniedCases) + plan.duplicateCaseCount + len(plan.blockingCases)
	return Manifest{
		Schema: ManifestSchemaID, SchemaVersion: SchemaVersion,
		Generator: GeneratorMetadata{Name: generatorName, Version: GeneratorVersion},
		Seed:      FixedSeed, Parameters: Parameters{Units: units},
		Sources: SourceMetadata{
			TransferRulesetID:                  plan.matrixRulesetID,
			TransferMatrixSchemaVersion:        plan.matrixSchemaVersion,
			StateMachineVersion:                plan.stateMachineVersion,
			OrganizationsManifestSchemaVersion: plan.manifestVersion,
		},
		Dataset: Metadata{
			File: DatasetFileName, HashFile: HashFileName, SHA256: hash,
			Units: units, HappyPathUnits: units - rejections, RejectionUnits: rejections,
			UnauthorizedTransferCases: len(plan.deniedCases),
			DuplicateIdentityCases:    plan.duplicateCaseCount,
			BlockingStateCases:        len(plan.blockingCases),
			ExplicitProhibitionCases:  plan.explicitDenials,
			DefaultDenyCases:          plan.defaultDenials,
		},
		Organizations: organizations,
	}
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, byte(10))
	return os.WriteFile(path, encoded, 0o600)
}
