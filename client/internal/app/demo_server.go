package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/contracterr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/fabric"
)

// Servidor local de la demo de la defensa (DEMO-2, #144). Ejecuta el guion de
// docs/defense-demo.md contra la red real, cada paso con la identidad de la
// organización que corresponde, y publica a la pantalla los bloques
// confirmados, las alertas de ANMAT y lo que cada organización ve de los datos
// privados de una transferencia.

const (
	defaultDemoServerAddress = "127.0.0.1:8080"
	demoTheftReason          = "sustraccion informada por la farmacia"
	demoSubscriberBuffer     = 64
	demoKeepAliveInterval    = 15 * time.Second
	demoObserverRetry        = time.Second

	demoPreparing = "PREPARANDO"
	demoReady     = "LISTA"
	demoFailed    = "FALLIDA"
)

var demoOrganizationLabels = map[string]string{
	"anmat":        "ANMAT",
	"lab":          "Laboratorio",
	"drogueria":    "Droguería",
	"distribuidor": "Distribuidor",
	"farmacia":     "Farmacia",
	"centromedico": "Centro médico",
	"financiador":  "PAMI",
}

// Los tres orderers Raft de ADR-007 los operan ANMAT, el laboratorio y la
// droguería.
var demoOrderers = []demoOrderer{
	{Name: "orderer.anmat", Organization: "anmat"},
	{Name: "orderer.lab", Organization: "lab"},
	{Name: "orderer.drogueria", Organization: "drogueria"},
}

var demoStepTitles = []string{
	"Registrar",
	"Enviar a droguería",
	"Enviar a farmacia",
	"Intento prohibido",
	"Dispensar",
	"Informar robo",
	"PAMI verifica",
}

// demoPeer es la conexión de una organización: envía y consulta con su
// identidad y lee de su propio peer los datos privados que le corresponden.
type demoPeer interface {
	SubmitTransaction(context.Context, string, []string, map[string][]byte) (fabric.Commit, error)
	Query(context.Context, string, []string, map[string][]byte) ([]byte, error)
	PrivateWrites(context.Context, uint64, string) (fabric.BlockTransaction, []fabric.PrivateWrite, error)
	Blocks(context.Context, *uint64) (<-chan fabric.Block, error)
	ChaincodeEvents(context.Context, *uint64) (<-chan fabric.ChaincodeEvent, error)
	Close() error
}

type demoServerOptions struct {
	address        string
	repositoryRoot string
	channelName    string
	chaincodeName  string
	gtin           string
	timeout        time.Duration
	retryInterval  time.Duration
}

type demoOrganization struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
	MSPID string `json:"mspId"`
	ID    string `json:"id"`
}

type demoOrderer struct {
	Name         string `json:"name"`
	Organization string `json:"organization"`
}

type demoNetwork struct {
	Channel       string             `json:"channel"`
	Chaincode     string             `json:"chaincode"`
	Organizations []demoOrganization `json:"organizations"`
	Orderers      []demoOrderer      `json:"orderers"`
}

type demoTransaction struct {
	Function      string `json:"function"`
	Organization  string `json:"organization"`
	TransactionID string `json:"transactionId"`
	BlockNumber   uint64 `json:"blockNumber"`
}

type demoPrivacyEntry struct {
	Organization string `json:"organization"`
	Sees         bool   `json:"sees"`
	Remito       string `json:"remito,omitempty"`
	Factura      string `json:"factura,omitempty"`
	Error        string `json:"error,omitempty"`
}

type demoPrivacy struct {
	From          string             `json:"from"`
	To            string             `json:"to"`
	BlockNumber   uint64             `json:"blockNumber"`
	TransactionID string             `json:"transactionId"`
	Collection    string             `json:"collection"`
	Hash          string             `json:"hash"`
	Organizations []demoPrivacyEntry `json:"organizations"`
}

type demoTrace struct {
	History json.RawMessage `json:"history"`
	Verdict json.RawMessage `json:"verdict"`
}

