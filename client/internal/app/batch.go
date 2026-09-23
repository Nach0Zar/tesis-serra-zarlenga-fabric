package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/contracterr"
)

// El retiro y la prohibicion POR LOTE se resuelven aca, en el cliente, y no en
// el chaincode. La razon es de endoso y no de comodidad: ADR-007 punto 6.a fija
// la politica de reposo de la clave de una unidad en la organizacion de su
// custodio actual, SIN rama alternativa. Un lote esta repartido entre varios
// custodios, de modo que una transaccion unica sobre N unidades exigiria el
// endoso simultaneo de TODAS sus organizaciones custodias: insatisfacible en la
// practica, y con una ventana de fallo que crece con el tamano del lote.
//
// Que el endoso basado en estado imponga granularidad por unidad es un
// RESULTADO del trabajo y no una limitacion del prototipo; queda documentado
// como tal en docs/alcance-prototipo.md.
//
// Decision de esta issue (CLI-4, #114): los seriales del lote se resuelven con
// QueryUnitsByGTIN, que ya esta en la superficie congelada, filtrando por lote
// del lado del cliente. Las dos alternativas se descartaron con motivo:
//
//   - un indice UnitByLote en el chaincode replicaria el mecanismo de CC-9,
//     pero exigiria un agregado MINOR al contrato congelado y una secuencia
//     nueva de lifecycle para una consulta que el cliente puede derivar;
//   - el bundle de domain/dataset conoce el mapeo lote -> seriales, pero solo
//     para las unidades sembradas: el comando serviria para el dataset
//     sintetico de medicion y no contra un ledger real.
//
// El costo es traer todas las unidades del GTIN y descartar las de otro lote.
// El contrato ya declara que QueryUnitsByGTIN no pagina, de modo que este
// comando hereda ese limite y no introduce uno nuevo.

const (
	batchWithdraw = "withdraw-batch"
	batchProhibit = "prohibit-batch"
)

// medicationUnitView es la proyeccion de la vista publica que este comando
// necesita: el serial para invocar, el lote para filtrar y el estado para
// decidir la idempotencia. No se replica la vista completa del contrato
// porque el cliente no persiste ni reexpone el resto de los campos.
type medicationUnitView struct {
	NumeroSerie string `json:"numeroSerie"`
	Lote        string `json:"lote"`
	Estado      string `json:"estado"`
}

type batchOptions struct {
	organization    string
	gtin            string
	lot             string
	reason          string
	repositoryRoot  string
	gatewayEndpoint string
	tlsServerName   string
	channelName     string
	chaincodeName   string
	timeout         time.Duration
	function        string
	targetState     string
}

// batchUnitResult es el resultado de UNA unidad. El reporte es por unidad
// porque la operacion es por unidad: un retiro parcial tiene que quedar
// visible, y reportar el lote como un unico exito o fracaso escondería
// exactamente lo que el operador necesita saber.
type batchUnitResult struct {
	NumeroSerie string             `json:"numeroSerie"`
	Resultado   string             `json:"resultado"`
	Estado      string             `json:"estado,omitempty"`
	Error       *contracterr.Error `json:"error,omitempty"`
}

type batchReport struct {
	Operacion   string            `json:"operacion"`
	GTIN        string            `json:"gtin"`
	Lote        string            `json:"lote"`
	Alcanzadas  int               `json:"unidadesAlcanzadas"`
	Confirmadas int               `json:"confirmadas"`
	YaAplicadas int               `json:"yaAplicadas"`
	Rechazadas  int               `json:"rechazadas"`
	Unidades    []batchUnitResult `json:"unidades"`
}

const (
	batchResultConfirmed = "CONFIRMADA"
	batchResultAlready   = "YA_APLICADA"
	batchResultRejected  = "RECHAZADA"
)

func isBatchCommand(command string) bool {
	return command == batchWithdraw || command == batchProhibit
}

func runBatch(
	ctx context.Context,
	command string,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	opts, help, err := parseBatchOptions(command, arguments, stderr)
	if help {
		return exitSuccess
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}

	repositoryRoot, err := resolveRepositoryRoot(opts.repositoryRoot, deps)
	if err != nil {
		writeRuntimeError(stderr, err, "configuration")
		return exitRuntime
	}
	profile, err := deps.resolveProfile(
		repositoryRoot, opts.organization, opts.gatewayEndpoint, opts.tlsServerName)
	if err != nil {
		writeRuntimeError(stderr, err, "configuration")
		return exitRuntime
	}
	gatewayClient, err := deps.connect(profile, opts.channelName, opts.chaincodeName, opts.timeout)
	if err != nil {
		writeRuntimeError(stderr, err, "connect")
		return exitRuntime
	}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	report, err := applyBatch(ctx, gatewayClient, opts)
	closeErr := gatewayClient.Close()
	if err != nil {
		writeRuntimeError(stderr, err, command)
		return exitRuntime
	}
	if closeErr != nil {
		writeRuntimeError(stderr, closeErr, "close")
		return exitRuntime
	}
	if err := writeJSONLine(stdout, report); err != nil {
		writeRuntimeError(stderr, err, "output")
		return exitRuntime
	}

	// Una sola unidad rechazada hace fallar el comando. Lo contrario --
	// devolver 0 porque "la mayoria" se aplico -- convertiria un retiro
	// parcial en un exito silencioso, que es justo lo que un retiro del
	// mercado no puede ser.
	if report.Rechazadas > 0 {
		return exitRuntime
	}
	return exitSuccess
}

