package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/fabric"
)

var testDemoMSPs = map[string]string{
	"anmat":        "AnmatMSP",
	"lab":          "LabMSP",
	"drogueria":    "DrogueriaMSP",
	"distribuidor": "DistribuidorMSP",
	"farmacia":     "FarmaciaMSP",
	"centromedico": "CentroMedicoMSP",
	"financiador":  "FinanciadorMSP",
}

var testDemoIDs = map[string]string{
	"anmat":        "REG:ANMAT",
	"lab":          "GLN:7791234500017",
	"drogueria":    "GLN:7791234500024",
	"distribuidor": "GLN:7791234500031",
	"farmacia":     "GLN:7791234500048",
	"centromedico": "GLN:7791234500055",
	"financiador":  "REG:INSSJP-PAMI",
}

// fakeDemoLedger simula la red compartida por los siete peers: numera los
// bloques, aplica la matriz farmacia -> droguería y guarda el último despacho
// para responder qué datos privados tiene cada organización.
type fakeDemoLedger struct {
	mu              sync.Mutex
	block           uint64
	calls           []recordedTransaction
	undisseminated  int
	acceptForbidden bool
	failFunction    string
	dispatches      map[string]fakeDispatch
	unitStates      map[string]string
	blocks          chan fabric.Block
	events          chan fabric.ChaincodeEvent
}

type fakeDispatch struct {
	from       string
	to         string
	commercial []byte
}

func newFakeDemoLedger() *fakeDemoLedger {
	return &fakeDemoLedger{
		block:      100,
		dispatches: map[string]fakeDispatch{},
		unitStates: map[string]string{},
		blocks:     make(chan fabric.Block, 64),
		events:     make(chan fabric.ChaincodeEvent, 64),
	}
}

type fakeDemoPeer struct {
	organization string
	ledger       *fakeDemoLedger
}

func (peer *fakeDemoPeer) SubmitTransaction(
	_ context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) (fabric.Commit, error) {
	ledger := peer.ledger
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.calls = append(ledger.calls, recordedTransaction{
		organization: peer.organization,
		command:      "invoke",
		function:     function,
		arguments:    append([]string(nil), arguments...),
		transient:    transient,
	})
	if function == ledger.failFunction {
		return fabric.Commit{}, errors.New(`{"code":"INTERNAL_ERROR","message":"peer caido"}`)
	}
	var reference unitRefRequest
	_ = json.Unmarshal([]byte(arguments[0]), &reference)

	switch function {
	case "DispatchTransfer":
		var destination struct {
			Destino string `json:"destino"`
		}
		_ = json.Unmarshal(transient["destinatario"], &destination)
		if peer.organization == "farmacia" && destination.Destino == testDemoIDs["drogueria"] &&
			!ledger.acceptForbidden {
			return fabric.Commit{}, errors.New(
				`endorse: {"code":"TRANSFER_NOT_AUTHORIZED","message":"el par PHARMACY -> DRUGSTORE no esta autorizado"}`,
			)
		}
	case "ReceiveTransfer":
		if ledger.undisseminated > 0 {
			ledger.undisseminated--
			return fabric.Commit{}, errors.New(
				`{"code":"INTERNAL_ERROR","message":"datos privados pendientes","details":{"reintentable":true,"causa":"PRIVATE_DATA_NOT_DISSEMINATED"}}`,
			)
		}
	}

	ledger.block++
	transactionID := fmt.Sprintf("tx-%d", ledger.block)
	state := map[string]string{
		"RegisterUnit":     "EN_LABORATORIO",
		"DispatchTransfer": "EN_TRANSITO",
		"ReceiveTransfer":  "EN_CUSTODIA",
		"Dispense":         "DISPENSADO",
		"ReportStolen":     "ROBADO",
	}[function]
	if function == "DispatchTransfer" {
		var destination struct {
			Destino string `json:"destino"`
		}
		_ = json.Unmarshal(transient["destinatario"], &destination)
		ledger.dispatches[transactionID] = fakeDispatch{
			from:       peer.organization,
			to:         slugForID(destination.Destino),
			commercial: transient["commercial"],
		}
	}
	ledger.unitStates[reference.SerialNumber] = state
	payload := []byte(fmt.Sprintf(`{"numeroSerie":%q,"estado":%q}`, reference.SerialNumber, state))
	return fabric.Commit{Payload: payload, TransactionID: transactionID, BlockNumber: ledger.block}, nil
}

