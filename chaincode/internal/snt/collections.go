package snt

import (
	"encoding/json"
	"sort"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Este archivo implementa el acceso a las colecciones explicitas por par de
// organizaciones de ADR-006: la resolucion determinística del nombre y el ciclo
// de vida activo/cerrado del registro de operacion de ADR-004.

// pairCollectionPrefix es el prefijo de las colecciones explicitas por par que
// genera la herramienta de NET-5 (#24) en collections_config.json.
const pairCollectionPrefix = "transfer_"

// pairCollectionName resuelve el nombre de la coleccion del par de
// organizaciones: `transfer_<mspIdA>_<mspIdB>` con los identificadores
// ordenados LEXICOGRAFICAMENTE, no por origen -> destino (ADR-006, punto 1).
//
// El orden lexicografico hace que la coleccion sea la misma cualquiera sea el
// sentido del flujo: la transferencia en la direccion autorizada y la
// devolucion en sentido inverso resuelven al mismo nombre desde cualquier peer
// endosante, sin tablas de mapeo y sin depender de quien invoca.
func pairCollectionName(mspIDA, mspIDB string) string {
	pair := []string{mspIDA, mspIDB}
	sort.Strings(pair)
	return pairCollectionPrefix + pair[0] + "_" + pair[1]
}

// TransferOperation es el registro de operacion de ADR-004, persistido en la
// coleccion privada del par. Contiene el destinatario declarado, los datos
// documentales del transient `commercial` y el ruleId/schemaVersion de la
// matriz que autorizo el par (ADR-006, punto 5; ADR-008, punto 3).
//
// Las contrapartes se guardan como identificador canonico GLN/CUFE y no como
// mspId: el mspId es un detalle de configuracion de red y el registro del
// ledger es la unica fuente de verdad que los vincula (ADR-003).
type TransferOperation struct {
	GTIN                  string `json:"gtin"`
	NumeroSerie           string `json:"numeroSerie"`
	TxIDDespacho          string `json:"txIdDespacho"`
	Emisor                string `json:"emisor"`
	DestinatarioPendiente string `json:"destinatarioPendiente"`

	// ruleId y schemaVersion de la matriz que autorizo el par. Es lo que el
	// peer del receptor contrasta contra su propia matriz embebida en la
	// recepcion (ADR-008, punto 5) y lo que hace verificable la comprobacion 5
	// de ADR-011.
	RuleID        string `json:"ruleId"`
	SchemaVersion string `json:"schemaVersion"`

	// Datos documentales del transient `commercial` (ADR-002: informacion
	// comercial y documental, nunca estado publico del canal).
	NumeroRemito  string `json:"numeroRemito"`
	NumeroFactura string `json:"numeroFactura"`
	Cantidad      int    `json:"cantidad"`

	DespachadaEn string `json:"despachadaEn"`

	// Campos que solo lleva el registro historico, al cerrarse la operacion.
	CerradaEn    string          `json:"cerradaEn,omitempty"`
	MotivoCierre string          `json:"motivoCierre,omitempty"`
	Recepcion    *CommercialData `json:"recepcion,omitempty"`
}

// CommercialData son los datos documentales que viajan por el transient
// `commercial`. Nunca son argumentos publicos de la transaccion.
type CommercialData struct {
	NumeroRemito  string `json:"numeroRemito"`
	NumeroFactura string `json:"numeroFactura"`
	Cantidad      int    `json:"cantidad"`
}

// Motivos de cierre del registro de operacion. Al salir de EN_TRANSITO por
// cualquier via la operacion deja de estar activa y su registro se conserva
// como historial auditable (ADR-004, regla 4).
const (
	closureReception     = "RECEPCION"
	closureRejection     = "RECHAZO"
	closureExtraordinary = "EVENTO_EXTRAORDINARIO"
)

// Claves del campo transient del contrato (docs/api-contract.md).
const (
	transientDestinatario = "destinatario"
	transientCommercial   = "commercial"
	transientDevolucion   = "devolucion"
)

type destinatarioTransient struct {
	Destino string `json:"destino"`
}

// readTransient devuelve el valor de una clave del campo transient de la
// propuesta. `found` distingue "no vino" de "vino vacio".
func readTransient(ctx contractapi.TransactionContextInterface, key string) ([]byte, bool, error) {
	transient, err := ctx.GetStub().GetTransient()
	if err != nil {
		return nil, false, cerr.Internal(err, "no se pudo leer el campo transient de la propuesta")
	}
	value, found := transient[key]
	return value, found, nil
}

// putActiveTransferOperation escribe el registro de la operacion ACTIVA bajo la
// clave TransferOpActive+[gtin, numeroSerie]. Por construccion existe a lo sumo
// uno por unidad (ADR-004, regla 2).
func putActiveTransferOperation(
	ctx contractapi.TransactionContextInterface,
	collection string,
	op TransferOperation,
) error {
	key, err := transferOpActiveKey(ctx.GetStub(), op.GTIN, op.NumeroSerie)
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la clave del registro de operacion activa")
	}
	payload, err := json.Marshal(op)
	if err != nil {
		return cerr.Internal(err, "no se pudo serializar el registro de operacion")
	}
	if err := ctx.GetStub().PutPrivateData(collection, key, payload); err != nil {
		return cerr.Internal(err, "no se pudo escribir el registro de operacion en la coleccion del par")
	}
	return nil
}

