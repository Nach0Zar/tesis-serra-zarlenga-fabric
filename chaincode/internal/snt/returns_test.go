package snt

import (
	"encoding/json"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de EXT-4 (#30): ReturnProduct (T21-T24) conforme ADR-009.

func returnRequest() UnitEventRequest {
	return UnitEventRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "devolucion por sobrestock documentada",
	}
}

func devolucionTransientFor(receptor string) map[string]any {
	return map[string]any{transientDevolucion: devolucionTransient{Receptor: receptor}}
}

// TestReturnProductWithoutTransient cubre el criterio explicito de la issue: sin
// receptor declarado la devolucion se asienta y NO se escribe dato privado
// alguno ni se resuelve coleccion. Una devolucion sin contraparte declarada es
// un caso legitimo, no una invocacion incompleta.
func TestReturnProductWithoutTransient(t *testing.T) {
	stub, contract := verifyFixture(t)

	view, err := contract.ReturnProduct(
		testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateDevuelto {
		t.Fatalf("estado = %s, se esperaba DEVUELTO", view.Estado)
	}
	// La custodia NO se mueve: es la decision central de ADR-009, punto 1.
	if view.CustodioActual != "GLN:"+drogueriaGLN {
		t.Fatalf("custodio = %s, ADR-009 no mueve la custodia", view.CustodioActual)
	}
	for collection, entries := range stub.privateData {
		for key := range entries {
			if len(key) > 0 && key[0] == 0 && contains(key, "ReturnOp") {
				t.Fatalf("sin transient no debe escribirse ReturnOp (%s)", collection)
			}
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestReturnProductWithDeclaredReceiver comprueba la clave PROPIA: ReturnOp con
// el txIdDevolucion, no un TransferOp, y con el declarante conservando la
// custodia.
func TestReturnProductWithDeclaredReceiver(t *testing.T) {
	stub, contract := verifyFixture(t)
	stub.txID = "tx-devolucion"
	withTransient(stub, devolucionTransientFor("GLN:"+labGLN))

	view, err := contract.ReturnProduct(
		testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateDevuelto {
		t.Fatalf("estado = %s, se esperaba DEVUELTO", view.Estado)
	}

	collection := pairCollectionName(drogueriaMSP, labMSP)
	key, err := returnOpKey(stub, validGTIN, validSerial, "tx-devolucion")
	requireNoError(t, err)
	raw := stub.privateData[collection][key]
	if raw == nil {
		t.Fatal("la devolucion con receptor declarado debe escribir el registro ReturnOp en la PDC del par")
	}
	var record ReturnOperation
	requireNoError(t, json.Unmarshal(raw, &record))
	if record.ReceptorDeclarado != "GLN:"+labGLN {
		t.Fatalf("receptor = %q", record.ReceptorDeclarado)
	}
	if record.Declarante != "GLN:"+drogueriaGLN {
		t.Fatalf("declarante = %q, debe ser el custodio que declara", record.Declarante)
	}
	if record.TxIDDevolucion != "tx-devolucion" {
		t.Fatalf("txIdDevolucion = %q", record.TxIDDevolucion)
	}

	// El receptor declarado NO aparece en la vista publica: revela una relacion
	// comercial que el canal no tiene por que ver.
	public, err := json.Marshal(view)
	requireNoError(t, err)
	if contains(string(public), labGLN) {
		t.Fatalf("el receptor declarado no debe viajar en la vista publica: %s", public)
	}
}

// TestReturnProductKeepsHistoryOnSecondReturn: el registro es historico e
// inmutable. Una devolucion posterior de la misma unidad usa otro
// txIdDevolucion y crea una clave NUEVA en vez de sobreescribir la anterior.
func TestReturnProductKeepsHistoryOnSecondReturn(t *testing.T) {
	stub, contract := verifyFixture(t)
	stub.txID = "tx-devolucion-1"
	withTransient(stub, devolucionTransientFor("GLN:"+labGLN))
	_, err := contract.ReturnProduct(
		testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
	requireNoError(t, err)

	// Reingreso a stock para poder devolver otra vez no esta implementado
	// (EXT-5), asi que se comprueba la invariante sobre la clave directamente:
	// una segunda escritura con otro txID no toca la primera.
	stub.txID = "tx-devolucion-2"
	ctx := testContext(stub, drogueriaMSP, RoleOperator)
	custodio, err := lookupOrganizationByCanonicalID(ctx, "GLN:"+drogueriaGLN)
	requireNoError(t, err)
	unit, err := readUnit(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	receptor, err := lookupOrganizationByCanonicalID(ctx, "GLN:"+labGLN)
	requireNoError(t, err)
	requireNoError(t, writeReturnOperation(ctx, unit, custodio, receptor, "segunda devolucion", "2026-09-13T00:00:00Z"))

	collection := pairCollectionName(drogueriaMSP, labMSP)
	first, err := returnOpKey(stub, validGTIN, validSerial, "tx-devolucion-1")
	requireNoError(t, err)
	second, err := returnOpKey(stub, validGTIN, validSerial, "tx-devolucion-2")
	requireNoError(t, err)
	if stub.privateData[collection][first] == nil || stub.privateData[collection][second] == nil {
		t.Fatal("las dos devoluciones deben coexistir: el registro es historico e inmutable")
	}
}

// TestReturnProductReceiverValidations recorre las SEIS validaciones de ADR-009
// punto 2, cada una con el codigo que fija el contrato.
func TestReturnProductReceiverValidations(t *testing.T) {
	t.Run("1 forma no canonica", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		withTransient(stub, devolucionTransientFor(labMSP))
		_, err := contract.ReturnProduct(
			testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
		requireCode(t, err, cerr.InvalidRequest)
	})

	t.Run("2 receptor inexistente", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		withTransient(stub, devolucionTransientFor("GLN:7791234500079"))
		_, err := contract.ReturnProduct(
			testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
		requireCode(t, err, cerr.OrgNotRegistered)
	})

	t.Run("3 receptor inactivo", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.SetOrganizationActive(
			testContext(stub, anmatMSP, RoleRegulatoryAdmin),
			SetOrganizationActiveRequest{MSPID: labMSP, Active: false})
		requireNoError(t, err)

		withTransient(stub, devolucionTransientFor("GLN:"+labGLN))
		_, err = contract.ReturnProduct(
			testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
		requireCode(t, err, cerr.OrgInactive)
	})

	// La validacion 4 (agentType custodial -> INVALID_DESTINATION) es
	// DEFENSIVA y no alcanzable por el camino publico, y el test lo deja
	// asentado en lugar de simular que la ejercita: una organizacion no
	// custodial solo puede registrarse con idType=REG (ADR-010, y el contrato lo
	// valida en RegisterOrganization), y parseCanonicalID rechaza REG: en la
	// validacion 1. Las dos reglas juntas hacen que nunca llegue a la 4 un
	// receptor no custodial.
	//
	// Se conserva en el codigo igual, porque es una invariante del registro y no
	// de esta operacion: el dia que ADR-010 admitiera un agentType no custodial
	// con identificador GLN, la validacion 4 pasaria a ser el unico control.
	t.Run("4 no custodial: lo ataja la validacion 1", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		registerNonCustodial(t, stub, financiadorMSP, "REG-FIN-0001", domain.AgentFinancier)
		withTransient(stub, devolucionTransientFor("REG:REG-FIN-0001"))
		_, err := contract.ReturnProduct(
			testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
		requireCode(t, err, cerr.InvalidRequest)
	})

	t.Run("5 receptor es el propio custodio", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		withTransient(stub, devolucionTransientFor("GLN:"+drogueriaGLN))
		_, err := contract.ReturnProduct(
			testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
		requireCode(t, err, cerr.InvalidDestination)
	})

	// 6. El par «receptor -> custodio» debe estar autorizado, porque es lo que
	// garantiza que exista la coleccion del par. FARMACIA -> DRUGSTORE esta
	// prohibido por la matriz.
	t.Run("6 par no autorizado por la matriz", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		withTransient(stub, devolucionTransientFor("GLN:"+farmaciaGLN))
		_, err := contract.ReturnProduct(
			testContext(stub, drogueriaMSP, RoleOperator), returnRequest())
		requireCode(t, err, cerr.TransferNotAuthorized)
	})
}

// TestReturnProductRejectsNonCustodian: T21 habilita unicamente al custodio
// actual, de modo que ni ANMAT puede declararla desde EN_CUSTODIA.
func TestReturnProductRejectsNonCustodian(t *testing.T) {
	stub, contract := verifyFixture(t)

	_, err := contract.ReturnProduct(
		testContext(stub, farmaciaMSP, RoleOperator), returnRequest())
	requireCode(t, err, cerr.UnauthorizedCustodian)

	_, err = contract.ReturnProduct(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), returnRequest())
	requireCode(t, err, cerr.InvalidStateTransition)
}

// TestReturnProductRequiresMotivo mantiene el criterio comun a los eventos
// extraordinarios.
func TestReturnProductRequiresMotivo(t *testing.T) {
	stub, contract := verifyFixture(t)
	_, err := contract.ReturnProduct(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidRequest)
}

// registerNonCustodial da de alta una organizacion no custodial, que exige
// idType=REG (ADR-010).
func registerNonCustodial(t *testing.T, stub *mockStub, mspID, id string, agentType domain.AgentType) {
	t.Helper()
	contract := new(SNTContract)
	_, err := contract.RegisterOrganization(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		RegisterOrganizationRequest{
			MSPID: mspID, ID: id, IDType: IDTypeREG,
			AgentType: agentType, Active: true,
		})
	requireNoError(t, err)
}

// TestReturnProductFromEveryOrigin cubre los cuatro origenes que ADR-001
// declara para la devolucion, con sus actores habilitados. Los estados que
// todavia no tienen operacion propia -- RETIRADO_MERCADO y PROHIBIDO son EXT-6,
// VENCIDO se alcanza con EXT-2 -- se siembran directamente: lo que este test
// ejercita es el camino de la devolucion, no como se llego al estado de origen.
func TestReturnProductFromEveryOrigin(t *testing.T) {
	cases := []struct {
		transicion string
		origen     domain.State
		invocador  string
		rol        string
	}{
		{"T21", domain.StateEnCustodia, drogueriaMSP, RoleOperator},
		{"T22 custodio", domain.StateEnCuarentena, drogueriaMSP, RoleOperator},
		{"T22 ANMAT", domain.StateEnCuarentena, anmatMSP, RoleRegulatoryAdmin},
		{"T23 retirado", domain.StateRetiradoMercado, drogueriaMSP, RoleOperator},
		{"T23 prohibido ANMAT", domain.StateProhibido, anmatMSP, RoleRegulatoryAdmin},
		{"T24", domain.StateVencido, drogueriaMSP, RoleOperator},
	}
	for _, c := range cases {
		t.Run(c.transicion, func(t *testing.T) {
			stub, contract := verifyFixture(t)
			if c.origen != domain.StateEnCustodia {
				stub.txID = "tx-origen"
				seedUnit(t, stub, c.origen, "GLN:"+drogueriaGLN)
			}

			stub.txID = "tx-devolucion-" + c.transicion
			view, err := contract.ReturnProduct(
				testContext(stub, c.invocador, c.rol), returnRequest())
			requireNoError(t, err)
			if view.Estado != domain.StateDevuelto {
				t.Fatalf("estado = %s, se esperaba DEVUELTO", view.Estado)
			}
			// La custodia no cambia en ninguno de los origenes, ni siquiera
			// cuando la declara ANMAT (ADR-009, punto 1).
			if view.CustodioActual != "GLN:"+drogueriaGLN {
				t.Fatalf("custodio = %s, ADR-009 no mueve la custodia", view.CustodioActual)
			}
		})
	}
}

// TestReturnProductByRegulatorResolvesCustodianCollection es el test que faltaba
// y que habria detectado el defecto: ANMAT declara la devolucion con receptor
// declarado, y la PDC debe ser la del par CUSTODIO <-> receptor.
//
// Resolverla con el invocador daria transfer_AnmatMSP_<receptor>, que no existe
// porque ANMAT no es un agente custodial y ADR-006 solo genera colecciones para
// pares autorizados por la matriz. PutPrivateData fallaria sobre una coleccion
// inexistente y la unidad nunca alcanzaria DEVUELTO.
func TestReturnProductByRegulatorResolvesCustodianCollection(t *testing.T) {
	stub, contract := verifyFixture(t)
	stub.txID = "tx-cuarentena"
	seedUnit(t, stub, domain.StateEnCuarentena, "GLN:"+drogueriaGLN)

	stub.txID = "tx-devolucion-anmat"
	withTransient(stub, devolucionTransientFor("GLN:"+labGLN))
	view, err := contract.ReturnProduct(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), returnRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateDevuelto {
		t.Fatalf("estado = %s, se esperaba DEVUELTO", view.Estado)
	}

	// La coleccion correcta es la del custodio, no la del invocador.
	custodial := pairCollectionName(drogueriaMSP, labMSP)
	regulatory := pairCollectionName(anmatMSP, labMSP)
	key, err := returnOpKey(stub, validGTIN, validSerial, "tx-devolucion-anmat")
	requireNoError(t, err)
	if stub.privateData[custodial][key] == nil {
		t.Fatalf("el registro debe escribirse en la coleccion del par custodio-receptor (%s)", custodial)
	}
	if stub.privateData[regulatory][key] != nil {
		t.Fatalf("no debe escribirse en una coleccion del invocador regulatorio (%s)", regulatory)
	}

	// Y el marcador regulatorio sigue siendo obligatorio.
	requireRegulatoryMarker(t, stub, opReturnProduct)
}

// TestReturnProductValidation4RejectsNonCustodialAgentType ejercita la
// validacion 4 DE VERDAD, sobre el helper y con un registro preparado a
// proposito.
//
// Por el camino publico es inalcanzable -- una organizacion no custodial solo
// puede registrarse con idType=REG y parseCanonicalID rechaza REG: en la
// validacion 1 --, pero la validacion existe como invariante del registro y
// documentarla no es demostrarla. Se siembra directamente en el world state una
// entrada con identificador GLN y agentType no custodial, que es la unica forma
// de alcanzar esa rama.
func TestReturnProductValidation4RejectsNonCustodialAgentType(t *testing.T) {
	stub, contract := verifyFixture(t)
	_ = contract
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	// Entrada imposible por el alta publica: GLN con agentType FINANCIER.
	_, err := putOrganization(ctx, OrganizationRecord{
		MSPID: financiadorMSP, ID: "7791234500079", IDType: IDTypeGLN,
		AgentType: domain.AgentFinancier, Active: true,
	})
	requireNoError(t, err)

	custodio, err := lookupOrganizationByCanonicalID(ctx, "GLN:"+drogueriaGLN)
	requireNoError(t, err)

	_, err = validateReturnReceiver(ctx, "GLN:7791234500079", custodio)
	requireCode(t, err, cerr.InvalidDestination)
}
