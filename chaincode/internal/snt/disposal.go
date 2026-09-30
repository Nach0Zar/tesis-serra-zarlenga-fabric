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
// como estado de origen, y por lo tanto que la unidad NO PUEDE SALIR de ese
// estado. Esa es la garantia, y no un rechazo uniforme de toda escritura: cada
// operacion valida lo suyo antes de llegar a la maquina de estados y devuelve
// la primera condicion que falla.
//
//   - Las trece que llegan a la transicion -- transferencia, dispensacion, los
//     eventos extraordinarios, Restock y esta misma --: INVALID_STATE_TRANSITION.
//   - ReceiveTransfer y RejectTransfer: NOT_IN_TRANSIT, porque comprueban antes
//     que la unidad este EN_TRANSITO.
//   - RegisterUnit: UNIT_ALREADY_EXISTS. No intenta una transicion, intenta
//     crear una clave que ya existe, y es lo que impide reciclar una unidad
//     dispuesta registrandola de nuevo.
//   - AuthorizeLabIntervention NO falla, y es un limite declarado del contrato:
//     la autorizacion que emite es inconsumible, porque ejercerla exigiria una
//     transicion que no existe.
//
// No hace falta una regla propia para nada de eso: la tabla de ADR-001 lo
// produce sola, y TestDisposedUnitCannotLeaveItsState lo comprueba sobre las
// diecisiete escrituras publicas.
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
// Registro del hecho y norma aplicable: el destino de una unidad dispuesta
// finalmente es un residuo peligroso. La Ley 24.051, Anexo I, categoria Y3
// ("Desechos de medicamentos y productos farmaceuticos") lo clasifica como tal
// -- https://www.argentina.gob.ar/normativa/nacional/450/actualizacion --.
//
// Esa ley NO queda satisfecha por esta operacion, y conviene no afirmarlo: sus
// arts. 12 a 14 imponen que el residuo circule acompanado de un MANIFIESTO que
// identifique a generador, transportista y planta de tratamiento o disposicion,
// con la naturaleza y cantidad del residuo y su origen y destino.
//
// Los tres sujetos no estan en la misma situacion, y la diferencia importa: el
// GENERADOR, que el art. 14 define como quien produce el residuo, ES una
// organizacion del SNT -- el custodio actual al disponer la unidad -- y esta
// operacion lo deja registrado, porque no mueve CustodioActual. El
// TRANSPORTISTA y la PLANTA no pertenecen a la cadena de trazabilidad de
// medicamentos de ADR-002 y no se modelan, y el manifiesto como documento no se
// emite. El faltante esta declarado como limite de alcance en
// docs/alcance-prototipo.md.
//
// Lo que esta operacion SI aporta son los dos datos que vuelven atribuible el
// hecho, y es lo que el criterio de EXT-8 pide del chaincode: `motivo`, que el
// motor exige y donde viaja la referencia normativa y el acto que la respalda, y
// CustodioActual, que esta operacion NO mueve, de modo que el responsable
// registrado al momento de la disposicion queda en la traza.
//
// Lo que el chaincode NO hace es validar el CONTENIDO de `motivo`: exigir que
// cite una norma seria una condicion de rechazo nueva que ningun ADR decide, y
// un texto libre no es verificable. Un campo dedicado para la referencia seria
// ademas un cambio MINOR del contrato congelado, que una issue de
// implementacion no puede hacer. La fecha sale de GetTxTimestamp() y la escribe
// el motor en UltimaActualizacion, nunca del reloj local.
//
// Auditoria de la autoridad, en sus dos alcances, ambos cubiertos. POR UNIDAD:
// las lecturas publicas del canal no son restringibles (ADR-005), de modo que
// ANMAT audita cualquier disposicion con ReadUnit y GetUnitHistory partiendo de
// su GTIN y numero de serie, y la transaccion emite ademas su evento. GLOBAL:
// QueryUnitsByState, que integro CC-9 (#112), enumera el conjunto de unidades
// en DISPUESTO_FINAL, y esta operacion mantiene el indice UnitByState porque
// escribe por putUnit como el resto -- TestDisposedUnitsAreEnumerableByRegulator
// lo comprueba disponiendo varias unidades y consultando el estado.
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
