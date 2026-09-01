package snt

import (
	"errors"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// TestResolveRegulatorErrorMapping cubre el conjunto COMPLETO de errores que
// resolveInvoker puede producir y fija, para cada uno, si resolveRegulator lo
// enmascara como REGULATORY_ONLY o lo propaga intacto.
//
// La distincion no es cosmetica: son dos diagnosticos opuestos. REGULATORY_ONLY
// le dice al cliente y al operador «tu identidad no alcanza para esta
// operacion», y la respuesta correcta es revisar el registro o el atributo
// snt.role. INTERNAL_ERROR dice «la plataforma o el chaincode fallaron», y la
// respuesta correcta es mirar el peer. Enmascarar el segundo con el primero
// manda a depurar permisos ante una caida de infraestructura.
//
// Lo que SI debe quedar indistinguible entre si son las fallas de
// autorizacion: un invocador no autorizado no puede deducir, del codigo de
// error, si su organizacion figura en el registro o si esta habilitada.
func TestResolveRegulatorErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		// setup deja el stub y la identidad en el estado que dispara el error.
		setup func(t *testing.T) (*mockStub, *mockIdentity)
		want  cerr.Code
	}{
		{
			// Falla de la API de identidad de la plataforma.
			name: "GetMSPID falla",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				return stub, &mockIdentity{mspID: anmatMSP, failMSPID: true}
			},
			want: cerr.InternalError,
		},
		{
			// Falla de lectura del ledger.
			name: "GetState falla",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				stub.failOn("GetState", errors.New("fallo simulado del ledger"))
				return stub, regulatorIdentity()
			},
			want: cerr.InternalError,
		},
		{
			// Falla de lectura del atributo ABAC.
			name: "lectura de atributos falla",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				identity := regulatorIdentity()
				identity.failAttributes = true
				return stub, identity
			},
			want: cerr.InternalError,
		},
		{
			// Entrada del registro corrupta: es un error del chaincode o del
			// ledger, no del invocador.
			name: "entrada del registro con JSON corrupto",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				key, err := organizationKey(stub, anmatMSP)
				requireNoError(t, err)
				requireNoError(t, stub.PutState(key, []byte("{no es json")))
				return stub, regulatorIdentity()
			},
			want: cerr.InternalError,
		},
		{
			// Desde aca, fallas de autorizacion: se enmascaran.
			name: "organizacion sin entrada en el registro",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				return stub, &mockIdentity{
					mspID:      farmaciaMSP,
					attributes: map[string]string{roleAttribute: RoleRegulatoryAdmin},
				}
			},
			want: cerr.RegulatoryOnly,
		},
		{
			name: "organizacion registrada pero inactiva",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
				record, found, err := readOrganization(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin), labMSP)
				requireNoError(t, err)
				if !found {
					t.Fatal("la organizacion de prueba deberia estar registrada")
				}
				record.Active = false
				_, err = putOrganization(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin), record)
				requireNoError(t, err)
				return stub, &mockIdentity{
					mspID:      labMSP,
					attributes: map[string]string{roleAttribute: RoleRegulatoryAdmin},
				}
			},
			want: cerr.RegulatoryOnly,
		},
		{
			name: "agentType que no es REGULATOR",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
				return stub, &mockIdentity{
					mspID:      labMSP,
					attributes: map[string]string{roleAttribute: RoleRegulatoryAdmin},
				}
			},
			want: cerr.RegulatoryOnly,
		},
		{
			name: "regulador sin snt.role=regulatory-admin",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				return stub, &mockIdentity{
					mspID:      anmatMSP,
					attributes: map[string]string{roleAttribute: RoleOperator},
				}
			},
			want: cerr.RegulatoryOnly,
		},
		{
			name: "regulador sin el atributo snt.role",
			setup: func(t *testing.T) (*mockStub, *mockIdentity) {
				stub := newMockStub()
				seedRegistry(t, stub)
				return stub, &mockIdentity{mspID: anmatMSP, attributes: map[string]string{}}
			},
			want: cerr.RegulatoryOnly,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, identity := tc.setup(t)
			_, err := resolveRegulator(testContextWithIdentity(stub, identity))
			requireCode(t, err, tc.want)
		})
	}
}

// TestRegulatoryOnlyDoesNotLeakRegistryState comprueba la propiedad que motiva
// el enmascaramiento: ante distintas situaciones registrales, un invocador no
// autorizado recibe SIEMPRE la misma respuesta, sin detalles que le permitan
// deducir si su organizacion existe o si esta habilitada.
func TestRegulatoryOnlyDoesNotLeakRegistryState(t *testing.T) {
	scenarios := []string{
		"organizacion sin entrada en el registro",
		"organizacion registrada pero inactiva",
		"agentType que no es REGULATOR",
	}

	var messages []string
	for _, name := range scenarios {
		t.Run(name, func(t *testing.T) {
			stub := newMockStub()
			seedRegistry(t, stub)

			identity := &mockIdentity{
				mspID:      labMSP,
				attributes: map[string]string{roleAttribute: RoleRegulatoryAdmin},
			}
			switch name {
			case "organizacion registrada pero inactiva":
				registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
				ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
				record, _, err := readOrganization(ctx, labMSP)
				requireNoError(t, err)
				record.Active = false
				_, err = putOrganization(ctx, record)
				requireNoError(t, err)
			case "agentType que no es REGULATOR":
				registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
			}

			_, err := resolveRegulator(testContextWithIdentity(stub, identity))
			parsed, ok := cerr.Parse(err)
			if !ok {
				t.Fatalf("el error no tiene el formato del contrato: %v", err)
			}
			if parsed.Details != nil {
				t.Errorf("REGULATORY_ONLY no debe llevar detalles del registro: %v", parsed.Details)
			}
			messages = append(messages, parsed.Message)
		})
	}

	for i := 1; i < len(messages); i++ {
		if messages[i] != messages[0] {
			t.Errorf("los mensajes distinguen entre situaciones registrales:\n  %q\n  %q",
				messages[0], messages[i])
		}
	}
}

// regulatorIdentity devuelve la identidad del regulador del manifiesto
// fundacional, con el rol que exigen las operaciones REGULATORY_ONLY.
func regulatorIdentity() *mockIdentity {
	return &mockIdentity{
		mspID:      anmatMSP,
		attributes: map[string]string{roleAttribute: RoleRegulatoryAdmin},
	}
}
