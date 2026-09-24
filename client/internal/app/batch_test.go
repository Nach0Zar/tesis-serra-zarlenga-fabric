package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Tests de CLI-4 (#114): retiro y prohibicion por lote.

// scriptedClient responde una invocacion distinta por unidad, que es lo que un
// lote necesita y el doble compartido no puede dar: el valor del comando esta
// justamente en distinguir unidad por unidad.
type scriptedClient struct {
	queryPayload []byte
	queryErr     error
	// responses mapea numero de serie -> error de la invocacion (nil = exito).
	responses map[string]error
	// rereads mapea numero de serie -> estado que devuelve ReadUnit, para
	// simular que otra corrida movio la unidad entre la consulta y la
	// invocacion.
	rereads map[string]string
	// cancelAfter cancela el contexto tras N invocaciones, para ejercitar el
	// corte del lote a mitad de camino.
	cancelAfter int
	cancel      context.CancelFunc
	invoked     []string
	readUnits   []string
	closed      bool
}

func (client *scriptedClient) Query(
	_ context.Context, function string, arguments []string, _ map[string][]byte,
) ([]byte, error) {
	if function == "ReadUnit" {
		serial := arguments[1]
		client.readUnits = append(client.readUnits, serial)
		estado, found := client.rereads[serial]
		if !found {
			return nil, errors.New(`{"code":"UNIT_NOT_FOUND","message":"sin relectura"}`)
		}
		return []byte(`{"numeroSerie":"` + serial + `","estado":"` + estado + `"}`), nil
	}
	return client.queryPayload, client.queryErr
}

func (client *scriptedClient) Invoke(
	_ context.Context, _ string, arguments []string, _ map[string][]byte,
) ([]byte, error) {
	var request unitEventRequest
	if err := json.Unmarshal([]byte(arguments[0]), &request); err != nil {
		return nil, err
	}
	client.invoked = append(client.invoked, request.SerialNumber)
	if client.cancelAfter > 0 && len(client.invoked) >= client.cancelAfter && client.cancel != nil {
		client.cancel()
		return nil, context.DeadlineExceeded
	}
	if err, found := client.responses[request.SerialNumber]; found && err != nil {
		return nil, err
	}
	return []byte(`{"numeroSerie":"` + request.SerialNumber + `","estado":"RETIRADO_MERCADO"}`), nil
}

func (client *scriptedClient) Close() error {
	client.closed = true
	return nil
}

func unitsPayload(t *testing.T, units ...medicationUnitView) []byte {
	t.Helper()
	raw, err := json.Marshal(units)
	if err != nil {
		t.Fatalf("no se pudo serializar el fixture: %v", err)
	}
	return raw
}

func runBatchCommand(
	t *testing.T, client *scriptedClient, arguments ...string,
) (batchReport, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runBatch(
		context.Background(), batchWithdraw, arguments, &stdout, &stderr, stubDependencies(client))
	var report batchReport
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
			t.Fatalf("la salida no es un reporte JSON: %v (%s)", err, stdout.String())
		}
	}
	return report, stderr.String(), code
}

func batchArguments(extra ...string) []string {
	return append([]string{
		"--org", "anmat", "--gtin", "07791234567898", "--lot", "L-2026-01",
		"--reason", "retiro dispuesto por desvio de calidad", "--repo-root", "/tmp",
	}, extra...)
}

// TestBatchAppliesOneTransactionPerUnit fija la decision de fondo de CLI-4: el
// lote se resuelve con UNA transaccion por unidad, no con una transaccion sobre
// el lote. La razon es de endoso (ADR-007, punto 6.a) y esta explicada en
// batch.go; este test la vuelve verificable.
func TestBatchAppliesOneTransactionPerUnit(t *testing.T) {
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-1", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
			medicationUnitView{NumeroSerie: "SN-2", Lote: "L-2026-01", Estado: "EN_LABORATORIO"},
		),
		responses: map[string]error{},
	}

	report, _, code := runBatchCommand(t, client, batchArguments()...)
	if code != exitSuccess {
		t.Fatalf("codigo de salida = %d", code)
	}
	if len(client.invoked) != 2 {
		t.Fatalf("invocaciones = %v, se esperaba una por unidad", client.invoked)
	}
	if report.Confirmadas != 2 || report.Alcanzadas != 2 || report.Rechazadas != 0 {
		t.Fatalf("reporte = %+v", report)
	}
	if !client.closed {
		t.Fatal("el cliente debe cerrarse")
	}
}