func (peer *fakeDemoPeer) Query(
	_ context.Context,
	function string,
	arguments []string,
	_ map[string][]byte,
) ([]byte, error) {
	ledger := peer.ledger
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.calls = append(ledger.calls, recordedTransaction{
		organization: peer.organization,
		command:      "query",
		function:     function,
		arguments:    append([]string(nil), arguments...),
	})
	switch function {
	case "GetUnitHistory":
		return []byte(`[{"txId":"a"},{"txId":"b"}]`), nil
	case "VerifyTrace":
		return []byte(`{"legitima":true,"motivo":""}`), nil
	}
	return nil, fmt.Errorf("unexpected query %s", function)
}

func (peer *fakeDemoPeer) PrivateWrites(
	_ context.Context,
	blockNumber uint64,
	transactionID string,
) (fabric.BlockTransaction, []fabric.PrivateWrite, error) {
	ledger := peer.ledger
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	dispatch, ok := ledger.dispatches[transactionID]
	if !ok {
		return fabric.BlockTransaction{}, nil, fmt.Errorf("transaction %s not found in block %d", transactionID, blockNumber)
	}
	collection := "transfer_" + testDemoMSPs[dispatch.from] + "_" + testDemoMSPs[dispatch.to]
	transaction := fabric.BlockTransaction{
		TransactionID: transactionID,
		PrivateWrites: []fabric.PrivateWriteHash{
			{Collection: "_implicit_org_LabMSP", ValueHash: "marker"},
			{Collection: collection, ValueHash: "hash-" + transactionID},
		},
	}
	if peer.organization != dispatch.from && peer.organization != dispatch.to && peer.organization != "anmat" {
		return transaction, []fabric.PrivateWrite{}, nil
	}
	return transaction, []fabric.PrivateWrite{
		{Collection: collection, Key: "TransferOpActive", Value: dispatch.commercial},
	}, nil
}

func (peer *fakeDemoPeer) Blocks(context.Context, *uint64) (<-chan fabric.Block, error) {
	return peer.ledger.blocks, nil
}

func (peer *fakeDemoPeer) ChaincodeEvents(context.Context, *uint64) (<-chan fabric.ChaincodeEvent, error) {
	return peer.ledger.events, nil
}

func (peer *fakeDemoPeer) Close() error {
	return nil
}

func slugForID(id string) string {
	for slug, candidate := range testDemoIDs {
		if candidate == id {
			return slug
		}
	}
	return ""
}

func newTestDemoServer(t *testing.T, ledger *fakeDemoLedger) *demoServer {
	t.Helper()
	server := newDemoServer(demoServerOptions{
		channelName:   "snt-channel",
		chaincodeName: "snt",
		gtin:          defaultDemoGTIN,
		timeout:       time.Second,
		retryInterval: time.Millisecond,
	}, func(ctx context.Context, _ time.Duration) error { return ctx.Err() }, io.Discard)
	server.now = func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	for _, slug := range config.SupportedOrganizations {
		server.addOrganization(slug, testDemoMSPs[slug], testDemoIDs[slug], &fakeDemoPeer{
			organization: slug,
			ledger:       ledger,
		})
	}
	return server
}

func waitForPreparation(t *testing.T, server *demoServer) demoScenario {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		scenario := server.snapshot().Scenario
		if scenario.Preparation != demoPreparing {
			return scenario
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("preparation did not finish")
	return demoScenario{}
}

func postStep(t *testing.T, handler http.Handler, number int) (int, demoStepResult) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/steps/%d", number), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var result demoStepResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode step %d response %q: %v", number, recorder.Body.String(), err)
	}
	return recorder.Code, result
}