type demoStepResult struct {
	Step         int                `json:"step"`
	Title        string             `json:"title"`
	OK           bool               `json:"ok"`
	Transactions []demoTransaction  `json:"transactions"`
	Rejection    *contracterr.Error `json:"rejection,omitempty"`
	Error        *contracterr.Error `json:"error,omitempty"`
	Unit         json.RawMessage    `json:"unit,omitempty"`
	Privacy      *demoPrivacy       `json:"privacy,omitempty"`
	Trace        *demoTrace         `json:"trace,omitempty"`
}

type demoStepInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Done   bool   `json:"done"`
}

type demoScenario struct {
	GTIN             string             `json:"gtin"`
	Serial           string             `json:"serial"`
	TheftSerial      string             `json:"theftSerial"`
	Preparation      string             `json:"preparation"`
	PreparationError *contracterr.Error `json:"preparationError,omitempty"`
	NextStep         int                `json:"nextStep"`
	Steps            []demoStepInfo     `json:"steps"`
	Unit             json.RawMessage    `json:"unit,omitempty"`
	TheftUnit        json.RawMessage    `json:"theftUnit,omitempty"`
	Results          []demoStepResult   `json:"results"`
	Privacy          []demoPrivacy      `json:"privacy"`
	Trace            *demoTrace         `json:"trace,omitempty"`
}

type demoBlockTransaction struct {
	TransactionID  string   `json:"transactionId"`
	Function       string   `json:"function"`
	Endorsers      []string `json:"endorsers"`
	Valid          bool     `json:"valid"`
	ValidationCode string   `json:"validationCode"`
}

type demoBlock struct {
	Number       uint64                 `json:"number"`
	Transactions []demoBlockTransaction `json:"transactions"`
}

type demoAlert struct {
	EventName     string          `json:"eventName"`
	BlockNumber   uint64          `json:"blockNumber"`
	TransactionID string          `json:"transactionId"`
	Unit          json.RawMessage `json:"unit"`
	ReceivedAt    time.Time       `json:"receivedAt"`
}

type demoSnapshot struct {
	Network   demoNetwork  `json:"network"`
	Connected bool         `json:"connected"`
	Scenario  demoScenario `json:"scenario"`
	Blocks    []demoBlock  `json:"blocks"`
	Alerts    []demoAlert  `json:"alerts"`
}

type demoMessage struct {
	event string
	data  []byte
}

type demoServer struct {
	network      demoNetwork
	peers        map[string]demoPeer
	destinations map[string]string
	mspToSlug    map[string]string
	gtin         string
	timeout      time.Duration
	retry        time.Duration
	wait         func(context.Context, time.Duration) error
	now          func() time.Time
	assets       fs.FS
	stderr       io.Writer
	// ctx vive lo que vive el servidor; los pasos y la preparación lo usan en
	// lugar del contexto de la petición HTTP.
	ctx context.Context

	// run serializa los pasos y el reinicio: el guion es secuencial.
	run sync.Mutex

	mu          sync.Mutex
	scenario    demoScenario
	hiddenUntil uint64
	connected   bool
	blocks      []demoBlock
	alerts      []demoAlert
	subscribers map[chan demoMessage]struct{}
	// seenEvents deduplica las alertas que repite la reanudación de eventos.
	seenEvents map[string]struct{}
	background sync.WaitGroup
}

