package snt

import (
	"encoding/json"
	"fmt"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/statebased"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Este archivo implementa los dos mecanismos de plataforma con los que ADR-007
// (punto 6) materializa la tabla de endoso de DES-6:
//
//   - el state-based endorsement (SBE) por clave publica, para los requisitos
//     derivables del estado ya confirmado (politica de reposo y de transito);
//   - el marcador de participacion en la coleccion implicita de una
//     organizacion, para exigir el endoso de una organizacion que no es la
//     titular de la clave escrita, o para exigirlo en la PRIMERA escritura de
//     una clave, donde SBE todavia no puede aplicarse.
//
// Tres reglas de plataforma condicionan el diseno y conviene tenerlas presentes
// al leer este codigo (ADR-007, punto 6):
//
//  1. Fabric valida una transaccion contra la politica que la clave tenia ANTES
//     de esa transaccion: lo que se escribe aca rige las escrituras posteriores.
//  2. La politica es de la CLAVE, no de la funcion: una rama disyuntiva
//     agregada para un caso excepcional habilita todos los casos ordinarios.
//     Por eso ninguna politica de clave de unidad admite a la organizacion
//     regulatoria como rama alternativa.
//  3. Escribir en una coleccion arrastra su politica de endoso. La coleccion
//     implicita de una organizacion lleva la politica de esa organizacion y
//     rige desde el despliegue.

// implicitCollection devuelve el nombre de la coleccion implicita de una
// organizacion. Fabric la crea automaticamente para toda organizacion del canal
// y NO se declara en collections_config.json (ADR-006, "Marcadores de
// participacion en colecciones implicitas").
func implicitCollection(mspID string) string {
	return "_implicit_org_" + mspID
}

// participationMarker es el contenido del marcador. Es determinístico: lo
// calcula el chaincode a partir de la operacion, el mspId del invocador y el
// timestamp de la transaccion. No transporta informacion de negocio, no viaja
// por transient y no requiere nada del cliente.
type participationMarker struct {
	Operacion string `json:"operacion"`
	MSPID     string `json:"mspId"`
	Timestamp string `json:"timestamp"`
}

func marshalMarker(ctx contractapi.TransactionContextInterface, operation, invokerMSPID string) ([]byte, error) {
	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(participationMarker{
		Operacion: operation,
		MSPID:     invokerMSPID,
		Timestamp: timestamp,
	})
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo serializar el marcador de participacion")
	}
	return payload, nil
}

// writeUnitParticipationMarker escribe el marcador de la variante `Unidad`:
// clave Participacion+[Unidad, gtin, numeroSerie, txId] en la coleccion
// implicita de ownerMSPID.
//
// El efecto buscado no es el dato sino la politica: al escribir en esa
// coleccion, la plataforma exige el endoso del peer de ownerMSPID en ESTA misma
// transaccion. El txId va ultimo, con lo que la clave es unica por transaccion
// y no hay contencion MVCC -- condicion para que las 50.000 altas del dataset
// de medicion no se serialicen (ADR-007, punto 6.g).
func writeUnitParticipationMarker(
	ctx contractapi.TransactionContextInterface,
	ownerMSPID, operation, invokerMSPID, gtin, numeroSerie string,
) error {
	stub := ctx.GetStub()
	key, err := unitParticipationKey(stub, gtin, numeroSerie, stub.GetTxID())
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la clave del marcador de participacion")
	}
	payload, err := marshalMarker(ctx, operation, invokerMSPID)
	if err != nil {
		return err
	}
	if err := stub.PutPrivateData(implicitCollection(ownerMSPID), key, payload); err != nil {
		return cerr.Internal(err, fmt.Sprintf("no se pudo escribir el marcador de participacion de %s", ownerMSPID))
	}
	return nil
}

// writeOrganizationParticipationMarker escribe el marcador de la variante
// `Organizacion`: clave Participacion+[Organizacion, mspId, txId]. Es la
// variante de las operaciones del registro organizacion-establecimiento, que no
// recaen sobre una unidad y por lo tanto no tienen GTIN ni numero de serie
// (ADR-007, punto 6, "Marcador de participacion").
func writeOrganizationParticipationMarker(
	ctx contractapi.TransactionContextInterface,
	ownerMSPID, operation, invokerMSPID, targetMSPID string,
) error {
	stub := ctx.GetStub()
	key, err := organizationParticipationKey(stub, targetMSPID, stub.GetTxID())
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la clave del marcador de participacion")
	}
	payload, err := marshalMarker(ctx, operation, invokerMSPID)
	if err != nil {
		return err
	}
	if err := stub.PutPrivateData(implicitCollection(ownerMSPID), key, payload); err != nil {
		return cerr.Internal(err, fmt.Sprintf("no se pudo escribir el marcador de participacion de %s", ownerMSPID))
	}
	return nil
}

// setKeyEndorsement fija la politica de endoso basada en estado de una clave
// publica. Con un unico mspId la politica exige a esa organizacion; con varios,
// statebased construye la conjuncion de todas -- que es exactamente la
// semantica que ADR-007 punto 6.b necesita para AND(emisor, receptor).
//
// Ninguna llamada de este chaincode debe incluir a la organizacion regulatoria
// junto a otra como rama alternativa: statebased no produce disyunciones y
// ADR-007 punto 6.a prohibe expresamente esa rama.
func setKeyEndorsement(ctx contractapi.TransactionContextInterface, key string, mspIDs ...string) error {
	if len(mspIDs) == 0 {
		return cerr.Internal(fmt.Errorf("sin organizaciones"), "politica de endoso por clave vacia")
	}

	policy, err := statebased.NewStateEP(nil)
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la politica de endoso por clave")
	}
	if err := policy.AddOrgs(statebased.RoleTypePeer, mspIDs...); err != nil {
		return cerr.Internal(err, "no se pudo agregar organizaciones a la politica de endoso por clave")
	}
	encoded, err := policy.Policy()
	if err != nil {
		return cerr.Internal(err, "no se pudo serializar la politica de endoso por clave")
	}
	if err := ctx.GetStub().SetStateValidationParameter(key, encoded); err != nil {
		return cerr.Internal(err, "no se pudo fijar la politica de endoso por clave")
	}
	return nil
}
