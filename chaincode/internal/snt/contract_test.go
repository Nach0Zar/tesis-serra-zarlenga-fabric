package snt

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
	"github.com/hyperledger/fabric-contract-api-go/v2/metadata"
	"github.com/hyperledger/fabric-contract-api-go/v2/serializer"
)

// contractOperations es la superficie publica congelada por
// docs/api-contract.md. Es la lista completa: agregar, quitar o
// renombrar una operacion es un cambio del contrato que exige su propio PR con
// aprobacion explicita, nunca un efecto colateral de una issue de
// implementacion.
var contractOperations = []string{
	// Inicializacion (ADR-010, punto 4)
	"Init",
	// Escritura ordinaria
	"RegisterUnit",
	"DispatchTransfer",
	"ReceiveTransfer",
	"RejectTransfer",
	"Dispense",
	// Eventos extraordinarios y resolucion
	"Quarantine",
	"ReleaseQuarantine",
	"ReportExpired",
	"ReportStolen",
	"ReportLost",
	"ReportDamaged",
	"WithdrawFromMarket",
	"ProhibitProduct",
	"ReturnProduct",
	"Restock",
	"FinalDisposition",
	// Intervencion de laboratorio no custodio (ADR-007, puntos 6.e y 6.f)
	"AuthorizeLabIntervention",
	"RevokeLabIntervention",
	// Registro organizacion-establecimiento (ADR-003, ADR-010)
	"RegisterOrganization",
	"SetOrganizationActive",
	// Lectura
	"ReadUnit",
	"GetUnitHistory",
	"GetLabInterventionHistory",
	"QueryUnitsByGTIN",
	"QueryUnitsByState",
	"VerifyUnit",
	"VerifyTrace",
}

// TestContractSurfaceMatchesFrozenContract verifica que el chaincode declare
// exactamente las operaciones del contrato congelado: ni una de menos (una
// operacion que el cliente o la baseline esperan y no existe) ni una de mas
// (superficie publica no documentada).
func TestContractSurfaceMatchesFrozenContract(t *testing.T) {
	contractType := reflect.TypeOf(&SNTContract{})

	// Metodos que SNTContract hereda de contractapi.Contract y que no forman
	// parte de la superficie de negocio.
	inherited := map[string]bool{}
	base := reflect.TypeOf(&contractapi.Contract{})
	for i := 0; i < base.NumMethod(); i++ {
		inherited[base.Method(i).Name] = true
	}

	var declared []string
	for i := 0; i < contractType.NumMethod(); i++ {
		name := contractType.Method(i).Name
		if inherited[name] {
			continue
		}
		declared = append(declared, name)
	}

	expected := append([]string(nil), contractOperations...)
	sort.Strings(expected)
	sort.Strings(declared)

	if !reflect.DeepEqual(expected, declared) {
		t.Fatalf("superficie del chaincode distinta de la del contrato v%s\n  declaradas: %v\n  contrato:   %v",
			ContractVersion, declared, expected)
	}
}

// TestChaincodeBuildsWithContractAPI comprueba que contractapi acepte todas las
// firmas declaradas. Es lo que garantiza el criterio "invocacion dummy
// responde" de CC-1: si una firma no fuera admisible, NewChaincode fallaria y
// el chaincode no arrancaria en el peer.
func TestChaincodeBuildsWithContractAPI(t *testing.T) {
	contract := new(SNTContract)
	contract.Info.Version = ContractVersion

	chaincode, err := contractapi.NewChaincode(contract)
	if err != nil {
		t.Fatalf("contractapi rechazo la superficie del contrato: %v", err)
	}
	if chaincode == nil {
		t.Fatal("NewChaincode devolvio un chaincode nulo")
	}
}