// TestBatchFiltersByLot: el comando alcanza solo las unidades del lote, aunque
// QueryUnitsByGTIN devuelva todo el GTIN. Sin este filtro un retiro de lote
// retiraria el producto entero.
func TestBatchFiltersByLot(t *testing.T) {
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-1", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
			medicationUnitView{NumeroSerie: "SN-9", Lote: "L-2026-02", Estado: "EN_CUSTODIA"},
		),
		responses: map[string]error{},
	}

	report, _, code := runBatchCommand(t, client, batchArguments()...)
	if code != exitSuccess {
		t.Fatalf("codigo de salida = %d", code)
	}
	if len(client.invoked) != 1 || client.invoked[0] != "SN-1" {
		t.Fatalf("invocaciones = %v, solo SN-1 pertenece al lote", client.invoked)
	}
	if report.Alcanzadas != 1 {
		t.Fatalf("unidades alcanzadas = %d", report.Alcanzadas)
	}
}

// TestBatchReportsPartialWithdrawal es el criterio que el issue enuncia como
// "un retiro parcial debe quedar visible; no puede reportarse como exito
// global". El reporte discrimina por unidad y el codigo de salida no es cero.
func TestBatchReportsPartialWithdrawal(t *testing.T) {
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-1", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
			medicationUnitView{NumeroSerie: "SN-2", Lote: "L-2026-01", Estado: "DISPENSADO"},
		),
		responses: map[string]error{
			"SN-2": errors.New(`{"code":"INVALID_STATE_TRANSITION","message":"la maquina de estados no declara el evento RETIRAR_MERCADO desde el estado DISPENSADO"}`),
		},
	}

	report, _, code := runBatchCommand(t, client, batchArguments()...)
	if code == exitSuccess {
		t.Fatal("un lote con rechazos no puede terminar con codigo cero")
	}
	if report.Confirmadas != 1 || report.Rechazadas != 1 {
		t.Fatalf("reporte = %+v", report)
	}
	var rejected *batchUnitResult
	for i := range report.Unidades {
		if report.Unidades[i].Resultado == batchResultRejected {
			rejected = &report.Unidades[i]
		}
	}
	if rejected == nil {
		t.Fatal("el reporte debe nombrar la unidad rechazada")
	}
	if rejected.NumeroSerie != "SN-2" {
		t.Fatalf("unidad rechazada = %s", rejected.NumeroSerie)
	}
	if rejected.Error == nil || rejected.Error.Code != "INVALID_STATE_TRANSITION" {
		t.Fatalf("el rechazo debe conservar el code contractual: %+v", rejected.Error)
	}
}

// TestBatchIsIdempotentByObservedState cubre el criterio de idempotencia, y fija
// COMO se decide: por el estado observado y no atrapando
// INVALID_STATE_TRANSITION.
//
// La distincion no es cosmetica y este test la separa: una unidad ya retirada y
// una unidad DISPENSADA producen el MISMO codigo de error, y son casos
// opuestos. La primera es trabajo hecho; la segunda es un rechazo legitimo que
// ADR-001 produce porque T17-T19 no declaran DISPENSADO como origen. Atrapar el
// codigo reportaria la segunda como "ya aplicada" y escondería que esa unidad
// nunca se retiro.
func TestBatchIsIdempotentByObservedState(t *testing.T) {
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-1", Lote: "L-2026-01", Estado: "RETIRADO_MERCADO"},
			medicationUnitView{NumeroSerie: "SN-2", Lote: "L-2026-01", Estado: "DISPENSADO"},
		),
		responses: map[string]error{
			"SN-2": errors.New(`{"code":"INVALID_STATE_TRANSITION","message":"desde DISPENSADO"}`),
		},
	}

	report, _, code := runBatchCommand(t, client, batchArguments()...)
	if code == exitSuccess {
		t.Fatal("la unidad DISPENSADA sigue siendo un rechazo")
	}
	if report.YaAplicadas != 1 || report.Rechazadas != 1 || report.Confirmadas != 0 {
		t.Fatalf("reporte = %+v", report)
	}
	// La unidad ya retirada NO se reinvoca: reintentar el lote no vuelve a
	// pedir trabajo hecho.
	for _, serial := range client.invoked {
		if serial == "SN-1" {
			t.Fatal("una unidad ya retirada no debe reinvocarse")
		}
	}
}