func runDemoServer(
	ctx context.Context,
	arguments []string,
	_ io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	opts, help, err := parseDemoServerOptions(arguments, stderr)
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

	server := newDemoServer(opts, deps.wait, stderr)
	defer server.close()

	for _, slug := range config.SupportedOrganizations {
		profile, err := deps.resolveProfile(repositoryRoot, slug, "", "")
		if err != nil {
			writeRuntimeError(stderr, err, "configuration")
			return exitRuntime
		}
		canonicalID, err := deps.resolveCanonicalID(repositoryRoot, slug)
		if err != nil {
			writeRuntimeError(stderr, err, "configuration")
			return exitRuntime
		}
		peer, err := deps.connectDemo(profile, opts.channelName, opts.chaincodeName, opts.timeout)
		if err != nil {
			writeRuntimeError(stderr, err, "connect")
			return exitRuntime
		}
		server.addOrganization(slug, profile.MSPID, canonicalID, peer)
	}

	listener, err := net.Listen("tcp", opts.address)
	if err != nil {
		writeRuntimeError(stderr, err, "listen")
		return exitRuntime
	}
	httpServer := &http.Server{
		Handler:           server.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server.ctx = ctx
	server.background.Add(1)
	go func() {
		defer server.background.Done()
		server.observe(ctx, server.peers["anmat"])
	}()
	server.reset(ctx)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.Serve(listener)
	}()
	_ = writeJSONLine(stderr, map[string]string{
		"type": "DEMO_SERVER_READY",
		"url":  "http://" + listener.Addr().String() + "/",
	})

	select {
	case <-ctx.Done():
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.closeSubscribers()
		_ = httpServer.Shutdown(shutdownContext)
		return exitSuccess
	case err := <-serveErr:
		writeRuntimeError(stderr, err, "serve")
		return exitRuntime
	}
}

func newDemoServer(
	opts demoServerOptions,
	wait func(context.Context, time.Duration) error,
	stderr io.Writer,
) *demoServer {
	return &demoServer{
		network: demoNetwork{
			Channel:   opts.channelName,
			Chaincode: opts.chaincodeName,
			Orderers:  demoOrderers,
		},
		peers:        map[string]demoPeer{},
		destinations: map[string]string{},
		mspToSlug:    map[string]string{},
		gtin:         opts.gtin,
		timeout:      opts.timeout,
		retry:        opts.retryInterval,
		wait:         wait,
		now:          time.Now,
		assets:       demoAssets(),
		stderr:       stderr,
		ctx:          context.Background(),
		subscribers:  map[chan demoMessage]struct{}{},
		seenEvents:   map[string]struct{}{},
	}
}

func (s *demoServer) addOrganization(slug, mspID, canonicalID string, peer demoPeer) {
	s.peers[slug] = peer
	s.destinations[slug] = canonicalID
	s.mspToSlug[mspID] = slug
	s.network.Organizations = append(s.network.Organizations, demoOrganization{
		Slug:  slug,
		Label: demoOrganizationLabels[slug],
		MSPID: mspID,
		ID:    canonicalID,
	})
}

func parseDemoServerOptions(arguments []string, stderr io.Writer) (demoServerOptions, bool, error) {
	opts := demoServerOptions{
		address:       defaultDemoServerAddress,
		channelName:   config.DefaultChannelName,
		chaincodeName: config.DefaultChaincodeName,
		gtin:          defaultDemoGTIN,
		timeout:       30 * time.Second,
		retryInterval: defaultRetryInterval,
	}
	flags := flag.NewFlagSet("demo-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.address, "addr", opts.address, "dirección local de escucha (sólo loopback)")
	flags.StringVar(&opts.repositoryRoot, "repo-root", "", "raíz del repositorio")
	flags.StringVar(&opts.channelName, "channel", opts.channelName, "canal Fabric")
	flags.StringVar(&opts.chaincodeName, "chaincode", opts.chaincodeName, "chaincode")
	flags.StringVar(&opts.gtin, "gtin", opts.gtin, "GTIN-14 de las unidades de la demo")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "timeout por operación")
	flags.DurationVar(
		&opts.retryInterval,
		"retry-interval",
		opts.retryInterval,
		"espera ante datos privados aún no diseminados",
	)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: snt-client demo-server [options]")
		flags.PrintDefaults()
	}

	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return demoServerOptions{}, true, nil
		}
		return demoServerOptions{}, false, err
	}
	if flags.NArg() != 0 {
		return demoServerOptions{}, false, fmt.Errorf("unexpected positional arguments %q", flags.Args())
	}
	if err := requireLoopback(opts.address); err != nil {
		return demoServerOptions{}, false, err
	}
	if strings.TrimSpace(opts.gtin) == "" {
		return demoServerOptions{}, false, errors.New("--gtin must not be empty")
	}
	if opts.timeout <= 0 {
		return demoServerOptions{}, false, errors.New("--timeout must be greater than zero")
	}
	if opts.retryInterval <= 0 {
		return demoServerOptions{}, false, errors.New("--retry-interval must be greater than zero")
	}
	return opts, false, nil
}

