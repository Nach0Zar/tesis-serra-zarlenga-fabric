package snt

import (
	"errors"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

var errNoActiveRegulator = errors.New("registro sin organizacion REGULATOR activa")

const (
	opWithdrawFromMarket = "WithdrawFromMarket"
	opProhibitProduct    = "ProhibitProduct"
)

// WithdrawFromMarket implementa T17, T18 y T19 de ADR-001: el retiro del
// mercado. Estado resultante: RETIRADO_MERCADO, bloqueante y no terminal --
// ADR-001 le conserva la reincorporacion (T27) y la disposicion final (T31).
//
// Actores habilitados: la organizacion regulatoria o el LABORATORIO titular.
// Esta es la primera operacion del contrato con un actor habilitado que puede
// NO ser el custodio actual ni el regulador, y de ahi su complejidad: un
// laboratorio que ya despacho la unidad sigue siendo responsable del producto
// que puso en el mercado, y la normativa le exige poder retirarlo.
//
// Cuando ese laboratorio no es el custodio actual, el contrato exige una
// AUTORIZACION PREVIA de intervencion en estado ACTIVA y no vencida, la marca
// CONSUMIDA y escribe marcadores de participacion en la coleccion implicita del
// laboratorio y en la de la organizacion regulatoria. Lo que la plataforma
// obtiene con eso es el endoso del laboratorio designado, del regulador y del
// custodio actual -- este ultimo impuesto por la politica de la clave de la
// unidad --, que es el par que pide DES-6 mas el custodio (ADR-007, punto 6.e).
//
// Durante el transito son CUATRO y no tres: la SBE de la clave exige a emisor y
// receptor declarado (ADR-007, punto 6.b), y a eso se suman los dos marcadores.
//
// Sin autorizacion, o con una vencida, ya consumida o revocada:
// LAB_INTERVENTION_REQUIRED. La autorizacion es por unidad, laboratorio y
// operacion, de modo que una emitida para otra operacion no habilita esta.
func (c *SNTContract) WithdrawFromMarket(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(
		ctx, req, domain.EventRetirarMercado, opWithdrawFromMarket, consumeLabInterventionIfNonCustodial)
}

// ProhibitProduct implementa T20: la prohibicion de un producto. Estado
// resultante: PROHIBIDO.
//
// Es la operacion con la potestad mas asimetrica del contrato y la unica de los
// eventos extraordinarios reservada EXCLUSIVAMENTE a la autoridad de
// aplicacion: ni el custodio actual ni el laboratorio titular pueden prohibir.
// Un invocador que no sea la organizacion regulatoria recibe REGULATORY_ONLY,
// que es un rechazo de autorizacion y no de transicion.
//
// Es tambien la de mayor alcance de origenes: ADR-001 la declara desde
// EN_LABORATORIO, EN_TRANSITO, EN_CUSTODIA, EN_CUARENTENA, DEVUELTO y
// RETIRADO_MERCADO. Un producto puede prohibirse en cualquier punto de la
// cadena, incluido uno ya retirado -- retiro y prohibicion no son grados de lo
// mismo: el retiro lo puede iniciar el titular, la prohibicion solo la
// autoridad, y esa diferencia es la que el trabajo demuestra como potestad
// diferenciada.
//
// PROHIBIDO es bloqueante y no terminal: ADR-001 conserva la disposicion final
// (T32) y la devolucion (T23), de modo que el producto prohibido todavia puede
// resolverse.
func (c *SNTContract) ProhibitProduct(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	invoker, err := resolveRegulator(ctx)
	if err != nil {
		return nil, err
	}
	_ = invoker
	return applyExtraordinaryEvent(ctx, req, domain.EventProhibirProducto, opProhibitProduct, nil)
}

// consumeLabInterventionIfNonCustodial es la precondicion de WithdrawFromMarket
// para el laboratorio no custodio (ADR-007, punto 6.e; contrato, nota de
// intervencion de un laboratorio no custodio).
//
// No se aplica cuando el invocador es el custodio actual ni cuando es la
// organizacion regulatoria: en esos dos casos la politica de la clave ya exige
// al peer que corresponde y no hay una tercera organizacion cuyo endoso haya
// que forzar.
func consumeLabInterventionIfNonCustodial(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	_ domain.Transition,
) error {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		return err
	}
	if invoker.Org.AgentType != domain.AgentLaboratory {
		return nil
	}
	if unit.CustodioActual == invoker.CanonicalID() {
		return nil
	}

	// LIMITE ARQUITECTONICO, no una omision de esta issue. ADR-001 habilita T19
	// desde EN_TRANSITO al laboratorio titular, pero ADR-006 punto 1 limita la
	// membresia de la coleccion del par a {emisor, receptor, regulador}: un
	// laboratorio ajeno a ese par NO es miembro y no puede leer ni cerrar el
	// registro de operacion activa. Y cerrar el transito es obligatorio para
	// salir de EN_TRANSITO (ADR-007, punto 6.c).
	//
	// Se rechaza con un codigo del contrato en lugar de dejar que falle en la
	// plataforma sobre una coleccion que el invocador no puede ver. La
	// contradiccion entre ADR-001 y ADR-006/ADR-007 debe resolverse antes de
	// habilitar este camino; mientras no se resuelva, el retiro durante el
	// transito lo puede iniciar ANMAT -- que si es miembro de toda coleccion de
	// par -- o el propio emisor, que es parte del par.
	if unit.Estado == domain.StateEnTransito {
		_, _, inPair, findErr := findActiveTransferOperation(ctx, unit, invoker)
		if findErr != nil {
			return findErr
		}
		if !inPair {
			return cerr.New(cerr.InvalidStateTransition,
				"un laboratorio ajeno al par de la transferencia en curso no puede retirar del mercado "+
					"una unidad EN_TRANSITO: no es miembro de la coleccion privada del par (ADR-006, punto 1) "+
					"y no puede cerrar el registro de la operacion activa, que ADR-007 punto 6.c exige "+
					"para salir de EN_TRANSITO").
				WithDetails(map[string]any{
					"estado":         string(unit.Estado),
					"custodioActual": unit.CustodioActual,
					"causa":          "LABORATORIO_AJENO_AL_PAR",
				})
		}
	}

	authorization, found, err := readLabIntervention(ctx, unit.GTIN, unit.NumeroSerie)
	if err != nil {
		return err
	}
	if !found {
		return labInterventionRequired(unit, "no existe autorizacion de intervencion para la unidad")
	}
	if authorization.Laboratorio != invoker.CanonicalID() {
		return labInterventionRequired(unit,
			"la autorizacion vigente designa a otro laboratorio")
	}
	if authorization.Operacion != LabOpWithdrawFromMarket {
		return labInterventionRequired(unit,
			"la autorizacion vigente habilita la operacion "+string(authorization.Operacion))
	}
	if authorization.Estado != LabInterventionActiva {
		return labInterventionRequired(unit,
			"la autorizacion esta en estado "+string(authorization.Estado))
	}
	// La vigencia se compara contra GetTxTimestamp(), no contra el reloj local:
	// cada peer endosante evaluaria time.Now() en un instante distinto y sus
	// read-write sets podrian no coincidir.
	expiresAt, err := time.Parse(time.RFC3339, authorization.ExpiraEn)
	if err != nil {
		return cerr.Internal(err, "la autorizacion persistida tiene un expiraEn invalido")
	}
	now, err := txTime(ctx)
	if err != nil {
		return err
	}
	if !expiresAt.After(now) {
		return labInterventionRequired(unit,
			"la autorizacion vencio el "+authorization.ExpiraEn)
	}

	// La autorizacion se CONSUME: es de un solo uso, y por eso ADR-007 punto 6.f
	// agrego RevokeLabIntervention en lugar de confiar en la reemision con una
	// fecha pasada. Dejarla ACTIVA habilitaria retiros sucesivos con una sola
	// intervencion de la autoridad.
	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return err
	}
	authorization.Estado = LabInterventionConsumed
	authorization.ConsumidaEn = timestamp
	if _, err := putLabIntervention(ctx, authorization); err != nil {
		return err
	}

	// Los dos marcadores que convierten la intervencion en coendoso real: el del
	// laboratorio designado y el de la organizacion regulatoria. La firma de
	// creador del laboratorio acredita identidad pero no es un endoso de peer.
	if err := writeUnitParticipationMarker(
		ctx, invoker.MSPID, opWithdrawFromMarket, invoker.MSPID,
		unit.GTIN, unit.NumeroSerie); err != nil {
		return err
	}
	regulator, err := activeRegulatorMSPID(ctx)
	if err != nil {
		return err
	}
	return writeUnitParticipationMarker(
		ctx, regulator, opWithdrawFromMarket, invoker.MSPID, unit.GTIN, unit.NumeroSerie)
}

func labInterventionRequired(unit MedicationUnit, detail string) error {
	return cerr.New(cerr.LabInterventionRequired,
		"un laboratorio no custodio exige una autorizacion de intervencion ACTIVA y vigente: %s",
		detail).
		WithDetails(map[string]any{
			"gtin":           unit.GTIN,
			"numeroSerie":    unit.NumeroSerie,
			"custodioActual": unit.CustodioActual,
		})
}

// activeRegulatorMSPID resuelve la organizacion regulatoria contra el registro,
// nunca contra un literal de MSP (ADR-010, punto 2). ADR-010 garantiza que hay
// exactamente una entrada REGULATOR activa, invariante que SetOrganizationActive
// sostiene con LAST_ACTIVE_REGULATOR.
func activeRegulatorMSPID(ctx contractapi.TransactionContextInterface) (string, error) {
	orgs, err := listOrganizations(ctx)
	if err != nil {
		return "", err
	}
	for _, org := range orgs {
		if org.AgentType == domain.AgentRegulator && org.Active {
			return org.MSPID, nil
		}
	}
	return "", cerr.Internal(errNoActiveRegulator,
		"el registro no tiene una organizacion REGULATOR activa")
}