func parseBatchOptions(
	command string,
	arguments []string,
	stderr io.Writer,
) (batchOptions, bool, error) {
	opts := batchOptions{
		channelName:   config.DefaultChannelName,
		chaincodeName: config.DefaultChaincodeName,
		timeout:       5 * time.Minute,
	}
	switch command {
	case batchWithdraw:
		opts.function = "WithdrawFromMarket"
		opts.targetState = "RETIRADO_MERCADO"
	case batchProhibit:
		opts.function = "ProhibitProduct"
		opts.targetState = "PROHIBIDO"
	default:
		return batchOptions{}, false, fmt.Errorf("unknown batch command %q", command)
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.organization, "org", "", "organización: anmat, lab, drogueria o farmacia")
	flags.StringVar(&opts.gtin, "gtin", "", "GTIN-14 del producto")
	flags.StringVar(&opts.lot, "lot", "", "lote de elaboración alcanzado")
	flags.StringVar(&opts.reason, "reason", "", "motivo regulatorio del evento")
	flags.StringVar(&opts.repositoryRoot, "repo-root", "", "raíz del repositorio")
	flags.StringVar(&opts.gatewayEndpoint, "gateway-endpoint", "", "endpoint gRPC alternativo")
	flags.StringVar(&opts.tlsServerName, "tls-server-name", "", "hostname TLS alternativo")
	flags.StringVar(&opts.channelName, "channel", opts.channelName, "canal Fabric")
	flags.StringVar(&opts.chaincodeName, "chaincode", opts.chaincodeName, "chaincode")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "timeout total del lote")
	flags.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: snt-client %s [options]\n", command)
		flags.PrintDefaults()
	}

	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return batchOptions{}, true, nil
		}
		return batchOptions{}, false, err
	}
	if flags.NArg() != 0 {
		return batchOptions{}, false, fmt.Errorf("unexpected positional arguments %q", flags.Args())
	}
	if opts.organization == "" {
		return batchOptions{}, false, errors.New("--org is required")
	}
	if strings.TrimSpace(opts.gtin) == "" {
		return batchOptions{}, false, errors.New("--gtin is required")
	}
	if strings.TrimSpace(opts.lot) == "" {
		return batchOptions{}, false, errors.New("--lot is required")
	}
	if strings.TrimSpace(opts.reason) == "" {
		return batchOptions{}, false, errors.New("--reason is required")
	}
	if opts.timeout <= 0 {
		return batchOptions{}, false, errors.New("--timeout must be greater than zero")
	}
	return opts, false, nil
}

func applyBatch(
	ctx context.Context,
	gatewayClient transactionClient,
	opts batchOptions,
) (batchReport, error) {
	units, err := unitsInLot(ctx, gatewayClient, opts.gtin, opts.lot)
	if err != nil {
		return batchReport{}, err
	}

	report := batchReport{
		Operacion:  opts.function,
		GTIN:       opts.gtin,
		Lote:       opts.lot,
		Alcanzadas: len(units),
		Unidades:   make([]batchUnitResult, 0, len(units)),
	}

	for _, unit := range units {
		// La idempotencia se decide por el ESTADO OBSERVADO y no atrapando
		// INVALID_STATE_TRANSITION. Los dos casos devuelven ese codigo y son
		// distintos: una unidad ya retirada es trabajo hecho, y una unidad
		// DISPENSADA es un rechazo legitimo que ADR-001 produce porque T17-T19
		// no declaran ese estado de origen. Confundirlos reportaria como
		// "ya aplicada" una unidad que nunca se retiro.
		if unit.Estado == opts.targetState {
			report.YaAplicadas++
			report.Unidades = append(report.Unidades, batchUnitResult{
				NumeroSerie: unit.NumeroSerie,
				Resultado:   batchResultAlready,
				Estado:      unit.Estado,
			})
			continue
		}

		request := marshalArgument(unitEventRequest{
			GTIN:         opts.gtin,
			SerialNumber: unit.NumeroSerie,
			Reason:       opts.reason,
		})
		payload, invokeErr := gatewayClient.Invoke(ctx, opts.function, []string{request}, nil)
		if invokeErr != nil {
			// Un fallo de contexto corta el lote: seguir invocando con el
			// contexto vencido produciria una lista de errores de transporte
			// que no dicen nada del estado de las unidades.
			if ctx.Err() != nil {
				return batchReport{}, invokeErr
			}
			contractError := contracterr.Normalize(invokeErr, opts.function)
			report.Rechazadas++
			report.Unidades = append(report.Unidades, batchUnitResult{
				NumeroSerie: unit.NumeroSerie,
				Resultado:   batchResultRejected,
				Estado:      unit.Estado,
				Error:       &contractError,
			})
			continue
		}

		resultado := batchUnitResult{
			NumeroSerie: unit.NumeroSerie,
			Resultado:   batchResultConfirmed,
			Estado:      opts.targetState,
		}
		var view medicationUnitView
		if err := json.Unmarshal(payload, &view); err == nil && view.Estado != "" {
			resultado.Estado = view.Estado
		}
		report.Confirmadas++
		report.Unidades = append(report.Unidades, resultado)
	}
	return report, nil
}

// unitsInLot resuelve los seriales alcanzados por el lote con QueryUnitsByGTIN,
// que es la consulta por criterio que el contrato ya expone, y filtra por lote
// del lado del cliente.
func unitsInLot(
	ctx context.Context,
	gatewayClient transactionClient,
	gtin string,
	lot string,
) ([]medicationUnitView, error) {
	payload, err := gatewayClient.Query(ctx, "QueryUnitsByGTIN", []string{gtin}, nil)
	if err != nil {
		return nil, err
	}
	var all []medicationUnitView
	if err := json.Unmarshal(payload, &all); err != nil {
		return nil, fmt.Errorf("decode QueryUnitsByGTIN response: %w", err)
	}
	units := make([]medicationUnitView, 0, len(all))
	for _, unit := range all {
		if unit.Lote == lot {
			units = append(units, unit)
		}
	}
	return units, nil
}