// requireLoopback impide exponer la demo fuera de la máquina: el servidor
// firma transacciones con las identidades de las siete organizaciones.
func requireLoopback(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("--addr: %w", err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--addr must be a loopback address, got %q", address)
	}
	return nil
}

func (s *demoServer) close() {
	s.background.Wait()
	for _, peer := range s.peers {
		_ = peer.Close()
	}
}

// --- HTTP -----------------------------------------------------------------

func (s *demoServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, _ *http.Request) {
		writeDemoJSON(w, http.StatusOK, s.snapshot())
	})
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/reset", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		s.reset(s.ctx)
		writeDemoJSON(w, http.StatusAccepted, s.snapshot().Scenario)
	})
	mux.HandleFunc("POST /api/steps/{number}", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil || number < 1 || number > len(demoStepTitles) {
			http.Error(w, "unknown step", http.StatusNotFound)
			return
		}
		// El paso sigue aunque el navegador se desconecte: una transacción
		// enviada a medias dejaría el guion en un estado intermedio.
		result, status := s.runStep(s.ctx, number)
		writeDemoJSON(w, status, result)
	})
	if s.assets != nil {
		mux.Handle("GET /", http.FileServerFS(s.assets))
	}
	return mux
}

// sameOrigin rechaza escrituras iniciadas desde otra página del navegador.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == r.Host
}

func writeDemoJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func (s *demoServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	messages, snapshot := s.subscribe()
	defer s.unsubscribe(messages)
	if err := writeEvent(w, "snapshot", snapshot); err != nil {
		return
	}
	flusher.Flush()

	keepAlive := time.NewTicker(demoKeepAliveInterval)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case message, ok := <-messages:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", message.event, message.data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w io.Writer, event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

// --- Estado y difusión ------------------------------------------------------

func (s *demoServer) snapshot() demoSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *demoServer) snapshotLocked() demoSnapshot {
	return demoSnapshot{
		Network:   s.network,
		Connected: s.connected,
		Scenario:  s.scenarioLocked(),
		Blocks:    append([]demoBlock{}, s.blocks...),
		Alerts:    append([]demoAlert{}, s.alerts...),
	}
}

func (s *demoServer) scenarioLocked() demoScenario {
	scenario := s.scenario
	scenario.Steps = make([]demoStepInfo, len(demoStepTitles))
	for index, title := range demoStepTitles {
		scenario.Steps[index] = demoStepInfo{
			Number: index + 1,
			Title:  title,
			Done:   index+1 < s.scenario.NextStep,
		}
	}
	scenario.Results = append([]demoStepResult{}, s.scenario.Results...)
	scenario.Privacy = append([]demoPrivacy{}, s.scenario.Privacy...)
	return scenario
}

func (s *demoServer) subscribe() (chan demoMessage, demoSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make(chan demoMessage, demoSubscriberBuffer)
	s.subscribers[messages] = struct{}{}
	return messages, s.snapshotLocked()
}

func (s *demoServer) unsubscribe(messages chan demoMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[messages]; ok {
		delete(s.subscribers, messages)
		close(messages)
	}
}

func (s *demoServer) closeSubscribers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for messages := range s.subscribers {
		delete(s.subscribers, messages)
		close(messages)
	}
}

// publishLocked envía un evento a cada pantalla conectada. Una pantalla que no
// consume se desconecta en lugar de perder eventos: EventSource se reconecta
// sola y recibe una instantánea completa.
func (s *demoServer) publishLocked(event string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	for messages := range s.subscribers {
		select {
		case messages <- demoMessage{event: event, data: data}:
		default:
			delete(s.subscribers, messages)
			close(messages)
		}
	}
}

func (s *demoServer) publishScenarioLocked() {
	s.publishLocked("scenario", s.scenarioLocked())
}

// --- Observación de la red ---------------------------------------------------

