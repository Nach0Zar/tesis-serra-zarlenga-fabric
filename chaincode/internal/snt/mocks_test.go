package snt

import (
	"crypto/x509"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/cid"
	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/queryresult"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Este archivo implementa los dobles de prueba que pide CC-6 (#19): un mock del
// ChaincodeStub y uno de la identidad del cliente. Ambos embeben la interfaz de
// Fabric, de modo que un metodo que un test use sin estar implementado entra en
// panico de forma evidente en lugar de devolver un cero silencioso.

// mockStub es un world state en memoria con soporte de datos privados,
// politicas de endoso por clave y consulta por clave compuesta parcial.
type mockStub struct {
	shim.ChaincodeStubInterface

	txID      string
	timestamp time.Time
	transient map[string][]byte

	state       map[string][]byte
	privateData map[string]map[string][]byte // coleccion -> clave -> valor
	validation  map[string][]byte            // clave publica -> politica de endoso serializada
	events      map[string][]byte

	failGetState bool
}

func newMockStub() *mockStub {
	return &mockStub{
		txID:        "tx-0000000000000000",
		timestamp:   time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC),
		transient:   map[string][]byte{},
		state:       map[string][]byte{},
		privateData: map[string]map[string][]byte{},
		validation:  map[string][]byte{},
		events:      map[string][]byte{},
	}
}

func (s *mockStub) GetTxID() string { return s.txID }

func (s *mockStub) GetTxTimestamp() (*timestamppb.Timestamp, error) {
	return timestamppb.New(s.timestamp), nil
}

func (s *mockStub) GetTransient() (map[string][]byte, error) { return s.transient, nil }

func (s *mockStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return shim.CreateCompositeKey(objectType, attributes)
}

func (s *mockStub) GetState(key string) ([]byte, error) {
	if s.failGetState {
		return nil, errors.New("fallo simulado del ledger")
	}
	return s.state[key], nil
}

func (s *mockStub) PutState(key string, value []byte) error {
	s.state[key] = value
	return nil
}

func (s *mockStub) DelState(key string) error {
	delete(s.state, key)
	return nil
}

func (s *mockStub) GetPrivateData(collection, key string) ([]byte, error) {
	return s.privateData[collection][key], nil
}

func (s *mockStub) PutPrivateData(collection, key string, value []byte) error {
	if s.privateData[collection] == nil {
		s.privateData[collection] = map[string][]byte{}
	}
	s.privateData[collection][key] = value
	return nil
}

func (s *mockStub) DelPrivateData(collection, key string) error {
	delete(s.privateData[collection], key)
	return nil
}

func (s *mockStub) SetStateValidationParameter(key string, ep []byte) error {
	s.validation[key] = ep
	return nil
}

func (s *mockStub) GetStateValidationParameter(key string) ([]byte, error) {
	return s.validation[key], nil
}

func (s *mockStub) SetEvent(name string, payload []byte) error {
	s.events[name] = payload
	return nil
}

func (s *mockStub) GetStateByPartialCompositeKey(objectType string, keys []string) (shim.StateQueryIteratorInterface, error) {
	prefix, err := shim.CreateCompositeKey(objectType, keys)
	if err != nil {
		return nil, err
	}
	// CreateCompositeKey cierra la clave con un separador final; para un
	// prefijo parcial hay que quitarlo.
	prefix = strings.TrimSuffix(prefix, "\x00")

	var matched []*queryresult.KV
	for key, value := range s.state {
		if strings.HasPrefix(key, prefix) {
			matched = append(matched, &queryresult.KV{Key: key, Value: value})
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Key < matched[j].Key })
	return &mockIterator{items: matched}, nil
}

type mockIterator struct {
	shim.StateQueryIteratorInterface
	items []*queryresult.KV
	next  int
}

func (it *mockIterator) HasNext() bool { return it.next < len(it.items) }

func (it *mockIterator) Next() (*queryresult.KV, error) {
	if !it.HasNext() {
		return nil, errors.New("iterador agotado")
	}
	item := it.items[it.next]
	it.next++
	return item, nil
}

func (it *mockIterator) Close() error { return nil }

// mockIdentity simula la identidad del invocador: su MSP y sus atributos ABAC.
type mockIdentity struct {
	mspID          string
	attributes     map[string]string
	failMSPID      bool
	failAttributes bool
}

