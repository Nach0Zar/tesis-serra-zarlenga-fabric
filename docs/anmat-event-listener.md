# Listener de eventos para ANMAT

Este documento describe el comando `snt-client listen-anmat` de NET-8 (#64).
Su alcance es observar el canal existente; no modifica estados, roles,
políticas de endoso, colecciones privadas ni payloads emitidos por el
chaincode.

## Fundamento y alcance

La [Disposición ANMAT 3683/2011](https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/texto)
define «tiempo real» como la transmisión en línea en el momento en que se
produce el evento (art. 2.k), exige informar movimientos logísticos a ANMAT en
tiempo real e incluye cuarentena, producto robado/extraviado y vencido
(art. 8), y requiere que la autoridad pueda conocer irregularidades, anomalías
o desvíos en tiempo real (art. 9.e).

La decisión técnica de esta issue es usar los servicios de eventos de Fabric
Gateway para materializar esa observación en el prototipo. No se afirma que la
CLI sea por sí sola un sistema productivo de notificación regulatoria.

## Identidad y suscripciones

El comando:

- resuelve siempre el perfil `anmat`, exige `AnmatMSP` y no ofrece
  `--org`;
- abre `ChaincodeEvents` para el chaincode `snt`;
- abre `FilteredBlockEvents` para el canal `snt-channel`;
- anuncia disponibilidad solo después de que ambas suscripciones fueron
  establecidas.

Los nombres de canal y chaincode y los parámetros locales de conexión pueden
sobrescribirse con `--channel`, `--chaincode`, `--gateway-endpoint`,
`--tls-server-name` y `--repo-root`.

El registro de disponibilidad se escribe en stderr:

```json
{"type":"LISTENER_READY","profile":"anmat","mspId":"AnmatMSP","channel":"snt-channel","chaincode":"snt"}
```

La garantía operativa comienza después de ese registro. Sin
`--start-block`, Gateway entrega desde el siguiente bloque confirmado. Para
recuperar historia se puede indicar un bloque inicial inclusivo, incluido el
cero:

```bash
make -C client listen-anmat LISTENER_ARGS="--start-block 0"
```

El listener no persiste checkpoints. Un replay o un reinicio puede producir
duplicados, identificables por `transactionId`.

## Salida JSON Lines

stdout contiene un objeto JSON completo por línea.

Una de las cuatro alertas de negocio confirmadas:

```json
{"type":"VALID_BUSINESS_EVENT","blockNumber":42,"transactionId":"abc...","eventName":"ReportStolen","unit":{"gtin":"07791234567898","numeroSerie":"NET8-DEMO-001","lote":"L-NET8","fechaVencimiento":"2099-12-31","custodioActual":"GLN:7791234500048","estado":"ROBADO","ultimaActualizacion":"2026-09-19T18:30:00Z"}}
```

Los únicos nombres que producen una alerta son `Quarantine`,
`ReportExpired`, `ReportStolen` y `ReportLost`. Los demás eventos válidos
del catálogo DES-5 se consumen pero se omiten de la salida regulatoria.

Una transacción ordenada e incluida en un bloque, pero invalidada por los
peers:

```json
{"type":"INVALID_COMMITTED_TRANSACTION","blockNumber":43,"transactionId":"def...","transactionType":"ENDORSER_TRANSACTION","validationCode":"ENDORSEMENT_POLICY_FAILURE","validationCodeValue":10}
```

Este segundo flujo cubre todo el canal, no solo invocaciones del chaincode
`snt`. Usa bloques filtrados y por lo tanto no imprime argumentos,
`transient`, writesets ni datos de PDC.

## Tres resultados distintos

| Resultado | Llega al ledger | Salida del listener |
|---|---:|---|
| Evento de negocio de una transacción válida confirmada | Sí | `VALID_BUSINESS_EVENT` para una de las cuatro alertas |
| Transacción incluida pero invalidada durante validación | Sí | `INVALID_COMMITTED_TRANSACTION` con código simbólico y numérico |
| Propuesta rechazada antes del ordenamiento | No | Ninguna; el error vuelve al cliente que presentó la propuesta |

Un rechazo de propuesta no puede generar una alerta on-ledger porque no tiene
bloque, `transactionId` confirmado ni código de validación del commit. Agregar
telemetría off-ledger para esos rechazos queda fuera de NET-8.

Un payload de alerta que no sea un objeto JSON válido, un fallo al escribir la
salida o el cierre inesperado de cualquiera de los streams termina el proceso
con error. `SIGINT` y `SIGTERM` solicitan una cancelación limpia y terminan
con éxito.

## Demo farmacia → `ReportStolen`

Requisitos: red descartable levantada, canal creado, chaincode desplegado e
identidades `User1` generadas. Usar una serie nueva en cada ejecución.

Consola 1, desde la raíz del repositorio:

```bash
make -C client listen-anmat
```

Esperar el registro `LISTENER_READY`. Consola 2:

```bash
cd client
SERIAL=NET8DEMO001

go run ./cmd/snt-client register-unit \
  --org lab --gtin 07791234567898 --serial "$SERIAL" \
  --lot L-NET8 --expiry 2099-12-31

printf '%s' '{"destinatario":{"destino":"GLN:7791234500024"},"commercial":{"numeroRemito":"R-NET8-1","numeroFactura":"F-NET8-1","cantidad":1}}' |
  go run ./cmd/snt-client dispatch-transfer \
    --org lab --gtin 07791234567898 --serial "$SERIAL" --transient-file -

go run ./cmd/snt-client receive-transfer \
  --org drogueria --gtin 07791234567898 --serial "$SERIAL"

printf '%s' '{"destinatario":{"destino":"GLN:7791234500048"},"commercial":{"numeroRemito":"R-NET8-2","numeroFactura":"F-NET8-2","cantidad":1}}' |
  go run ./cmd/snt-client dispatch-transfer \
    --org drogueria --gtin 07791234567898 --serial "$SERIAL" --transient-file -

go run ./cmd/snt-client receive-transfer \
  --org farmacia --gtin 07791234567898 --serial "$SERIAL"

go run ./cmd/snt-client invoke \
  --org farmacia --function ReportStolen \
  --arg "{\"gtin\":\"07791234567898\",\"numeroSerie\":\"$SERIAL\",\"motivo\":\"sustracción informada por la farmacia\"}"
```

Después de que `ReportStolen` confirma el commit, la consola 1 debe mostrar en
segundos una línea `VALID_BUSINESS_EVENT` con el mismo `transactionId`,
`eventName` igual a `ReportStolen` y la vista pública en estado `ROBADO`.

## Comparación con la baseline centralizada

La baseline conserva cada cambio confirmado en la tabla append-only
`unit_events` y ofrece APIs REST de lectura puntual e historial. No ofrece un
transporte push ni un endpoint de streaming equivalente a los eventos de
Fabric. Por lo tanto, un observador regulatorio de la baseline debe consultar
periódicamente esas APIs o incorporar un mecanismo off-ledger separado.

Agregar polling, streaming, colas o notificaciones externas a la baseline no
forma parte de NET-8 y se mantiene fuera de esta implementación.
