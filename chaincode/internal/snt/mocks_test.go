package snt

import (
	"crypto/sha256"
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
	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/statebased"
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

	// privateHash modela la OTRA capa que Fabric mantiene por cada escritura
	// privada: el hash de clave y valor que queda en el estado publico del
	// canal. Es legible desde cualquier peer, sea o no miembro de la
	// coleccion, y es lo que permite distinguir "el dato existe pero todavia
	// no me llego" de "aca nunca se escribio nada".
	//
	// Separarlo de privateData es lo que hace que hidePrivateData pueda
	// simular una diseminacion pendiente de forma fiel: se va el contenido y
	// queda el hash, que es exactamente lo que ve un peer miembro que todavia
	// no recibio el bloque privado.
	privateHash map[string]map[string][]byte

	failGetState bool
}

func newMockStub() *mockStub {
	return &mockStub{
		txID:        "tx-0000000000000000",
		timestamp:   time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC),
		transient:   map[string][]byte{},
		state:       map[string][]byte{},
		privateData: map[string]map[string][]byte{},
		privateHash: map[string]map[string][]byte{},
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

// errPvtdataNotAvailable reproduce el mensaje con el que Fabric rechaza la
// lectura privada de una clave cuyo hash publico esta confirmado pero cuyo
// contenido este peer todavia no tiene. No es una invencion del mock: el query
// helper del peer compara la version del hash con la del dato privado y, si
// difieren, devuelve un ErrPvtdataNotAvailable con este texto.
const errPvtdataNotAvailable = "private data matching public hash version is not available"

// GetPrivateData reproduce la semantica REAL de Fabric, que no es la de un mapa:
//
//   - sin hash y sin contenido, la clave no existe y la lectura devuelve vacio
//     sin error;
//   - con hash confirmado en el estado publico y sin contenido en este peer, la
//     lectura FALLA. Es el caso de la diseminacion pendiente de ADR-006 punto 1.
//
// Devolver (nil, nil) en el segundo caso -- como haria un mapa vacio -- dejaria
// que el chaincode pareciera manejar la condicion transitoria cuando en la red
// real nunca llegaria a ese camino: el error de lectura lo desviaria antes. El
// mock reproduce la falla justamente para que el test no pueda pasar por esa
// via.
func (s *mockStub) GetPrivateData(collection, key string) ([]byte, error) {
	if value, ok := s.privateData[collection][key]; ok {
		return value, nil
	}
	if len(s.privateHash[collection][key]) > 0 {
		return nil, errors.New(errPvtdataNotAvailable)
	}
	return nil, nil
}

func (s *mockStub) PutPrivateData(collection, key string, value []byte) error {
	if s.privateData[collection] == nil {
		s.privateData[collection] = map[string][]byte{}
	}
	if s.privateHash[collection] == nil {
		s.privateHash[collection] = map[string][]byte{}
	}
	s.privateData[collection][key] = value
	digest := sha256.Sum256(value)
	s.privateHash[collection][key] = digest[:]
	return nil
}

// DelPrivateData borra el contenido Y su hash: la eliminacion se propaga al
// estado publico como cualquier otra escritura del read-write set, de modo que
// una operacion cerrada deja de tener hash vivo (ADR-006, punto 4). Lo que
// permanece en el ledger es el hash de la escritura ORIGINAL, en su bloque, no
// una entrada viva del estado.
func (s *mockStub) DelPrivateData(collection, key string) error {
	delete(s.privateData[collection], key)
	delete(s.privateHash[collection], key)
	return nil
}

// GetPrivateDataHash devuelve el hash que Fabric conserva en el estado publico
// del canal. No exige membresia en la coleccion.
func (s *mockStub) GetPrivateDataHash(collection, key string) ([]byte, error) {
	return s.privateHash[collection][key], nil
}

// hidePrivateData simula que el peer todavia no recibio el contenido privado de
// una clave que si esta escrita: se va el contenido y queda el hash. Devuelve el
// contenido para poder reponerlo y simular la reconciliacion posterior.
func (s *mockStub) hidePrivateData(collection, key string) []byte {
	stored := s.privateData[collection][key]
	delete(s.privateData[collection], key)
	return stored
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

// endorsingOrganizations decodifica una politica de endoso por clave y devuelve
// las organizaciones que exige. Permite afirmar sobre la politica que fijo el
// chaincode en lugar de solo sobre su presencia.
func endorsingOrganizations(t *testing.T, policy []byte) []string {
	t.Helper()
	parsed, err := statebased.NewStateEP(policy)
	if err != nil {
		t.Fatalf("la politica de endoso por clave no es decodificable: %v", err)
	}
	orgs := parsed.ListOrgs()
	sort.Strings(orgs)
	return orgs
}

// requireNoError falla el test si la operacion devolvio error.
func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("operacion fallida: %v", err)
	}
}