// readActiveTransferOperation lee el registro de la operacion activa de una
// coleccion resolviendo, en el mismo paso, la ambiguedad que ADR-006 (punto 1)
// obliga a distinguir:
//
//   - found=false con err nil: el ledger PUBLICO no registra ninguna escritura
//     viva de la clave en esa coleccion. Es concluyente y vale en todos los
//     peers: aca nunca hubo operacion activa. Que codigo del contrato
//     corresponde a esa ausencia lo decide el llamador segun su contexto;
//   - err = errPrivateDataNotDisseminated: el hash publico existe pero el
//     contenido privado todavia no es legible desde este peer. Es la condicion
//     TRANSITORIA y reintentable;
//   - found=true: el registro esta disponible.
//
// El orden de las dos consultas no es intercambiable, y es la razon por la que
// esta decision vive aca dentro y no en el llamador. Fabric NO devuelve
// (nil, nil) cuando el hash esta confirmado y el dato privado todavia no llego:
// el query helper del peer compara la version del hash publico con la del dato
// privado y, si difieren, la lectura falla con `private data matching public
// hash version is not available`. Consultar el hash DESPUES de una lectura
// privada que se asumia vacia nunca llegaria a ejecutarse: el error de la
// lectura sepultaria la condicion transitoria bajo un INTERNAL_ERROR generico,
// indistinguible de cualquier otra falla de plataforma -- exactamente lo que
// ADR-006 y esta issue exigen separar.
func readActiveTransferOperation(
	ctx contractapi.TransactionContextInterface,
	collection, gtin, numeroSerie string,
) (TransferOperation, bool, error) {
	key, err := transferOpActiveKey(ctx.GetStub(), gtin, numeroSerie)
	if err != nil {
		return TransferOperation{}, false, cerr.Internal(err, "no se pudo construir la clave del registro de operacion activa")
	}

	written, err := activeTransferOperationIsWritten(ctx, collection, gtin, numeroSerie)
	if err != nil {
		return TransferOperation{}, false, err
	}
	if !written {
		return TransferOperation{}, false, nil
	}

	raw, err := ctx.GetStub().GetPrivateData(collection, key)
	if err != nil {
		// Hay hash confirmado en el estado publico y la lectura privada falla:
		// desde este peer el contenido no esta disponible todavia. La causa
		// subyacente viaja en los detalles para no perder diagnostico.
		return TransferOperation{}, false, errPrivateDataNotDisseminated(gtin, numeroSerie, collection, err)
	}
	if raw == nil {
		// Con el hash escrito Fabric no deberia devolver vacio sin error, pero
		// si lo hiciera el significado seria el mismo: la operacion existe y
		// este peer no la ve.
		return TransferOperation{}, false, errPrivateDataNotDisseminated(gtin, numeroSerie, collection, nil)
	}

	var op TransferOperation
	if err := json.Unmarshal(raw, &op); err != nil {
		return TransferOperation{}, false, cerr.Internal(err, "registro de operacion corrupto")
	}
	return op, true, nil
}