// TestHistoryDeleteEntryValidatesAgainstTheGeneratedSchema protege la promesa
// que el contrato hace para las dos operaciones de historial: en una entrada de
// BORRADO, `value` es `null`.
//
// Ejerce el camino REAL y no la struct: compila el schema con CompileSchemas
// --que es lo que envuelve el retorno con los components y resuelve los
// `$ref`-- y devuelve la respuesta con el JSONSerializer del chaincode, que la
// valida contra ese schema antes de entregarla.
//
// Hoy la promesa se cumple, y por una razon que conviene tener fijada porque no
// es evidente: el schema del componente referenciado por `value` declara
// `properties`, `required` y `additionalProperties`, pero NO puede declarar
// `type: "object"` -- ObjectMetadata de esta version de la libreria no tiene
// ese campo --. En JSON Schema esas tres palabras clave solo restringen
// instancias que YA son objetos, de modo que un `null` las satisface
// vacuamente. Si una version futura emitiera el `type`, la respuesta que el
// contrato promete pasaria a ser invalida y este test es el unico que lo
// notaria.
//
// La validacion que ejerce es real y no vacua: con un MedicationUnit vacio en
// lugar del nil, el serializador rechaza la respuesta por los campos
// obligatorios que faltan.
func TestHistoryDeleteEntryValidatesAgainstTheGeneratedSchema(t *testing.T) {
	casos := map[string]any{
		"UnitHistoryEntry": []UnitHistoryEntry{
			{TxID: "tx-borrado", Timestamp: "2026-01-01T00:00:00Z", IsDelete: true, Value: nil},
		},
		"LabInterventionHistoryEntry": []LabInterventionHistoryEntry{
			{TxID: "tx-borrado", Timestamp: "2026-01-01T00:00:00Z", IsDelete: true, Value: nil},
		},
	}

	for nombre, respuesta := range casos {
		t.Run(nombre, func(t *testing.T) {
			responseType := reflect.TypeOf(respuesta)
			components := new(metadata.ComponentMetadata)
			schema, err := metadata.GetSchema(responseType, components)
			if err != nil {
				t.Fatalf("no se pudo generar el schema: %v", err)
			}

			contractMetadata := metadata.ContractChaincodeMetadata{
				Contracts: map[string]metadata.ContractMetadata{
					"snt": {
						Name: "snt",
						Transactions: []metadata.TransactionMetadata{
							{Name: nombre, Returns: metadata.ReturnMetadata{Schema: schema}},
						},
					},
				},
				Components: *components,
			}
			if err := contractMetadata.CompileSchemas(); err != nil {
				t.Fatalf("no se pudo compilar el schema: %v", err)
			}
			returns := contractMetadata.Contracts["snt"].Transactions[0].Returns

			serialized, err := new(serializer.JSONSerializer).ToString(
				reflect.ValueOf(respuesta), responseType, &returns, components)
			if err != nil {
				t.Fatalf("la respuesta que el contrato promete no se pudo devolver: %v", err)
			}
			if !strings.Contains(serialized, `"value":null`) {
				t.Fatalf("la entrada de borrado debe serializar value como null: %s", serialized)
			}
		})
	}
}

// TestHistoryValueIsOptionalInMetadata declara `value` opcional en la metadata
// legible por maquina de las dos entradas de historial.
//
// Alcance de lo que corrige, que NO es la validacion de la respuesta: el
// servidor ya aceptaba `value: null` con el campo en `required`, porque la
// clave esta presente y el schema referenciado la satisface vacuamente (ver el
// test de arriba). Lo que estaba mal es lo que la metadata le DICE a un
// consumidor: declarar obligatorio un campo que el propio contrato documenta
// como nulo lleva a que un cliente generado desde ese schema lo modele como no
// opcional y falle al recibir una entrada de borrado.
func TestHistoryValueIsOptionalInMetadata(t *testing.T) {
	for _, entry := range []any{UnitHistoryEntry{}, LabInterventionHistoryEntry{}} {
		entryType := reflect.TypeOf(entry)
		t.Run(entryType.Name(), func(t *testing.T) {
			components := new(metadata.ComponentMetadata)
			if _, err := metadata.GetSchema(entryType, components); err != nil {
				t.Fatalf("no se pudo generar el schema: %v", err)
			}
			schema, found := components.Schemas[entryType.Name()]
			if !found {
				t.Fatalf("metadata no contiene %s", entryType.Name())
			}
			for _, field := range schema.Required {
				if field == "value" {
					t.Fatalf("value no puede ser required en %s: el contrato lo declara null en un borrado",
						entryType.Name())
				}
			}
			if _, found := schema.Properties["value"]; !found {
				t.Fatalf("metadata no contiene la propiedad value de %s", entryType.Name())
			}
		})
	}
}

// TestLabInterventionConditionalFieldsAreOptionalInMetadata protege la
// diferencia entre `json:,omitempty` y `metadata:,optional`: Contract API no
// infiere la segunda a partir de la primera. Si estos campos aparecen en
// `required`, una autorizacion ACTIVA valida falla durante la serializacion de
// la respuesta porque todavia no tiene datos de consumo ni revocacion.
func TestLabInterventionConditionalFieldsAreOptionalInMetadata(t *testing.T) {
	components := new(metadata.ComponentMetadata)
	_, err := metadata.GetSchema(reflect.TypeOf(LabInterventionView{}), components)
	if err != nil {
		t.Fatalf("no se pudo generar el schema de LabInterventionView: %v", err)
	}
	schema, found := components.Schemas["LabInterventionView"]
	if !found {
		t.Fatal("metadata no contiene LabInterventionView")
	}
	required := make(map[string]bool, len(schema.Required))
	for _, field := range schema.Required {
		required[field] = true
	}
	for _, field := range []string{"consumidaEn", "revocadaEn", "motivoRevocacion"} {
		if required[field] {
			t.Fatalf("%s no puede ser required en LabInterventionView", field)
		}
		if _, found := schema.Properties[field]; !found {
			t.Fatalf("metadata no contiene la propiedad condicional %s", field)
		}
	}
}