// TestBatchRequiresItsArguments: el motivo es obligatorio con el mismo criterio
// que en el chaincode -- un evento extraordinario deja asiento permanente y la
// causa regulatoria es parte del asiento.
func TestBatchRequiresItsArguments(t *testing.T) {
	casos := map[string][]string{
		"sin org":    {"--gtin", "07791234567898", "--lot", "L", "--reason", "x"},
		"sin gtin":   {"--org", "anmat", "--lot", "L", "--reason", "x"},
		"sin lote":   {"--org", "anmat", "--gtin", "07791234567898", "--reason", "x"},
		"sin motivo": {"--org", "anmat", "--gtin", "07791234567898", "--lot", "L"},
	}
	for nombre, argumentos := range casos {
		t.Run(nombre, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runBatch(context.Background(), batchWithdraw, argumentos,
				&stdout, &stderr, stubDependencies(&scriptedClient{}))
			if code != exitUsage {
				t.Fatalf("codigo = %d, se esperaba error de uso", code)
			}
		})
	}
}

// TestBatchCommandsMapToTheirContractOperation comprueba que los dos comandos
// apunten a la operacion y al estado destino correctos. Un cruce dejaria
// `prohibit-batch` saltando unidades ya retiradas en lugar de prohibirlas, que
// ADR-001 admite expresamente en T20 desde RETIRADO_MERCADO.
func TestBatchCommandsMapToTheirContractOperation(t *testing.T) {
	for command, esperado := range map[string]struct{ function, state string }{
		batchWithdraw: {"WithdrawFromMarket", "RETIRADO_MERCADO"},
		batchProhibit: {"ProhibitProduct", "PROHIBIDO"},
	} {
		opts, _, err := parseBatchOptions(command, batchArguments(), &bytes.Buffer{})
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if opts.function != esperado.function || opts.targetState != esperado.state {
			t.Fatalf("%s -> %s/%s", command, opts.function, opts.targetState)
		}
	}
}

// TestBatchSurfacesQueryFailure: si no se pueden resolver los seriales, el
// comando falla entero en lugar de reportar un lote vacio como exito.
func TestBatchSurfacesQueryFailure(t *testing.T) {
	client := &scriptedClient{queryErr: errors.New(`{"code":"INTERNAL_ERROR","message":"sin conexion"}`)}

	_, stderr, code := runBatchCommand(t, client, batchArguments()...)
	if code != exitRuntime {
		t.Fatalf("codigo = %d", code)
	}
	if !strings.Contains(stderr, "INTERNAL_ERROR") {
		t.Fatalf("el error debe conservar el code contractual: %s", stderr)
	}
}

// TestBatchKeepsPartialReportWhenInterrupted: si el contexto vence a mitad del
// lote, el ledger YA tiene unidades retiradas y callar el reporte dejaria esa
// modificacion real sin registro visible. El reporte se emite igual, con las
// unidades no intentadas distinguidas de las rechazadas.
func TestBatchKeepsPartialReportWhenInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-1", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
			medicationUnitView{NumeroSerie: "SN-2", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
			medicationUnitView{NumeroSerie: "SN-3", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
		),
		responses:   map[string]error{},
		cancelAfter: 2,
		cancel:      cancel,
	}

	var stdout, stderr bytes.Buffer
	code := runBatch(ctx, batchWithdraw, batchArguments(), &stdout, &stderr, stubDependencies(client))
	if code == exitSuccess {
		t.Fatal("un lote interrumpido no puede terminar con codigo cero")
	}
	if stdout.Len() == 0 {
		t.Fatal("el reporte parcial debe emitirse aunque el lote aborte")
	}
	var report batchReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("la salida no es un reporte JSON: %v", err)
	}
	if !report.Interrumpido {
		t.Fatal("el reporte debe declarar que el lote se interrumpio")
	}
	if report.Confirmadas != 1 {
		t.Fatalf("la unidad confirmada antes del corte debe quedar visible: %+v", report)
	}
	// SN-2 se intento y fallo por contexto; SN-3 nunca se intento. Las dos
	// quedan como NO_INTENTADA, que es lo que un reintento necesita saber.
	if report.NoIntentadas != 2 {
		t.Fatalf("unidades no intentadas = %d, se esperaban 2: %+v", report.NoIntentadas, report)
	}
	if report.Rechazadas != 0 {
		t.Fatal("un corte por contexto no es un rechazo de la unidad")
	}
}

