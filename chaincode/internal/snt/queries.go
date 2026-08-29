package snt

import (
	"encoding/json"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Operaciones de lectura del contrato. No mutan estado ni generan endoso de
// escritura, y se rigen por las politicas de visibilidad de lectura del canal y
// de las colecciones privadas (ADR-002).
//
// No resuelven la identidad del invocador contra el registro: el contrato no
// declara errores de autorizacion para estas operaciones, y ADR-005 reconoce
// expresamente que el acceso de lectura al estado publico del canal no puede
// restringirse por chaincode -- es una propiedad del modelo de canales de
// Fabric y un supuesto de confianza declarado del prototipo. La verificacion
// del financiador, que SI valida agentType y rol, es VerifyTrace (CC-8, #62).

// ReadUnit devuelve el estado publico de una unidad.
func (c *SNTContract) ReadUnit(
	ctx contractapi.TransactionContextInterface,
	gtin string,
	numeroSerie string,
) (*MedicationUnitView, error) {
	if err := validateUnitRef(gtin, numeroSerie); err != nil {
		return nil, err
	}
	unit, err := readUnit(ctx, gtin, numeroSerie)
	if err != nil {
		return nil, err
	}
	view := MedicationUnitView(unit)
	return &view, nil
}

// GetUnitHistory devuelve la traza completa de una unidad con
// GetHistoryForKey: el identificador de transaccion, el timestamp y el valor
// ENTERO de la clave en cada punto del historial.
//
// Es la operacion que demuestra el pilar de auditabilidad: permite reconstruir
// la secuencia de custodios y estados de una unidad desde su alta hasta su
// estado actual, sin depender de ningun indice adicional.
//
// Semantica que hereda de la plataforma (ADR-005, ADR-011): devuelve unicamente
// las modificaciones CONFIRMADAS de la clave. Los intentos rechazados -- por
// endoso, por validacion o por estado -- nunca llegan al world state y no
// aparecen aca: el historial audita lo que ocurrio, no lo que se intento.
func (c *SNTContract) GetUnitHistory(
	ctx contractapi.TransactionContextInterface,
	gtin string,
	numeroSerie string,
) ([]UnitHistoryEntry, error) {
	if err := validateUnitRef(gtin, numeroSerie); err != nil {
		return nil, err
	}

	entries, err := readUnitHistory(ctx, gtin, numeroSerie)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, cerr.New(cerr.UnitNotFound,
			"la unidad %s/%s no existe", gtin, numeroSerie).
			WithDetails(map[string]any{"gtin": gtin, "numeroSerie": numeroSerie})
	}
	return entries, nil
}

// readUnitHistory devuelve las modificaciones CONFIRMADAS de la clave de una
// unidad, en el orden en que Fabric las entrega.
//
// Existe como helper y no dentro de GetUnitHistory porque tiene dos consumidores
// con reglas distintas sobre el mismo dato: la operacion de lectura, que
// convierte el historial vacio en UNIT_NOT_FOUND, y la verificacion de
// autenticidad de ADR-013, que lo convierte en el veredicto NO_ENCONTRADA. La
// decision de que significa "vacio" es del llamador; leer el historial, no.
func readUnitHistory(
	ctx contractapi.TransactionContextInterface,
	gtin, numeroSerie string,
) ([]UnitHistoryEntry, error) {
	key, err := medicationUnitKey(ctx.GetStub(), gtin, numeroSerie)
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo construir la clave de la unidad")
	}
	iterator, err := ctx.GetStub().GetHistoryForKey(key)
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo leer el historial de la unidad")
	}
	defer func() { _ = iterator.Close() }()

	entries := []UnitHistoryEntry{}
	for iterator.HasNext() {
		modification, err := iterator.Next()
		if err != nil {
			return nil, cerr.Internal(err, "no se pudo leer una entrada del historial")
		}

		entry := UnitHistoryEntry{
			TxID:     modification.GetTxId(),
			IsDelete: modification.GetIsDelete(),
		}
		if ts := modification.GetTimestamp(); ts != nil {
			entry.Timestamp = ts.AsTime().UTC().Format(time.RFC3339)
		}
		if !modification.GetIsDelete() && len(modification.GetValue()) > 0 {
			var unit MedicationUnit
			if err := json.Unmarshal(modification.GetValue(), &unit); err != nil {
				return nil, cerr.Internal(err, "entrada del historial corrupta")
			}
			entry.Value = &unit
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// QueryUnitsByGTIN recupera todas las unidades registradas bajo un GTIN
// mediante GetStateByPartialCompositeKey.
//
// Es la unica consulta por criterio del contrato, y opera por clave compuesta
// parcial precisamente para que LevelDB alcance: ADR-007 (punto 2) descarto
// CouchDB porque ninguna operacion del contrato requiere rich queries.
//
// SIN paginacion, conforme la exclusion registrada en
// docs/alcance-prototipo.md: con el dataset sintetico de 50.000 unidades un
// GTIN puede acumular miles de resultados, y los bookmarks de Fabric agregan
// complejidad que no aporta a la hipotesis. La operacion no forma parte de las
// mediciones de DES-7.
//
// Una consulta por CUSTODIO no esta en el contrato y no se implementa aca: no
// es expresable como clave compuesta parcial, de modo que incorporarla exigiria
// revisar ADR-007 -- que ya decidio LevelDB -- ademas de un agregado MINOR al
// contrato.
func (c *SNTContract) QueryUnitsByGTIN(
	ctx contractapi.TransactionContextInterface,
	gtin string,
) ([]MedicationUnitView, error) {
	if err := validateGTIN(gtin); err != nil {
		return nil, err
	}

	iterator, err := ctx.GetStub().GetStateByPartialCompositeKey(objectTypeMedicationUnit, []string{gtin})
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo consultar las unidades del GTIN")
	}
	defer func() { _ = iterator.Close() }()

	units := []MedicationUnitView{}
	for iterator.HasNext() {
		kv, err := iterator.Next()
		if err != nil {
			return nil, cerr.Internal(err, "no se pudo leer una unidad del resultado")
		}
		var unit MedicationUnit
		if err := json.Unmarshal(kv.GetValue(), &unit); err != nil {
			return nil, cerr.Internal(err, "estado publico de la unidad corrupto")
		}
		units = append(units, MedicationUnitView(unit))
	}
	return units, nil
}
