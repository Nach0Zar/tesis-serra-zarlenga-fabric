package snt

import (
	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
)

// Tipos de objeto de las claves compuestas del world state y de las
// colecciones privadas. Las claves publicas siguen modelo-datos.md §2.2 y
// ADR-007; las privadas, ADR-006 punto 4.
const (
	// objectTypeMedicationUnit + [gtin, numeroSerie] identifica una unidad en
	// el world state (modelo-datos.md §2.2). Usar clave compuesta -- y no
	// concatenar el string a mano -- es lo que habilita QueryUnitsByGTIN por
	// clave compuesta parcial sin indice secundario.
	objectTypeMedicationUnit = "MedicationUnit"

	// objectTypeOrganization + [mspId] identifica una entrada del registro
	// organizacion-establecimiento (ADR-003, extendido por ADR-010).
	objectTypeOrganization = "Organization"

	// objectTypeLabIntervention + [gtin, numeroSerie] identifica la
	// autorizacion previa de intervencion de un laboratorio no custodio
	// (ADR-007, punto 6.e). Es una clave por unidad: una autorizacion nueva
	// reemplaza a la anterior.
	objectTypeLabIntervention = "LabIntervention"

	// objectTypeTransferOpActive + [gtin, numeroSerie] es el registro de la
	// operacion de transferencia ACTIVA en la coleccion del par
	// (ADR-006, punto 4; ADR-004, regla 2: a lo sumo una por unidad).
	objectTypeTransferOpActive = "TransferOpActive"

	// objectTypeTransferOp + [gtin, numeroSerie, txIdDespacho] es el registro
	// historico de una operacion de transferencia cerrada (ADR-006, punto 4).
	objectTypeTransferOp = "TransferOp"

	// objectTypeReturnOp + [gtin, numeroSerie, txIdDevolucion] es el registro
	// historico e inmutable de una devolucion T21-T24 (ADR-006, punto 4;
	// ADR-009, punto 2). No tiene ciclo activo/cerrado.
	objectTypeReturnOp = "ReturnOp"

	// objectTypeParticipation es el marcador de participacion de ADR-007
	// punto 6. Su clave tiene dos variantes y el txId va SIEMPRE ultimo: hace
	// la clave unica por transaccion -- sin contencion MVCC -- y deja los
	// componentes anteriores disponibles para consulta por clave compuesta
	// parcial.
	objectTypeParticipation = "Participacion"

	// participationTargetUnit y participationTargetOrganization son el primer
	// componente de cada variante del marcador.
	participationTargetUnit         = "Unidad"
	participationTargetOrganization = "Organizacion"
)

func medicationUnitKey(stub shim.ChaincodeStubInterface, gtin, numeroSerie string) (string, error) {
	return stub.CreateCompositeKey(objectTypeMedicationUnit, []string{gtin, numeroSerie})
}

func organizationKey(stub shim.ChaincodeStubInterface, mspID string) (string, error) {
	return stub.CreateCompositeKey(objectTypeOrganization, []string{mspID})
}

func labInterventionKey(stub shim.ChaincodeStubInterface, gtin, numeroSerie string) (string, error) {
	return stub.CreateCompositeKey(objectTypeLabIntervention, []string{gtin, numeroSerie})
}

func transferOpActiveKey(stub shim.ChaincodeStubInterface, gtin, numeroSerie string) (string, error) {
	return stub.CreateCompositeKey(objectTypeTransferOpActive, []string{gtin, numeroSerie})
}

func transferOpKey(stub shim.ChaincodeStubInterface, gtin, numeroSerie, txIDDespacho string) (string, error) {
	return stub.CreateCompositeKey(objectTypeTransferOp, []string{gtin, numeroSerie, txIDDespacho})
}

func returnOpKey(stub shim.ChaincodeStubInterface, gtin, numeroSerie, txIDDevolucion string) (string, error) {
	return stub.CreateCompositeKey(objectTypeReturnOp, []string{gtin, numeroSerie, txIDDevolucion})
}

func unitParticipationKey(stub shim.ChaincodeStubInterface, gtin, numeroSerie, txID string) (string, error) {
	return stub.CreateCompositeKey(objectTypeParticipation,
		[]string{participationTargetUnit, gtin, numeroSerie, txID})
}

func organizationParticipationKey(stub shim.ChaincodeStubInterface, mspID, txID string) (string, error) {
	return stub.CreateCompositeKey(objectTypeParticipation,
		[]string{participationTargetOrganization, mspID, txID})
}