func TestDemoServerRunsTheDefenseScript(t *testing.T) {
	ledger := newFakeDemoLedger()
	ledger.undisseminated = 2
	server := newTestDemoServer(t, ledger)
	server.reset(context.Background())
	scenario := waitForPreparation(t, server)
	if scenario.Preparation != demoReady {
		t.Fatalf("preparation = %s (%v)", scenario.Preparation, scenario.PreparationError)
	}
	if ledger.unitStates[scenario.TheftSerial] != "EN_CUSTODIA" {
		t.Fatalf("theft unit state = %q, want EN_CUSTODIA", ledger.unitStates[scenario.TheftSerial])
	}
	preparationCalls := len(ledger.calls)
	handler := server.handler()

	type want struct {
		transactions []string
		state        string
	}
	expectations := map[int]want{
		1: {[]string{"RegisterUnit@lab"}, "EN_LABORATORIO"},
		2: {[]string{"DispatchTransfer@lab", "ReceiveTransfer@drogueria"}, "EN_CUSTODIA"},
		3: {[]string{"DispatchTransfer@drogueria", "ReceiveTransfer@farmacia"}, "EN_CUSTODIA"},
		4: {nil, ""},
		5: {[]string{"Dispense@farmacia"}, "DISPENSADO"},
		6: {[]string{"ReportStolen@farmacia"}, "ROBADO"},
		7: {nil, ""},
	}
	results := map[int]demoStepResult{}
	for number := 1; number <= len(demoStepTitles); number++ {
		status, result := postStep(t, handler, number)
		if status != http.StatusOK || !result.OK {
			t.Fatalf("step %d: status %d, result %+v", number, status, result)
		}
		var got []string
		for _, transaction := range result.Transactions {
			got = append(got, transaction.Function+"@"+transaction.Organization)
		}
		if strings.Join(got, ",") != strings.Join(expectations[number].transactions, ",") {
			t.Fatalf("step %d transactions = %v, want %v", number, got, expectations[number].transactions)
		}
		results[number] = result
	}

	if results[4].Rejection == nil || results[4].Rejection.Code != "TRANSFER_NOT_AUTHORIZED" {
		t.Fatalf("step 4 rejection = %+v", results[4].Rejection)
	}
	if ledger.unitStates[scenario.TheftSerial] != "ROBADO" {
		t.Fatalf("step 6 reported theft on %q", scenario.TheftSerial)
	}
	if string(results[7].Trace.Verdict) != `{"legitima":true,"motivo":""}` {
		t.Fatalf("step 7 verdict = %s", results[7].Trace.Verdict)
	}
	for _, call := range ledger.calls[preparationCalls:] {
		if call.function == "VerifyTrace" || call.function == "GetUnitHistory" {
			if call.organization != "financiador" || call.arguments[1] != scenario.Serial {
				t.Fatalf("%s invoked as %s on %v", call.function, call.organization, call.arguments)
			}
		}
	}

	privacy := results[3].Privacy
	if privacy == nil || privacy.Collection != "transfer_DrogueriaMSP_FarmaciaMSP" || privacy.Hash == "" {
		t.Fatalf("step 3 privacy = %+v", privacy)
	}
	seen := map[string]string{}
	for _, entry := range privacy.Organizations {
		if entry.Sees {
			seen[entry.Organization] = entry.Remito
		}
	}
	if len(seen) != 3 || seen["drogueria"] != "R-DROG-FARM" || seen["farmacia"] == "" || seen["anmat"] == "" {
		t.Fatalf("organizations that see the commercial data = %v", seen)
	}

	final := server.snapshot().Scenario
	if final.NextStep != 8 || len(final.Privacy) != 2 || final.Trace == nil {
		t.Fatalf("final scenario = %+v", final)
	}
	if string(final.Unit) != fmt.Sprintf(`{"numeroSerie":%q,"estado":"DISPENSADO"}`, scenario.Serial) {
		t.Fatalf("final unit = %s", final.Unit)
	}
}

func TestDemoServerRejectsStepsOutOfOrderAndBeforePreparation(t *testing.T) {
	ledger := newFakeDemoLedger()
	server := newTestDemoServer(t, ledger)
	handler := server.handler()

	status, result := postStep(t, handler, 1)
	if status != http.StatusConflict || result.Error.Code != "DEMO_NOT_READY" {
		t.Fatalf("before reset: status %d, %+v", status, result.Error)
	}

	server.reset(context.Background())
	waitForPreparation(t, server)
	status, result = postStep(t, handler, 2)
	if status != http.StatusConflict || result.Error.Code != "DEMO_OUT_OF_ORDER" {
		t.Fatalf("out of order: status %d, %+v", status, result.Error)
	}
	if status, _ := postStep(t, handler, 1); status != http.StatusOK {
		t.Fatalf("step 1 status = %d", status)
	}
	if status, _ := postStep(t, handler, 1); status != http.StatusConflict {
		t.Fatalf("repeated step 1 status = %d", status)
	}
}