// closeTransferOperation materializa el punto 4 del ciclo de vida de ADR-004:
// escribe el registro historico bajo TransferOp+[gtin, numeroSerie,
// txIdDespacho] y elimina la clave activa con DelPrivateData.
//
// La eliminacion borra el contenido de la base de datos privada de los peers
// miembros, pero el hash de la escritura original permanece en el ledger del
// canal como evidencia inmutable de que la operacion existio (ADR-006, punto 4).
//
// Es reutilizable por las tres vias de salida de EN_TRANSITO: recepcion (T04),
// rechazo (T05) y los eventos extraordinarios que cierran el transito
// (T09, T13-T16), que implementan las issues EXT.
func closeTransferOperation(
	ctx contractapi.TransactionContextInterface,
	collection string,
	op TransferOperation,
	motivo string,
	recepcion *CommercialData,
) error {
	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return err
	}

	historical := op
	historical.CerradaEn = timestamp
	historical.MotivoCierre = motivo
	historical.Recepcion = recepcion

	histKey, err := transferOpKey(ctx.GetStub(), op.GTIN, op.NumeroSerie, op.TxIDDespacho)
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la clave del registro historico de operacion")
	}
	payload, err := json.Marshal(historical)
	if err != nil {
		return cerr.Internal(err, "no se pudo serializar el registro historico de operacion")
	}
	if err := ctx.GetStub().PutPrivateData(collection, histKey, payload); err != nil {
		return cerr.Internal(err, "no se pudo escribir el registro historico de operacion")
	}

	activeKey, err := transferOpActiveKey(ctx.GetStub(), op.GTIN, op.NumeroSerie)
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la clave del registro de operacion activa")
	}
	if err := ctx.GetStub().DelPrivateData(collection, activeKey); err != nil {
		return cerr.Internal(err, "no se pudo eliminar el registro de operacion activa")
	}
	return nil
}

// activeTransferOperationIsWritten informa si el ledger PUBLICO del canal
// registra una escritura viva de la clave TransferOpActive en esa coleccion.
//
// Es la pieza que permite distinguir dos situaciones que, mirando solo el
// contenido privado, son indistinguibles: "la operacion existe pero su
// contenido todavia no llego a este peer" y "aca nunca hubo ninguna operacion".
// La consulta readActiveTransferOperation, que es la unica que la usa, la
// ejecuta ANTES de la lectura privada, porque despues ya seria tarde: ver el
// comentario de esa funcion.
// Fabric persiste, por cada escritura privada, el hash de clave y valor en el
// estado publico del canal (ADR-006, punto 6), y GetPrivateDataHash lo lee sin
// exigir membresia en la coleccion ni que el dato se haya diseminado.
//
// Al cerrar una operacion, DelPrivateData elimina tambien esa entrada del estado
// publico, de modo que un registro historico no deja hash vivo y no puede
// confundirse con una operacion en curso.
func activeTransferOperationIsWritten(
	ctx contractapi.TransactionContextInterface,
	collection, gtin, numeroSerie string,
) (bool, error) {
	key, err := transferOpActiveKey(ctx.GetStub(), gtin, numeroSerie)
	if err != nil {
		return false, cerr.Internal(err, "no se pudo construir la clave del registro de operacion activa")
	}
	hash, err := ctx.GetStub().GetPrivateDataHash(collection, key)
	if err != nil {
		return false, cerr.Internal(err, "no se pudo leer el hash del registro de operacion")
	}
	return len(hash) > 0, nil
}