// TestBatchRejectsEmptySelection: con un GTIN o un lote mal tipeado no se emite
// ninguna transaccion, y devolver cero dejaria que una automatizacion registre
// como exitoso un no-op completo sobre un retiro del mercado.
func TestBatchRejectsEmptySelection(t *testing.T) {
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-9", Lote: "L-OTRO", Estado: "EN_CUSTODIA"},
		),
		responses: map[string]error{},
	}

	report, stderr, code := runBatchCommand(t, client, batchArguments()...)
	if code == exitSuccess {
		t.Fatal("una seleccion vacia no puede reportarse como exito")
	}
	if len(client.invoked) != 0 {
		t.Fatalf("no debe emitirse ninguna transaccion: %v", client.invoked)
	}
	if report.Alcanzadas != 0 {
		t.Fatalf("unidades alcanzadas = %d", report.Alcanzadas)
	}
	if !strings.Contains(stderr, "INVALID_REQUEST") {
		t.Fatalf("el error debe llevar un code ramificable: %s", stderr)
	}
}

// TestBatchResolvesConcurrentApplicationByRereading cubre la carrera: entre la
// consulta que resuelve el lote y la invocacion, otra corrida lleva la unidad
// al estado destino. Esta ejecucion recibe INVALID_STATE_TRANSITION por trabajo
// que YA esta hecho.
//
// La relectura lo resuelve sin perder la distincion que motivo el diseño: SN-1
// esta ahora RETIRADO_MERCADO y se cuenta como ya aplicada, mientras SN-2 esta
// DISPENSADO --mismo codigo de error-- y sigue siendo un rechazo legitimo,
// porque ADR-001 no declara ese origen para T17-T19.
func TestBatchResolvesConcurrentApplicationByRereading(t *testing.T) {
	invalidTransition := errors.New(`{"code":"INVALID_STATE_TRANSITION","message":"transicion no declarada"}`)
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-1", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
			medicationUnitView{NumeroSerie: "SN-2", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
		),
		responses: map[string]error{"SN-1": invalidTransition, "SN-2": invalidTransition},
		rereads:   map[string]string{"SN-1": "RETIRADO_MERCADO", "SN-2": "DISPENSADO"},
	}

	report, _, code := runBatchCommand(t, client, batchArguments()...)
	if code == exitSuccess {
		t.Fatal("SN-2 sigue siendo un rechazo y el lote no puede dar cero")
	}
	if report.YaAplicadas != 1 || report.Rechazadas != 1 {
		t.Fatalf("reporte = %+v", report)
	}
	for _, unidad := range report.Unidades {
		switch unidad.NumeroSerie {
		case "SN-1":
			if unidad.Resultado != batchResultAlready {
				t.Fatalf("SN-1 fue aplicada por otra corrida: %+v", unidad)
			}
		case "SN-2":
			if unidad.Resultado != batchResultRejected {
				t.Fatalf("SN-2 esta DISPENSADO y es un rechazo legitimo: %+v", unidad)
			}
			// El reporte muestra el estado RELEIDO, que es el que explica el
			// rechazo, y no el que traia la consulta inicial.
			if unidad.Estado != "DISPENSADO" {
				t.Fatalf("el rechazo debe mostrar el estado releido: %+v", unidad)
			}
		}
	}
	if len(client.readUnits) != 2 {
		t.Fatalf("ambas unidades deben releerse: %v", client.readUnits)
	}
}

// TestBatchKeepsRejectionWhenRereadFails: ante la duda el reporte muestra el
// problema en lugar de esconderlo como trabajo hecho.
func TestBatchKeepsRejectionWhenRereadFails(t *testing.T) {
	client := &scriptedClient{
		queryPayload: unitsPayload(t,
			medicationUnitView{NumeroSerie: "SN-1", Lote: "L-2026-01", Estado: "EN_CUSTODIA"},
		),
		responses: map[string]error{
			"SN-1": errors.New(`{"code":"INVALID_STATE_TRANSITION","message":"x"}`),
		},
		rereads: map[string]string{},
	}

	report, _, code := runBatchCommand(t, client, batchArguments()...)
	if code == exitSuccess {
		t.Fatal("sin relectura confiable el rechazo se conserva")
	}
	if report.Rechazadas != 1 || report.YaAplicadas != 0 {
		t.Fatalf("reporte = %+v", report)
	}
}