// demoCursor guarda por separado el progreso de los dos streams. Los bloques
// se reanudan en el siguiente al último recibido. Los eventos se reanudan en el
// bloque del último evento recibido, inclusive, o en el primer bloque visto si
// todavía no llegó ninguno: un corte entre un bloque y su evento no puede
// perder la alerta. El replay se deduplica por transacción en recordEvent.
type demoCursor struct {
	nextBlock *uint64
	eventFrom *uint64
}

func (cursor *demoCursor) block(number uint64) {
	following := number + 1
	cursor.nextBlock = &following
	if cursor.eventFrom == nil {
		first := number
		cursor.eventFrom = &first
	}
}

func (cursor *demoCursor) event(number uint64) {
	cursor.eventFrom = &number
}

// observe recibe con la identidad de ANMAT los bloques completos y los eventos
// del chaincode. Si un stream se corta, vuelve a suscribirse a cada uno desde
// su propio cursor.
func (s *demoServer) observe(ctx context.Context, observer demoPeer) {
	cursor := &demoCursor{}
	for ctx.Err() == nil {
		err := s.observeOnce(ctx, observer, cursor)
		s.setConnected(false)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			_ = writeJSONLine(s.stderr, contracterr.Normalize(err, "observe"))
		}
		if s.wait(ctx, demoObserverRetry) != nil {
			return
		}
	}
}

func (s *demoServer) observeOnce(ctx context.Context, observer demoPeer, cursor *demoCursor) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	blocks, err := observer.Blocks(ctx, cursor.nextBlock)
	if err != nil {
		return err
	}
	events, err := observer.ChaincodeEvents(ctx, cursor.eventFrom)
	if err != nil {
		return err
	}
	s.setConnected(true)

	for {
		select {
		case <-ctx.Done():
			return nil
		case block, ok := <-blocks:
			if !ok {
				return errors.New("block event stream closed unexpectedly")
			}
			cursor.block(block.Number)
			s.recordBlock(block)
		case event, ok := <-events:
			if !ok {
				return errors.New("chaincode event stream closed unexpectedly")
			}
			cursor.event(event.BlockNumber)
			s.recordEvent(event)
		}
	}
}

func (s *demoServer) setConnected(connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connected == connected {
		return
	}
	s.connected = connected
	s.publishLocked("connection", map[string]bool{"connected": connected})
}

// visibleLocked oculta lo que produjo la preparación del escenario: la
// pantalla muestra sólo los bloques del guion.
func (s *demoServer) visibleLocked(blockNumber uint64) bool {
	return s.scenario.Preparation != demoPreparing && blockNumber > s.hiddenUntil
}

func (s *demoServer) recordBlock(block fabric.Block) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.visibleLocked(block.Number) {
		return
	}
	summary := demoBlock{Number: block.Number, Transactions: []demoBlockTransaction{}}
	for _, transaction := range block.Transactions {
		endorsers := make([]string, 0, len(transaction.Endorsers))
		for _, mspID := range transaction.Endorsers {
			if slug, ok := s.mspToSlug[mspID]; ok {
				endorsers = append(endorsers, slug)
			} else {
				endorsers = append(endorsers, mspID)
			}
		}
		summary.Transactions = append(summary.Transactions, demoBlockTransaction{
			TransactionID:  transaction.TransactionID,
			Function:       transaction.Function,
			Endorsers:      endorsers,
			Valid:          transaction.Valid,
			ValidationCode: transaction.ValidationCode,
		})
	}
	s.blocks = append(s.blocks, summary)
	s.publishLocked("block", summary)
}

func (s *demoServer) recordEvent(event fabric.ChaincodeEvent) {
	if !isANMATAlert(event.EventName) || !json.Valid(event.Payload) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, replayed := s.seenEvents[event.TransactionID]; replayed {
		return
	}
	s.seenEvents[event.TransactionID] = struct{}{}
	if !s.visibleLocked(event.BlockNumber) {
		return
	}
	alert := demoAlert{
		EventName:     event.EventName,
		BlockNumber:   event.BlockNumber,
		TransactionID: event.TransactionID,
		Unit:          append(json.RawMessage(nil), event.Payload...),
		ReceivedAt:    s.now().UTC(),
	}
	s.alerts = append(s.alerts, alert)
	s.publishLocked("alert", alert)
}