func TestDemoServerFailsWhenTheForbiddenTransferIsAccepted(t *testing.T) {
	ledger := newFakeDemoLedger()
	ledger.acceptForbidden = true
	server := newTestDemoServer(t, ledger)
	server.reset(context.Background())
	waitForPreparation(t, server)
	handler := server.handler()
	for number := 1; number <= 3; number++ {
		postStep(t, handler, number)
	}

	status, result := postStep(t, handler, 4)
	if status != http.StatusBadGateway || result.OK || result.Rejection != nil {
		t.Fatalf("accepted forbidden transfer: status %d, %+v", status, result)
	}
	if server.snapshot().Scenario.NextStep != 4 {
		t.Fatal("a failed step must not advance the script")
	}
}

func TestDemoServerReportsTypedErrorsAndPreparationFailure(t *testing.T) {
	ledger := newFakeDemoLedger()
	ledger.failFunction = "ReceiveTransfer"
	server := newTestDemoServer(t, ledger)
	server.reset(context.Background())
	scenario := waitForPreparation(t, server)
	if scenario.Preparation != demoFailed || scenario.PreparationError.Code != "INTERNAL_ERROR" {
		t.Fatalf("preparation = %s, %+v", scenario.Preparation, scenario.PreparationError)
	}

	ledger.mu.Lock()
	ledger.failFunction = ""
	ledger.mu.Unlock()
	server.reset(context.Background())
	waitForPreparation(t, server)
	ledger.mu.Lock()
	ledger.failFunction = "RegisterUnit"
	ledger.mu.Unlock()
	status, result := postStep(t, server.handler(), 1)
	if status != http.StatusBadGateway || result.Error == nil || result.Error.Message != "peer caido" {
		t.Fatalf("status %d, %+v", status, result.Error)
	}
}

