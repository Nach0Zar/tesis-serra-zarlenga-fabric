package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const opDispense = "Dispense"

// Dispense implementa T06 de ADR-001: la entrega de la unidad al paciente
// dentro del alcance del SNT. Estado resultante: DISPENSADO, terminal.
//
// Autorizacion (DES-6): el invocador debe ser el custodio actual, con
// agentType PHARMACY o HEALTHCARE_FACILITY, activo y con snt.role=operator,
// resuelto por el registro y no contra literales de MSP.
//
// NO recibe ni persiste dato alguno del paciente. El request es una referencia
// a la unidad y nada mas: ni nombre, ni documento, ni obra social, ni numero de
// afiliado, ni diagnostico (Ley 25.326; modelo-datos.md §4; ADR-005). El
// vinculo afiliado <-> unidad vive deliberadamente fuera del ledger, y el
// financiador parte de el off-ledger para verificar la traza del serial.
//
// El fin de envase hospitalario reutiliza esta operacion, como simplificacion
// consciente registrada en docs/alcance-prototipo.md: el prototipo no distingue
// semanticamente la dispensacion ambulatoria del fin de envase.
//
// Endoso (ADR-007, punto 6.a): peer de la organizacion dispensadora, que es el
// custodio actual y la unica rama de la politica de reposo de la clave. La
// operacion NO modifica esa politica: DISPENSADO es terminal y ADR-001 no
// admite transiciones de salida, de modo que la clave queda en el valor de
// reposo del ultimo custodio registrado y no se endurece (ADR-007, punto 6.h);
// es la logica del chaincode la que rechaza todo intento posterior con
// INVALID_STATE_TRANSITION.
func (c *SNTContract) Dispense(
	ctx contractapi.TransactionContextInterface,
	req UnitRefRequest,
) (*MedicationUnitView, error) {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		return nil, err
	}
	if err := invoker.requireAgentType(domain.AgentPharmacy, domain.AgentHealthcare); err != nil {
		return nil, err
	}
	if err := invoker.requireRole(RoleOperator); err != nil {
		return nil, err
	}
	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}

	unit, err := readUnit(ctx, req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, err
	}
	if unit.CustodioActual != invoker.CanonicalID() {
		return nil, cerr.New(cerr.UnauthorizedCustodian,
			"el invocador no es el custodio actual de la unidad").
			WithDetails(map[string]any{"custodioActual": unit.CustodioActual})
	}

	// La aptitud del estado la decide ADR-001. Solo EN_CUSTODIA admite T06: una
	// unidad vencida, en cuarentena, retirada, prohibida, robada, extraviada,
	// deteriorada o devuelta no es dispensable, y tampoco lo es una que todavia
	// esta bajo custodia del laboratorio o en transito.
	transition, err := requireTransition(unit.Estado, domain.EventDispensarPaciente, domain.ActorDispensingAgent)
	if err != nil {
		return nil, err
	}

	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if _, err := putUnit(ctx, unit); err != nil {
		return nil, err
	}

	if err := emitUnitEvent(ctx, opDispense, unit); err != nil {
		return nil, err
	}

	view := MedicationUnitView(unit)
	return &view, nil
}
