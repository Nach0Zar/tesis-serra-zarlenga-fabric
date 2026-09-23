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
	invoked   []string
	closed    bool
}

func (client *scriptedClient) Query(
	_ context.Context, _ string, _ []string, _ map[string][]byte,
) ([]byte, error) {
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
