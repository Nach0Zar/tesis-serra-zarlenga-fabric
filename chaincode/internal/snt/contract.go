// Package snt implementa el chaincode `snt` del prototipo del PFI.
//
// Su superficie publica esta congelada en docs/api-contract.md (v2.7.1) y su
// logica debe respetar, sin excepciones:
//
//   - ADR-001: la maquina de estados del medicamento. El paquete compartido
//     `domain` la reproduce y este chaincode la consulta; toda transicion no
//     declarada se rechaza con INVALID_STATE_TRANSITION.
//   - ADR-003 y ADR-010: la identidad se resuelve SIEMPRE por
//     cid.GetMSPID() -> entrada del registro -> agentType, nunca contra
//     literales de MSP.
//   - ADR-004: la transferencia son dos transacciones (despacho y recepcion)
//     mas el rechazo; el destinatario declarado viaja por transient y vive en
//     la coleccion privada del par.
//   - ADR-006: colecciones explicitas por par de organizaciones, con nombre
//     determinístico y las claves TransferOpActive / TransferOp / ReturnOp.
//   - ADR-007: el endoso se materializa por tres mecanismos combinados
//     (politica de chaincode, SBE por clave y marcadores de participacion en
//     colecciones implicitas).
//   - ADR-008: la matriz de transferencias se consume del paquete compartido
//     embebido por go:embed, sin ifs duplicados.
package snt

import (
	"encoding/json"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// ContractVersion es la version del contrato publico que implementa este
// chaincode (docs/api-contract.md).
const ContractVersion = "2.7.1"

// SNTContract es el contrato publico del chaincode `snt`.
//
// El nombre lo congela docs/api-contract.md, que declara la firma de las 25
// operaciones como metodos de `*SNTContract`. Por eso no se acepta la
// sugerencia de revive de renombrarlo a `Contract` para evitar el stutter
// `snt.SNTContract`: cambiarlo seria un cambio del contrato congelado, que
// exige su propio PR con aprobacion explicita.
//
//nolint:revive // nombre fijado por el contrato congelado (docs/api-contract.md)
type SNTContract struct {
	contractapi.Contract
}

// readUnit lee el estado publico de una unidad y devuelve UNIT_NOT_FOUND si no
// existe.
func readUnit(ctx contractapi.TransactionContextInterface, gtin, numeroSerie string) (MedicationUnit, error) {
	key, err := medicationUnitKey(ctx.GetStub(), gtin, numeroSerie)
	if err != nil {
		return MedicationUnit{}, cerr.Internal(err, "no se pudo construir la clave de la unidad")
	}
	raw, err := ctx.GetStub().GetState(key)
	if err != nil {
		return MedicationUnit{}, cerr.Internal(err, "no se pudo leer la unidad")
	}
	if raw == nil {
		return MedicationUnit{}, cerr.New(cerr.UnitNotFound,
			"la unidad %s/%s no existe", gtin, numeroSerie).
			WithDetails(map[string]any{"gtin": gtin, "numeroSerie": numeroSerie})
	}

	var unit MedicationUnit
	if err := json.Unmarshal(raw, &unit); err != nil {
		return MedicationUnit{}, cerr.Internal(err, "estado publico de la unidad corrupto")
	}
	return unit, nil
}

// putUnit persiste el estado publico de una unidad y devuelve su clave, para
// que la operacion pueda ajustar la politica de endoso por clave.
func putUnit(ctx contractapi.TransactionContextInterface, unit MedicationUnit) (string, error) {
	key, err := medicationUnitKey(ctx.GetStub(), unit.GTIN, unit.NumeroSerie)
	if err != nil {
		return "", cerr.Internal(err, "no se pudo construir la clave de la unidad")
	}
	payload, err := json.Marshal(unit)
	if err != nil {
		return "", cerr.Internal(err, "no se pudo serializar la unidad")
	}
	if err := ctx.GetStub().PutState(key, payload); err != nil {
		return "", cerr.Internal(err, "no se pudo escribir la unidad")
	}
	return key, nil
}

// notImplemented es la respuesta de las operaciones que este chaincode declara
// -- porque el contrato las congela y CC-1 (#14) exige tenerlas declaradas --
// pero cuya logica pertenece a otra issue.
//
// Se usa INTERNAL_ERROR porque el catalogo del contrato no tiene un codigo para
// "operacion declarada sin implementar", y agregarlo seria un cambio MINOR del
// contrato que una issue de implementacion no puede hacer. El detalle nombra la
// issue duena de la operacion.
func notImplemented(operation, owner string) error {
	return cerr.New(cerr.InternalError,
		"la operacion %s esta declarada por el contrato pero su implementacion pertenece a %s", operation, owner).
		WithDetails(map[string]any{"operacion": operation, "issue": owner})
}