func (i *mockIdentity) GetID() (string, error) { return "x509::" + i.mspID, nil }

func (i *mockIdentity) GetMSPID() (string, error) {
	if i.failMSPID {
		return "", errors.New("fallo simulado de identidad")
	}
	return i.mspID, nil
}

func (i *mockIdentity) GetAttributeValue(name string) (string, bool, error) {
	if i.failAttributes {
		return "", false, errors.New("fallo simulado al leer atributos")
	}
	value, found := i.attributes[name]
	return value, found, nil
}

func (i *mockIdentity) AssertAttributeValue(name, value string) error {
	if got, found := i.attributes[name]; !found || got != value {
		return fmt.Errorf("el atributo %s no vale %s", name, value)
	}
	return nil
}

func (i *mockIdentity) GetX509Certificate() (*x509.Certificate, error) {
	return nil, errors.New("no implementado en el mock")
}

var _ cid.ClientIdentity = (*mockIdentity)(nil)

// testContext arma un contexto de transaccion con el stub y la identidad dados.
func testContext(stub *mockStub, mspID, role string) contractapi.TransactionContextInterface {
	attributes := map[string]string{}
	if role != "" {
		attributes[roleAttribute] = role
	}
	return testContextWithIdentity(stub, &mockIdentity{mspID: mspID, attributes: attributes})
}

// testContextWithIdentity arma el contexto con una identidad ya construida, para
// los tests que necesitan inyectar una falla de la API de identidad.
func testContextWithIdentity(stub *mockStub, identity *mockIdentity) contractapi.TransactionContextInterface {
	ctx := new(contractapi.TransactionContext)
	ctx.SetStub(stub)
	ctx.SetClientIdentity(identity)
	return ctx
}

// Identidades del dataset fundacional (network/organizations-manifest.json),
// usadas por los tests para no inventar organizaciones ad hoc.
const (
	anmatMSP       = "AnmatMSP"
	labMSP         = "LabMSP"
	drogueriaMSP   = "DrogueriaMSP"
	farmaciaMSP    = "FarmaciaMSP"
	financiadorMSP = "FinanciadorMSP"

	labGLN       = "7791234500017"
	drogueriaGLN = "7791234500024"
	farmaciaGLN  = "7791234500048"

	validGTIN   = "07791234567898"
	validSerial = "SN-0001-ABCD"
)

// seedRegistry siembra la entrada REGULATOR ejecutando la Init real, de modo
// que los tests parten del mismo estado que produce el bootstrap de ADR-007.
func seedRegistry(t *testing.T, stub *mockStub) {
	t.Helper()
	contract := new(SNTContract)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	if _, err := contract.Init(ctx); err != nil {
		t.Fatalf("Init de preparacion fallo: %v", err)
	}
}

// registerOrg da de alta una organizacion custodial con la identidad del
// regulador, como hace el paso (f) del bootstrap de ADR-007.
func registerOrg(t *testing.T, stub *mockStub, mspID, gln string, agentType domain.AgentType) {
	t.Helper()
	contract := new(SNTContract)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	_, err := contract.RegisterOrganization(ctx, RegisterOrganizationRequest{
		MSPID:     mspID,
		ID:        gln,
		IDType:    IDTypeGLN,
		AgentType: agentType,
		Active:    true,
	})
	if err != nil {
		t.Fatalf("RegisterOrganization(%s) fallo: %v", mspID, err)
	}
}

// requireCode exige que el error sea un error tipificado del contrato con el
// codigo indicado. Es la asercion central de los tests de reglas de rechazo:
// el cliente y la baseline ramifican sobre `code`, no sobre `message`.
func requireCode(t *testing.T, err error, want cerr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("se esperaba el error %s y la operacion tuvo exito", want)
	}
	parsed, ok := cerr.Parse(err)
	if !ok {
		t.Fatalf("el error no tiene el formato del contrato: %v", err)
	}
	if parsed.Code != want {
		t.Fatalf("codigo de error = %s, se esperaba %s (mensaje: %v)", parsed.Code, want, err)
	}
}

// requireNoError falla el test si la operacion devolvio error.
func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("operacion fallida: %v", err)
	}
}