// TestNoOperationRemainsAStub fija la invariante que reemplaza a la de CC-1
// (#14): ninguna operacion del contrato devuelve ya el error de stub.
//
// Hasta EXT-8 (#63) el test simetrico comprobaba que las operaciones declaradas
// SIN implementar nombraran a su issue duena. Con la ultima implementada, ese
// test se queda sin sujeto, y borrarlo sin reemplazo dejaria sin cubrir la
// direccion que ahora importa: que nadie reintroduzca un stub en silencio.
//
// Recorre contractOperations -- la misma lista que contrasta las firmas contra
// el contrato congelado -- e invoca cada operacion con argumentos cero sobre un
// registro vacio. NO se espera que ninguna tenga exito: la mayoria falla con
// ORG_NOT_REGISTERED o INVALID_REQUEST, y eso esta bien. Lo unico que se
// rechaza es la firma del stub: INTERNAL_ERROR con `details.issue`.
func TestNoOperationRemainsAStub(t *testing.T) {
	contractType := reflect.TypeOf(&SNTContract{})

	for _, name := range contractOperations {
		t.Run(name, func(t *testing.T) {
			method, found := contractType.MethodByName(name)
			if !found {
				t.Fatalf("el chaincode no declara %s", name)
			}

			stub := newMockStub()
			args := []reflect.Value{
				reflect.ValueOf(new(SNTContract)),
				reflect.ValueOf(testContext(stub, labMSP, RoleOperator)),
			}
			for i := len(args); i < method.Type.NumIn(); i++ {
				args = append(args, reflect.Zero(method.Type.In(i)))
			}

			results := method.Func.Call(args)
			err, _ := results[len(results)-1].Interface().(error)
			if err == nil {
				return
			}
			parsed, ok := cerr.Parse(err)
			if ok && parsed.Code == cerr.InternalError && parsed.Details["issue"] != nil {
				t.Fatalf("%s sigue siendo un stub de %v", name, parsed.Details["issue"])
			}
		})
	}
}

// TestImplementedOperationsAreNotStubs deja constancia de cuales quedan
// efectivamente implementadas por CC-1 (#14).
func TestImplementedOperationsAreNotStubs(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	contract := new(SNTContract)

	// Init ya se ejecuto en seedRegistry y devolvio la entrada sembrada.
	if _, err := contract.RegisterOrganization(ctx, RegisterOrganizationRequest{
		MSPID: labMSP, ID: labGLN, IDType: IDTypeGLN,
		AgentType: "LABORATORY", Active: true,
	}); err != nil {
		t.Fatalf("RegisterOrganization deberia estar implementada: %v", err)
	}
	if _, err := contract.SetOrganizationActive(ctx, SetOrganizationActiveRequest{
		MSPID: labMSP, Active: true,
	}); err != nil {
		t.Fatalf("SetOrganizationActive deberia estar implementada: %v", err)
	}
	// T01, implementada por CC-2 (#15).
	if _, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest()); err != nil {
		t.Fatalf("RegisterUnit deberia estar implementada: %v", err)
	}

	// T02/T03, T04 y T05, implementadas por CC-3 (#16).
	registerOrg(t, stub, drogueriaMSP, drogueriaGLN, domain.AgentDrugstore)
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	if _, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("DispatchTransfer deberia estar implementada: %v", err)
	}
	stub.transient = map[string][]byte{}
	if _, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("ReceiveTransfer deberia estar implementada: %v", err)
	}

	// T06, implementada por CC-4 (#17).
	registerOrg(t, stub, farmaciaMSP, farmaciaGLN, domain.AgentPharmacy)
	seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
	if _, err := contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("Dispense deberia estar implementada: %v", err)
	}

	// Operaciones de lectura, implementadas por CC-5 (#18).
	if _, err := contract.ReadUnit(ctx, validGTIN, validSerial); err != nil {
		t.Fatalf("ReadUnit deberia estar implementada: %v", err)
	}
	if _, err := contract.GetUnitHistory(ctx, validGTIN, validSerial); err != nil {
		t.Fatalf("GetUnitHistory deberia estar implementada: %v", err)
	}
	if _, err := contract.QueryUnitsByGTIN(ctx, validGTIN); err != nil {
		t.Fatalf("QueryUnitsByGTIN deberia estar implementada: %v", err)
	}

	// Verificacion de autenticidad del adquirente, implementada por CC-7 (#61).
	if _, err := contract.VerifyUnit(ctx, validGTIN, validSerial); err != nil {
		t.Fatalf("VerifyUnit deberia estar implementada: %v", err)
	}
}