// pairCollectionExists indica si ADR-006 define una coleccion para el par de
// organizaciones: existe cuando la matriz autoriza una transferencia entre sus
// agentType en ALGUNA de las dos direcciones.
//
// Comprobarlo antes de resolver el nombre es lo que evita que el chaincode
// intente leer o escribir una coleccion inexistente y falle con un error
// interno de la plataforma en lugar de con un codigo del contrato
// (ADR-009, punto 2).
func pairCollectionExists(a, b OrganizationRecord) (bool, error) {
	forward, err := domain.DecideTransfer(a.AgentType, b.AgentType)
	if err != nil {
		return false, cerr.Internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if forward.Allowed {
		return true, nil
	}
	backward, err := domain.DecideTransfer(b.AgentType, a.AgentType)
	if err != nil {
		return false, cerr.Internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	return backward.Allowed, nil
}

// findActiveTransferOperation busca el registro de la operacion activa de una
// unidad recorriendo las colecciones de las que el invocador es miembro.
//
// Existe porque el nombre de la coleccion depende de AMBAS contrapartes y el
// emisor no conoce al destinatario declarado sin leer el propio registro: el
// destinatario es un dato privado que, por decision de ADR-004, no esta en el
// estado publico. Cuando el invocador es el emisor -- o la organizacion
// regulatoria, miembro de todas las colecciones del par -- hay que buscarlo.
//
// El recorrido es determinístico: las organizaciones se ordenan por mspId y las
// colecciones candidatas son exactamente aquellas que ADR-006 define y de las
// que el invocador es miembro, de modo que todos los peers endosantes producen
// el mismo read-set.
//
// La coleccion se localiza por el HASH publico y no por el contenido privado:
// el hash existe desde que el despacho se confirma, en todos los peers y sin
// depender de la diseminacion, de modo que la busqueda es determinística y
// distingue "esta es la coleccion pero todavia no tengo el contenido" de "aca
// no hay ninguna operacion". Esa separacion la resuelve
// readActiveTransferOperation, que devuelve la falla transitoria ya tipificada;
// aqui found=false significa unicamente que NINGUNA coleccion candidata
// registra una operacion activa de la unidad.
//
// La recepcion NO usa esta funcion: ahi la contraparte se deriva del custodio
// publico y basta una lectura, que es lo que corresponde en el camino que mide
// DES-7.
func findActiveTransferOperation(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	invoker Invoker,
) (op TransferOperation, collection string, found bool, err error) {
	orgs, listErr := listOrganizations(ctx)
	if listErr != nil {
		return TransferOperation{}, "", false, listErr
	}
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].MSPID < orgs[j].MSPID })

	isRegulator := invoker.Org.AgentType == domain.AgentRegulator

	var candidates []string
	seen := map[string]struct{}{}
	addCandidate := func(a, b OrganizationRecord) error {
		exists, err := pairCollectionExists(a, b)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		name := pairCollectionName(a.MSPID, b.MSPID)
		if _, dup := seen[name]; dup {
			return nil
		}
		seen[name] = struct{}{}
		candidates = append(candidates, name)
		return nil
	}

	if isRegulator {
		// La organizacion regulatoria es miembro de TODAS las colecciones del
		// par por membresia {org A, org B, AnmatMSP} (ADR-006, punto 1).
		for i := range orgs {
			for j := i + 1; j < len(orgs); j++ {
				if err := addCandidate(orgs[i], orgs[j]); err != nil {
					return TransferOperation{}, "", false, err
				}
			}
		}
	} else {
		for _, org := range orgs {
			if org.MSPID == invoker.MSPID {
				continue
			}
			if err := addCandidate(invoker.Org, org); err != nil {
				return TransferOperation{}, "", false, err
			}
		}
	}

	// Cada candidata se resuelve consultando primero el hash publico: una
	// coleccion sin hash se descarta sin intentar la lectura privada, y una con
	// hash cuyo contenido todavia no llego a este peer corta el recorrido con la
	// falla transitoria tipificada en lugar de disfrazarse de "no hay nada aca".
	for _, candidate := range candidates {
		stored, ok, err := readActiveTransferOperation(ctx, candidate, unit.GTIN, unit.NumeroSerie)
		if err != nil {
			return TransferOperation{}, candidate, false, err
		}
		if !ok {
			continue
		}
		return stored, candidate, true, nil
	}
	return TransferOperation{}, "", false, nil
}

// errPrivateDataNotDisseminated describe la falla TRANSITORIA que ADR-006
// (punto 1) obliga a contemplar: con requiredPeerCount 1, el unico peer
// alcanzado durante el despacho puede ser el de la organizacion regulatoria, y
// el peer del receptor obtiene el registro de operacion despues, por pull del
// bloque privado o por reconciliacion en segundo plano.
//
// Hasta que eso ocurra, la recepcion falla porque el dato privado todavia no es
// legible, NO por una regla de negocio. El cliente debe reintentar de forma
// controlada, y el protocolo de medicion no debe contabilizar ese reintento
// como rechazo esperado.
//
// El catalogo del contrato no tiene un codigo para esta condicion, y agregarlo
// seria un cambio MINOR que una issue de implementacion no puede hacer: se usa
// INTERNAL_ERROR con el detalle `reintentable: true`, que es lo que distingue
// este caso de RECEIVER_MISMATCH o NOT_IN_TRANSIT en logs y evidencia.
//
// Solo readActiveTransferOperation lo construye, y unicamente despues de haber
// comprobado que el ledger publico registra la operacion en esa coleccion. Sin
// esa comprobacion previa este error se devolveria tambien a un invocador que no
// es el destinatario declarado -- su lectura tambien queda vacia --, y el
// contrato exige ahi RECEIVER_MISMATCH: un rechazo definitivo, no un reintento.
//
// `cause` es el error de plataforma que provoco el diagnostico, cuando lo hubo.
// Viaja en los detalles para no perder el mensaje original de Fabric: el cliente
// ramifica sobre `causa`, que es estable, y el operador lee `causaSubyacente`.
func errPrivateDataNotDisseminated(gtin, numeroSerie, collection string, cause error) error {
	details := map[string]any{
		"reintentable": true,
		"causa":        "PRIVATE_DATA_NOT_DISSEMINATED",
		"coleccion":    collection,
		"gtin":         gtin,
		"numeroSerie":  numeroSerie,
	}
	if cause != nil {
		details["causaSubyacente"] = cause.Error()
	}
	return cerr.New(cerr.InternalError,
		"el registro de la operacion de la unidad %s/%s todavia no es legible desde este peer; "+
			"reintentar hasta que la diseminacion o la reconciliacion de datos privados lo entregue",
		gtin, numeroSerie).
		WithDetails(details)
}
