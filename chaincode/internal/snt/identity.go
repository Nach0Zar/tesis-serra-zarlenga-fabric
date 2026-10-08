package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Invoker es la identidad del invocador ya resuelta contra el registro
// organizacion-establecimiento.
//
// La resolucion sigue, sin excepcion, el camino de ADR-003 extendido por
// ADR-010: cid.GetMSPID() -> entrada del registro -> agentType. El chaincode
// NUNCA compara contra literales de MSP ("AnmatMSP", "LabMSP"): el nombre de la
// MSP es un detalle de configuracion de red y el registro es la unica fuente de
// verdad de identidad (ADR-003, "Decision"; ADR-010, alternativa A descartada).
type Invoker struct {
	// MSPID es el resultado de cid.GetMSPID().
	MSPID string
	// Org es la entrada del registro que corresponde a ese mspId.
	Org OrganizationRecord
	// Role es el valor del atributo ABAC snt.role del certificado de
	// enrolamiento (DES-6). Vacio si el certificado no lo porta.
	Role string
}

// CanonicalID devuelve el identificador canonico `<idType>:<id>` del invocador,
// que es lo que se persiste como custodio (ADR-003, punto 4).
func (i Invoker) CanonicalID() string { return i.Org.CanonicalID() }

// invokerMSPID devuelve el mspId del invocador sin tocar el registro. Solo Init
// lo usa directamente: en el bootstrap todavia no existe entrada registral
// contra la cual resolver (ADR-010, punto 4).
func invokerMSPID(ctx contractapi.TransactionContextInterface) (string, error) {
	mspID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", cerr.Internal(err, "no se pudo resolver el MSP del invocador")
	}
	return mspID, nil
}

// invokerRole devuelve el atributo snt.role del certificado. DES-6 exige que la
// ausencia o un valor desconocido del atributo rechace la autorizacion cuando
// la operacion exige rol; esta funcion solo lo lee.
func invokerRole(ctx contractapi.TransactionContextInterface) (string, error) {
	value, found, err := ctx.GetClientIdentity().GetAttributeValue(roleAttribute)
	if err != nil {
		return "", cerr.Internal(err, "no se pudo leer el atributo "+roleAttribute)
	}
	if !found {
		return "", nil
	}
	return value, nil
}

// resolveInvoker resuelve la identidad del invocador contra el registro.
//
// Devuelve ORG_NOT_REGISTERED si el mspId no tiene entrada y ORG_INACTIVE si la
// entrada existe pero no esta habilitada (ADR-003, "Regla de validacion en
// chaincode", pasos 1, 2 y 6).
func resolveInvoker(ctx contractapi.TransactionContextInterface) (Invoker, error) {
	mspID, err := invokerMSPID(ctx)
	if err != nil {
		return Invoker{}, err
	}

	org, found, err := readOrganization(ctx, mspID)
	if err != nil {
		return Invoker{}, err
	}
	if !found {
		return Invoker{}, cerr.New(cerr.OrgNotRegistered,
			"la organizacion %s no tiene entrada en el registro organizacion-establecimiento", mspID).
			WithDetails(map[string]any{"mspId": mspID})
	}
	if !org.Active {
		return Invoker{}, cerr.New(cerr.OrgInactive,
			"la organizacion %s esta registrada pero no habilitada", mspID).
			WithDetails(map[string]any{"mspId": mspID})
	}

	role, err := invokerRole(ctx)
	if err != nil {
		return Invoker{}, err
	}

	return Invoker{MSPID: mspID, Org: org, Role: role}, nil
}

// requireRole exige que el invocador porte uno de los roles indicados.
// La ausencia del atributo tambien falla, conforme DES-6.
func (i Invoker) requireRole(allowed ...string) error {
	for _, role := range allowed {
		if i.Role == role {
			return nil
		}
	}
	return cerr.New(cerr.UnauthorizedRole,
		"el atributo %s del invocador (%q) no habilita esta operacion", roleAttribute, i.Role).
		WithDetails(map[string]any{"snt.role": i.Role, "rolesHabilitados": allowed})
}

// requireAgentType exige que el agentType de la organizacion del invocador
// pertenezca al conjunto indicado. snt.role no reemplaza a agentType: acotan
// planos distintos (DES-6, "Reglas de autorizacion").
func (i Invoker) requireAgentType(allowed ...domain.AgentType) error {
	for _, agentType := range allowed {
		if i.Org.AgentType == agentType {
			return nil
		}
	}
	return cerr.New(cerr.UnauthorizedAgentType,
		"el agentType %s no puede ejecutar esta operacion", i.Org.AgentType).
		WithDetails(map[string]any{"agentType": string(i.Org.AgentType)})
}

// resolveRegulator resuelve al invocador y exige que sea la organizacion
// regulatoria con snt.role=regulatory-admin, tal como el contrato define las
// operaciones REGULATORY_ONLY.
//
// La condicion se deriva del registro (agentType=REGULATOR), nunca del nombre
// de la MSP (ADR-010, punto 2).
//
// Las fallas de AUTORIZACION devuelven siempre REGULATORY_ONLY, sin distinguir
// entre ellas, para no filtrarle a un invocador no autorizado si su
// organizacion figura en el registro, si esta habilitada o cual es su
// agentType. Las fallas de INFRAESTRUCTURA se propagan intactas: ver
// maskedByRegulatoryOnly.
func resolveRegulator(ctx contractapi.TransactionContextInterface) (Invoker, error) {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		if code, ok := cerr.CodeOf(err); ok && maskedByRegulatoryOnly[code] {
			return Invoker{}, regulatoryOnly()
		}
		return Invoker{}, err
	}
	if invoker.Org.AgentType != domain.AgentRegulator || invoker.Role != RoleRegulatoryAdmin {
		return Invoker{}, regulatoryOnly()
	}
	return invoker, nil
}

// maskedByRegulatoryOnly enumera los codigos que resolveRegulator sustituye por
// REGULATORY_ONLY. Son exactamente los que revelarian, a un invocador que no
// esta autorizado a saberlo, si su organizacion figura en el registro o si esta
// habilitada.
//
// Todo lo demas se propaga sin tocar. Una falla de GetMSPID, de GetState, de
// lectura del atributo ABAC o una entrada del registro con JSON corrupto son
// INTERNAL_ERROR del catalogo del contrato (docs/api-contract.md): no son
// fallas de autorizacion, y devolverlas como REGULATORY_ONLY haria que el
// cliente y el operador leyeran una caida de la plataforma como una falta de
// permisos, que es el diagnostico opuesto.
//
// La lista es un allowlist de lo que se enmascara, no de lo que se propaga:
// un codigo nuevo en resolveInvoker sale a la luz en lugar de quedar
// silenciosamente convertido en REGULATORY_ONLY. TestResolveRegulatorErrorMapping
// cubre el conjunto completo de errores que resolveInvoker puede producir hoy.
var maskedByRegulatoryOnly = map[cerr.Code]bool{
	cerr.OrgNotRegistered: true,
	cerr.OrgInactive:      true,
}

func regulatoryOnly() error {
	return cerr.New(cerr.RegulatoryOnly,
		"la operacion exige una organizacion con agentType=REGULATOR activa y %s=%s",
		roleAttribute, RoleRegulatoryAdmin)
}