func TestDemoServerStreamsScriptBlocksAndANMATAlerts(t *testing.T) {
	ledger := newFakeDemoLedger()
	server := newTestDemoServer(t, ledger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.observe(ctx, server.peers["anmat"])

	httpServer := httptest.NewServer(server.handler())
	defer httpServer.Close()
	response, err := http.Get(httpServer.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	events := readServerSentEvents(response.Body)
	if first := <-events; first.event != "snapshot" {
		t.Fatalf("first event = %s", first.event)
	}

	server.mu.Lock()
	server.scenario.Preparation = demoPreparing
	server.mu.Unlock()
	ledger.blocks <- fabric.Block{Number: 10, Transactions: []fabric.BlockTransaction{{Function: "RegisterUnit"}}}
	server.mu.Lock()
	server.scenario.Preparation = demoReady
	server.hiddenUntil = 11
	server.mu.Unlock()
	ledger.blocks <- fabric.Block{Number: 11, Transactions: []fabric.BlockTransaction{{Function: "ReceiveTransfer"}}}
	ledger.blocks <- fabric.Block{Number: 12, Transactions: []fabric.BlockTransaction{{
		TransactionID:  "tx-12",
		Function:       "ReceiveTransfer",
		Endorsers:      []string{"DrogueriaMSP", "LabMSP"},
		Valid:          true,
		ValidationCode: "VALID",
	}}}
	ledger.events <- fabric.ChaincodeEvent{BlockNumber: 12, EventName: "ReceiveTransfer", Payload: []byte(`{}`)}
	ledger.events <- fabric.ChaincodeEvent{
		BlockNumber:   13,
		TransactionID: "tx-13",
		EventName:     "ReportStolen",
		Payload:       []byte(`{"estado":"ROBADO"}`),
	}

	// Bloques y eventos llegan por streams distintos: el orden entre ellos no
	// está garantizado.
	received := map[string]string{}
	for len(received) < 2 {
		select {
		case event := <-events:
			if event.event == "block" || event.event == "alert" {
				if _, repeated := received[event.event]; repeated {
					t.Fatalf("unexpected second %s: %s", event.event, event.data)
				}
				received[event.event] = event.data
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("received only %v", received)
		}
	}
	var block demoBlock
	if err := json.Unmarshal([]byte(received["block"]), &block); err != nil {
		t.Fatal(err)
	}
	if block.Number != 12 || strings.Join(block.Transactions[0].Endorsers, ",") != "drogueria,lab" {
		t.Fatalf("streamed block = %+v", block)
	}
	var alert demoAlert
	if err := json.Unmarshal([]byte(received["alert"]), &alert); err != nil {
		t.Fatal(err)
	}
	if alert.EventName != "ReportStolen" || alert.BlockNumber != 13 {
		t.Fatalf("streamed alert = %+v", alert)
	}
	if snapshot := server.snapshot(); len(snapshot.Blocks) != 1 || len(snapshot.Alerts) != 1 {
		t.Fatalf("snapshot keeps %d blocks and %d alerts", len(snapshot.Blocks), len(snapshot.Alerts))
	}
}

func TestDemoServerRejectsCrossOriginWrites(t *testing.T) {
	server := newTestDemoServer(t, newFakeDemoLedger())
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/reset", nil)
	request.Header.Set("Origin", "https://example.org")
	recorder := httptest.NewRecorder()
	server.handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin reset status = %d", recorder.Code)
	}
}

func TestParseDemoServerOptionsRequiresLoopback(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "localhost:9000", "[::1]:8080"} {
		if _, _, err := parseDemoServerOptions([]string{"--addr", address}, io.Discard); err != nil {
			t.Fatalf("%s rejected: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:8080", ":8080", "192.168.0.10:8080", "demo.example:80"} {
		if _, _, err := parseDemoServerOptions([]string{"--addr", address}, io.Discard); err == nil {
			t.Fatalf("%s accepted", address)
		}
	}
}

func TestRunDemoServerConnectsEveryOrganizationAndStops(t *testing.T) {
	ledger := newFakeDemoLedger()
	var mu sync.Mutex
	connected := map[string]string{}
	deps := dependencies{
		resolveProfile: func(_ string, organization string, _ string, _ string) (config.Profile, error) {
			return config.Profile{Organization: organization, MSPID: testDemoMSPs[organization]}, nil
		},
		resolveCanonicalID: func(_ string, organization string) (string, error) {
			return testDemoIDs[organization], nil
		},
		connectDemo: func(profile config.Profile, _ string, _ string, _ time.Duration) (demoPeer, error) {
			mu.Lock()
			defer mu.Unlock()
			connected[profile.Organization] = profile.MSPID
			return &fakeDemoPeer{organization: profile.Organization, ledger: ledger}, nil
		},
		wait: waitForRetry,
	}

	ctx, cancel := context.WithCancel(context.Background())
	stderr := &lockedBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- runDemoServer(ctx, []string{"--addr", "127.0.0.1:0", "--repo-root", "."}, io.Discard, stderr, deps)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(stderr.String(), "DEMO_SERVER_READY") {
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready: %s", stderr.String())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if code := <-done; code != exitSuccess {
		t.Fatalf("exit code = %d, stderr %s", code, stderr.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(connected) != len(config.SupportedOrganizations) || connected["financiador"] != "FinanciadorMSP" {
		t.Fatalf("connected organizations = %v", connected)
	}
}

type serverSentEvent struct {
	event string
	data  string
}

func readServerSentEvents(reader io.Reader) <-chan serverSentEvent {
	events := make(chan serverSentEvent, 16)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		var current serverSentEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				current.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			case line == "" && current.event != "":
				events <- current
				current = serverSentEvent{}
			}
		}
	}()
	return events
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// reconnectingObserver entrega streams nuevos en cada suscripción y registra
// desde qué bloque se pidió cada uno.
type reconnectingObserver struct {
	fakeDemoPeer
	subscriptions chan observerSubscription
}

type observerSubscription struct {
	blockStart *uint64
	eventStart *uint64
	blocks     chan fabric.Block
	events     chan fabric.ChaincodeEvent
}

func newReconnectingObserver() *reconnectingObserver {
	return &reconnectingObserver{subscriptions: make(chan observerSubscription)}
}

// reconnectingPeer adapta reconnectingObserver a demoPeer: Blocks y
// ChaincodeEvents se piden juntos en cada reconexión.
type reconnectingPeer struct {
	*reconnectingObserver
	pending *observerSubscription
}

func (peer *reconnectingPeer) Blocks(_ context.Context, start *uint64) (<-chan fabric.Block, error) {
	peer.pending = &observerSubscription{
		blockStart: copyStart(start),
		blocks:     make(chan fabric.Block),
		events:     make(chan fabric.ChaincodeEvent),
	}
	return peer.pending.blocks, nil
}

func (peer *reconnectingPeer) ChaincodeEvents(_ context.Context, start *uint64) (<-chan fabric.ChaincodeEvent, error) {
	subscription := *peer.pending
	subscription.eventStart = copyStart(start)
	peer.subscriptions <- subscription
	return subscription.events, nil
}

func copyStart(start *uint64) *uint64 {
	if start == nil {
		return nil
	}
	value := *start
	return &value
}

func startString(start *uint64) string {
	if start == nil {
		return "next"
	}
	return fmt.Sprint(*start)
}

func TestDemoServerRecoversAnAlertCutBetweenItsBlockAndItsEvent(t *testing.T) {
	server := newTestDemoServer(t, newFakeDemoLedger())
	peer := &reconnectingPeer{reconnectingObserver: newReconnectingObserver()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.observe(ctx, peer)

	first := <-peer.subscriptions
	if first.blockStart != nil || first.eventStart != nil {
		t.Fatalf("first subscription = blocks %s, events %s", startString(first.blockStart), startString(first.eventStart))
	}
	first.events <- fabric.ChaincodeEvent{BlockNumber: 12, TransactionID: "tx-12", EventName: "ReceiveTransfer", Payload: []byte(`{}`)}
	first.blocks <- fabric.Block{Number: 12}
	first.blocks <- fabric.Block{Number: 13, Transactions: []fabric.BlockTransaction{{TransactionID: "tx-13", Function: "ReportStolen"}}}
	// El stream de eventos se corta antes de entregar el evento del bloque 13.
	close(first.events)

	second := <-peer.subscriptions
	if startString(second.blockStart) != "14" || startString(second.eventStart) != "12" {
		t.Fatalf("resubscription = blocks %s, events %s; want 14 and 12",
			startString(second.blockStart), startString(second.eventStart))
	}
	stolen := fabric.ChaincodeEvent{BlockNumber: 13, TransactionID: "tx-13", EventName: "ReportStolen", Payload: []byte(`{"estado":"ROBADO"}`)}
	second.events <- fabric.ChaincodeEvent{BlockNumber: 12, TransactionID: "tx-12", EventName: "ReceiveTransfer", Payload: []byte(`{}`)}
	second.events <- stolen
	// Un segundo corte repite el replay: la alerta no se duplica.
	close(second.blocks)
	third := <-peer.subscriptions
	if startString(third.blockStart) != "14" || startString(third.eventStart) != "13" {
		t.Fatalf("second resubscription = blocks %s, events %s; want 14 and 13",
			startString(third.blockStart), startString(third.eventStart))
	}
	third.events <- stolen
	third.blocks <- fabric.Block{Number: 14}

	snapshot := server.snapshot()
	for deadline := time.Now().Add(2 * time.Second); len(snapshot.Blocks) < 3 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		snapshot = server.snapshot()
	}
	if len(snapshot.Alerts) != 1 || snapshot.Alerts[0].TransactionID != "tx-13" {
		t.Fatalf("alerts = %+v, want ReportStolen once", snapshot.Alerts)
	}
	if len(snapshot.Blocks) != 3 {
		t.Fatalf("blocks = %+v, want 12, 13 and 14 once", snapshot.Blocks)
	}
}

func TestDemoServerResumesEventsFromTheFirstBlockWhenNoEventArrived(t *testing.T) {
	server := newTestDemoServer(t, newFakeDemoLedger())
	peer := &reconnectingPeer{reconnectingObserver: newReconnectingObserver()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.observe(ctx, peer)

	first := <-peer.subscriptions
	first.blocks <- fabric.Block{Number: 20}
	first.blocks <- fabric.Block{Number: 21}
	close(first.events)

	second := <-peer.subscriptions
	if startString(second.blockStart) != "22" || startString(second.eventStart) != "20" {
		t.Fatalf("resubscription = blocks %s, events %s; want 22 and 20",
			startString(second.blockStart), startString(second.eventStart))
	}
}

func TestDemoServerServesTheScreenWithoutExternalResources(t *testing.T) {
	server := newTestDemoServer(t, newFakeDemoLedger())
	handler := server.handler()
	for _, path := range []string{"/", "/app.js", "/styles.css"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, recorder.Code)
		}
		body := recorder.Body.String()
		// La demo tiene que funcionar sin internet: ningún recurso externo.
		for _, external := range []string{"https://", `src="http`, `href="http`, "url(http", "@import"} {
			if strings.Contains(body, external) {
				t.Fatalf("GET %s references an external resource (%s)", path, external)
			}
		}
	}
}