// --- Escenario ----------------------------------------------------------------

// reset arranca un ensayo nuevo con series nuevas y prepara en segundo plano
// la unidad del paso 6, en custodia de la farmacia.
func (s *demoServer) reset(ctx context.Context) {
	s.run.Lock()
	suffix := strings.ToUpper(strconv.FormatInt(s.now().UnixMilli(), 36))
	s.mu.Lock()
	s.scenario = demoScenario{
		GTIN:        s.gtin,
		Serial:      "DEMO-" + suffix,
		TheftSerial: "ROBO-" + suffix,
		Preparation: demoPreparing,
		NextStep:    1,
		Results:     []demoStepResult{},
		Privacy:     []demoPrivacy{},
	}
	s.blocks = nil
	s.alerts = nil
	theftSerial := s.scenario.TheftSerial
	s.publishLocked("snapshot", s.snapshotLocked())
	s.mu.Unlock()

	s.background.Add(1)
	go func() {
		defer s.background.Done()
		defer s.run.Unlock()
		unit, lastBlock, err := s.prepareTheftUnit(ctx, theftSerial)
		s.mu.Lock()
		defer s.mu.Unlock()
		if lastBlock > s.hiddenUntil {
			s.hiddenUntil = lastBlock
		}
		if err != nil {
			normalized := contracterr.Normalize(err, "prepare")
			s.scenario.Preparation = demoFailed
			s.scenario.PreparationError = &normalized
		} else {
			s.scenario.Preparation = demoReady
			s.scenario.TheftUnit = unit
		}
		s.publishScenarioLocked()
	}()
}

func (s *demoServer) prepareTheftUnit(ctx context.Context, serial string) (json.RawMessage, uint64, error) {
	var lastBlock uint64
	var unit []byte
	operations := []func() (demoTransaction, []byte, error){
		func() (demoTransaction, []byte, error) { return s.register(ctx, serial) },
		func() (demoTransaction, []byte, error) {
			return s.dispatch(ctx, "lab", "drogueria", serial, "PREP-1")
		},
		func() (demoTransaction, []byte, error) { return s.receive(ctx, "drogueria", serial) },
		func() (demoTransaction, []byte, error) {
			return s.dispatch(ctx, "drogueria", "farmacia", serial, "PREP-2")
		},
		func() (demoTransaction, []byte, error) { return s.receive(ctx, "farmacia", serial) },
	}
	for _, operation := range operations {
		transaction, payload, err := operation()
		if transaction.BlockNumber > lastBlock {
			lastBlock = transaction.BlockNumber
		}
		if err != nil {
			return nil, lastBlock, err
		}
		unit = payload
	}
	return json.RawMessage(unit), lastBlock, nil
}

