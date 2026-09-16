package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const opFinalDisposition = "FinalDisposition"

// FinalDisposition implementa T28 a T33 de ADR-001: la salida definitiva de una
// unidad del sistema. Estado resultante: DISPUESTO_FINAL, el unico estado
// TERMINAL que estas seis transiciones alcanzan.
//
// Terminal significa que ADR-001 no declara ninguna fila con DISPUESTO_FINAL
// como estado de origen. No hace falta una regla propia que bloquee las
// operaciones posteriores: requireTransition las rechaza todas con
// INVALID_STATE_TRANSITION, dispensacion y despacho incluidos. Es la misma
// propiedad que el contrato ya usa para los estados bloqueantes, llevada al
// extremo -- aca no queda ninguna salida.
//
// Es la operacion con mas estados de origen del contrato despues de
// ProhibitProduct, y ADR-001 le asigna un actor distinto a casi cada fila:
//
//   - T28 (VENCIDO), T29 (DETERIORADO) y T33 (DEVUELTO):
//     RECOVERY_OR_DISPOSAL_AGENT unicamente, que ADR-009 punto 3 resuelve como
//     el custodio actual registrado. ANMAT NO es actor de estas tres: la unidad
//     esta fisicamente en poder de su custodio y es el quien ejecuta y
//     documenta la destruccion.
//   - T30 (EN_CUARENTENA): el custodio actual o ANMAT. La resolucion de una
//     cuarentena puede venir de la autoridad.
//   - T31 (RETIRADO_MERCADO): ANMAT, el LABORATORIO titular o el agente de
//     recupero.
//   - T32 (PROHIBIDO): ANMAT o el agente de recupero. El laboratorio titular NO
//     aparece, y es coherente con T20: la prohibicion es una potestad
//     exclusivamente regulatoria y su cierre no queda en manos del titular.
//
// La asimetria entre T31 y T27 merece leerse junta, porque dice algo que
// ninguna de las dos filas dice sola: desde RETIRADO_MERCADO el custodio PUEDE
// disponer finalmente la unidad (T31) pero NO puede reingresarla a stock (T27).
// Destruir lo retirado es una salida siempre admisible; devolverlo a la
// circulacion es una decision que solo el titular o la autoridad pueden tomar.
//
// Registro del hecho: `motivo` es obligatorio -- lo exige el motor comun -- y es
// donde viaja la causa regulatoria, incluida la referencia a la normativa de
// residuos peligrosos que la issue pide documentar. El contrato ya declara ese
// campo como el lugar de la causa regulatoria del evento, en texto breve y
// neutro; agregar un campo dedicado seria un cambio MINOR del contrato
// congelado, que una issue de implementacion no puede hacer. La fecha sale de
// GetTxTimestamp() y la escribe el motor en UltimaActualizacion, nunca del
// reloj local.
//
// Auditoria de la autoridad: las lecturas publicas del canal no son
// restringibles (ADR-005), de modo que ANMAT audita cualquier disposicion con
// ReadUnit y GetUnitHistory, y la transaccion emite ademas su evento de unidad.
// La enumeracion de todas las unidades en DISPUESTO_FINAL la aporta
// QueryUnitsByState, que implementa CC-9 (#112).
//
// El custodio NO cambia: ADR-004 acopla custodia y estado y solo T04 la mueve.
// El custodio registrado al momento de la disposicion queda en la traza como el
// responsable de haberla ejecutado, que es lo que la normativa de residuos
// peligrosos necesita poder atribuir.
func (c *SNTContract) FinalDisposition(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(
		ctx, req, domain.EventDisponerFinal, opFinalDisposition, consumeDisposalLabIntervention)
}

// consumeDisposalLabIntervention exige al laboratorio titular no custodio la
// autorizacion previa de intervencion con operacion FINAL_DISPOSITION. Solo T31
// lo habilita, de modo que en el resto de los origenes un laboratorio no
// custodio ni siquiera llega a evaluarse: la tabla lo rechaza antes.
//
// La autorizacion es POR OPERACION: una emitida para retirar del mercado no
// habilita disponer finalmente la unidad. La distincion importa mas aca que en
// las otras dos, porque la disposicion final es irreversible.
func consumeDisposalLabIntervention(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	_ domain.Transition,
) error {
	return consumeLabIntervention(ctx, unit, opFinalDisposition, LabOpFinalDisposition)
}