func (s *demoServer) runStep(ctx context.Context, number int) (demoStepResult, int) {
	result := demoStepResult{
		Step:         number,
		Title:        demoStepTitles[number-1],
		Transactions: []demoTransaction{},
	}
	if !s.run.TryLock() {
		result.Error = &contracterr.Error{Code: "DEMO_BUSY", Message: "hay otra operación en curso"}
		return result, http.StatusConflict
	}
	defer s.run.Unlock()

	s.mu.Lock()
	scenario := s.scenario
	s.mu.Unlock()
	if scenario.Preparation != demoReady {
		result.Error = &contracterr.Error{
			Code:    "DEMO_NOT_READY",
			Message: "el escenario no está listo: " + scenario.Preparation,
		}
		return result, http.StatusConflict
	}
	if number != scenario.NextStep {
		result.Error = &contracterr.Error{
			Code:    "DEMO_OUT_OF_ORDER",
			Message: fmt.Sprintf("el paso siguiente es el %d", scenario.NextStep),
		}
		return result, http.StatusConflict
	}

	err := s.executeStep(ctx, scenario, &result)
	if err != nil {
		normalized := contracterr.Normalize(err, "step")
		result.Error = &normalized
	} else {
		result.OK = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.scenario.Results = append(s.scenario.Results, result)
	if result.OK {
		s.scenario.NextStep = number + 1
		if result.Unit != nil {
			if number == 6 {
				s.scenario.TheftUnit = result.Unit
			} else {
				s.scenario.Unit = result.Unit
			}
		}
		if result.Privacy != nil {
			s.scenario.Privacy = append(s.scenario.Privacy, *result.Privacy)
		}
		if result.Trace != nil {
			s.scenario.Trace = result.Trace
		}
	}
	s.publishScenarioLocked()
	if result.OK {
		return result, http.StatusOK
	}
	return result, http.StatusBadGateway
}

func (s *demoServer) executeStep(ctx context.Context, scenario demoScenario, result *demoStepResult) error {
	serial := scenario.Serial
	record := func(transaction demoTransaction, payload []byte, err error) error {
		if transaction.TransactionID != "" {
			result.Transactions = append(result.Transactions, transaction)
		}
		if err == nil && json.Valid(payload) {
			result.Unit = append(json.RawMessage(nil), payload...)
		}
		return err
	}
	transfer := func(from, to, suffix string) error {
		dispatched, payload, err := s.dispatch(ctx, from, to, serial, suffix)
		if err := record(dispatched, payload, err); err != nil {
			return err
		}
		if err := record(s.receive(ctx, to, serial)); err != nil {
			return err
		}
		privacy := s.privacy(ctx, from, to, dispatched)
		result.Privacy = &privacy
		return nil
	}

	switch result.Step {
	case 1:
		return record(s.register(ctx, serial))
	case 2:
		return transfer("lab", "drogueria", "LAB-DROG")
	case 3:
		return transfer("drogueria", "farmacia", "DROG-FARM")
	case 4:
		_, _, err := s.dispatch(ctx, "farmacia", "drogueria", serial, "FARM-DROG")
		if err == nil {
			return errors.New("la red aceptó una transferencia farmacia -> droguería que la matriz prohíbe")
		}
		rejection := contracterr.Normalize(err, "invoke")
		if rejection.Code != "TRANSFER_NOT_AUTHORIZED" {
			return err
		}
		result.Rejection = &rejection
		return nil
	case 5:
		return record(s.submit(ctx, "farmacia", "Dispense", marshalArgument(unitRefRequest{
			GTIN:         s.gtin,
			SerialNumber: serial,
		}), nil, false))
	case 6:
		return record(s.submit(ctx, "farmacia", "ReportStolen", marshalArgument(unitEventRequest{
			GTIN:         s.gtin,
			SerialNumber: scenario.TheftSerial,
			Reason:       demoTheftReason,
		}), nil, false))
	case 7:
		history, err := s.query(ctx, "financiador", "GetUnitHistory", s.gtin, serial)
		if err != nil {
			return err
		}
		verdict, err := s.query(ctx, "financiador", "VerifyTrace", s.gtin, serial)
		if err != nil {
			return err
		}
		result.Trace = &demoTrace{History: history, Verdict: verdict}
		return nil
	default:
		return fmt.Errorf("unknown step %d", result.Step)
	}
}

// --- Operaciones del contrato ------------------------------------------------

func (s *demoServer) register(ctx context.Context, serial string) (demoTransaction, []byte, error) {
	return s.submit(ctx, "lab", "RegisterUnit", marshalArgument(registerUnitRequest{
		GTIN:           s.gtin,
		SerialNumber:   serial,
		Lot:            defaultDemoLot,
		ExpirationDate: defaultDemoExpiration,
	}), nil, false)
}

func (s *demoServer) dispatch(
	ctx context.Context,
	from string,
	to string,
	serial string,
	suffix string,
) (demoTransaction, []byte, error) {
	transient := map[string][]byte{
		"destinatario": []byte(marshalArgument(map[string]string{"destino": s.destinations[to]})),
		"commercial": []byte(marshalArgument(map[string]any{
			"numeroRemito":  "R-" + suffix,
			"numeroFactura": "F-" + suffix,
			"cantidad":      1,
		})),
	}
	return s.submit(ctx, from, "DispatchTransfer", marshalArgument(unitRefRequest{
		GTIN:         s.gtin,
		SerialNumber: serial,
	}), transient, false)
}

func (s *demoServer) receive(ctx context.Context, organization, serial string) (demoTransaction, []byte, error) {
	return s.submit(ctx, organization, "ReceiveTransfer", marshalArgument(unitRefRequest{
		GTIN:         s.gtin,
		SerialNumber: serial,
	}), nil, true)
}

// submit envía con la identidad de la organización y espera el commit. La
// recepción reintenta mientras los datos privados del despacho no llegaron al
// peer del receptor, como receive-transfer.
func (s *demoServer) submit(
	ctx context.Context,
	organization string,
	function string,
	argument string,
	transient map[string][]byte,
	retryPrivateData bool,
) (demoTransaction, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	peer := s.peers[organization]
	for {
		commit, err := peer.SubmitTransaction(ctx, function, []string{argument}, transient)
		if err == nil {
			return demoTransaction{
				Function:      function,
				Organization:  organization,
				TransactionID: commit.TransactionID,
				BlockNumber:   commit.BlockNumber,
			}, commit.Payload, nil
		}
		if !retryPrivateData || !isPrivateDataNotDisseminated(err) {
			return demoTransaction{}, nil, err
		}
		if waitErr := s.wait(ctx, s.retry); waitErr != nil {
			return demoTransaction{}, nil, err
		}
	}
}

func (s *demoServer) query(ctx context.Context, organization, function string, arguments ...string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	payload, err := s.peers[organization].Query(ctx, function, arguments, nil)
	if err != nil {
		return nil, err
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("%s returned a non-JSON payload", function)
	}
	return json.RawMessage(payload), nil
}

// privacy pregunta al peer de cada organización, con su propia identidad, qué
// datos privados tiene del despacho. El hash sale del bloque público.
func (s *demoServer) privacy(ctx context.Context, from, to string, dispatched demoTransaction) demoPrivacy {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result := demoPrivacy{
		From:          from,
		To:            to,
		BlockNumber:   dispatched.BlockNumber,
		TransactionID: dispatched.TransactionID,
		Organizations: make([]demoPrivacyEntry, len(s.network.Organizations)),
	}
	var mu sync.Mutex
	var group sync.WaitGroup
	for index, organization := range s.network.Organizations {
		group.Add(1)
		go func() {
			defer group.Done()
			entry := demoPrivacyEntry{Organization: organization.Slug}
			transaction, writes, err := s.peers[organization.Slug].PrivateWrites(
				ctx,
				dispatched.BlockNumber,
				dispatched.TransactionID,
			)
			if err != nil {
				entry.Error = contracterr.Normalize(err, "private-data").Message
				result.Organizations[index] = entry
				return
			}
			collection, hash := transferHash(transaction)
			mu.Lock()
			if result.Hash == "" {
				result.Collection, result.Hash = collection, hash
			}
			mu.Unlock()
			for _, write := range writes {
				if write.Collection != collection {
					continue
				}
				var commercial struct {
					Remito  string `json:"numeroRemito"`
					Factura string `json:"numeroFactura"`
				}
				if json.Unmarshal(write.Value, &commercial) == nil && commercial.Remito != "" {
					entry.Sees = true
					entry.Remito = commercial.Remito
					entry.Factura = commercial.Factura
				}
			}
			result.Organizations[index] = entry
		}()
	}
	group.Wait()
	return result
}

// transferHash devuelve la colección de la transferencia y el hash del valor
// escrito, que es lo único que el bloque muestra a quien no es miembro.
func transferHash(transaction fabric.BlockTransaction) (string, string) {
	for _, write := range transaction.PrivateWrites {
		if strings.HasPrefix(write.Collection, "transfer_") && !write.IsDelete {
			return write.Collection, write.ValueHash
		}
	}
	return "", ""
}
