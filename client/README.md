# Cliente Fabric Gateway

Cliente CLI para ejecutar operaciones de negocio e invocar o consultar el chaincode `snt` mediante
[Fabric Gateway](https://github.com/hyperledger/fabric-gateway). Implementa el alcance
de CLI-1 (#34) y CLI-2 (#35) contra el contrato público congelado en
[`docs/api-contract.md`](../docs/api-contract.md) (v2.7.1).

Es una interfaz de línea de comandos; no incluye frontend.

## Requisitos

- Go 1.23 o posterior.
- Red levantada, canal `snt-channel` creado y chaincode `snt` desplegado.
- Material criptográfico generado bajo `network/organizations/`.
- Identidades `User1` enroladas por NET-1.

El módulo usa `fabric-gateway` v1.8.0, compatible con la versión de Go
declarada por el repositorio.

## Construcción y pruebas

Desde `client/`:

```bash
go build ./...
go test -race ./...
go vet ./...
```

## Uso

```text
snt-client register-unit       --org <org> --gtin <gtin> --serial <serie> --lot <lote> --expiry <fecha>
snt-client dispatch-transfer   --org <org> --gtin <gtin> --serial <serie> --transient-file <archivo|->
snt-client receive-transfer    --org <org> --gtin <gtin> --serial <serie>
snt-client dispense            --org <org> --gtin <gtin> --serial <serie>
snt-client read-unit           --org <org> --gtin <gtin> --serial <serie>
snt-client unit-history        --org <org> --gtin <gtin> --serial <serie>
snt-client query-units-by-gtin --org <org> --gtin <gtin>
```

Los comandos tipados cubren los tres procesos core:

1. alta mediante `register-unit`;
2. transferencia, separada en `dispatch-transfer` y `receive-transfer`;
3. dispensación mediante `dispense`.

También exponen lectura puntual, historial cronológico y consulta por GTIN. Cada
comando arma exactamente el request público de `docs/api-contract.md`; el destino
de una transferencia nunca forma parte de ese argumento.

Ejemplo desde `client/`:

```bash
go run ./cmd/snt-client register-unit \
  --org lab \
  --gtin 07791234567898 \
  --serial SN-0001-ABCD \
  --lot L2026-014 \
  --expiry 2027-12-31

printf '%s' '{"destinatario":{"destino":"GLN:7791234500024"},"commercial":{"numeroRemito":"R-0001","numeroFactura":"F-0001","cantidad":1}}' |
  go run ./cmd/snt-client dispatch-transfer \
    --org lab \
    --gtin 07791234567898 \
    --serial SN-0001-ABCD \
    --transient-file -
```

Las respuestas se escriben en stdout. Si el payload es JSON, se indenta sin
modificar su contenido.

### Demo completa para la defensa

Con una red limpia ya levantada, el canal creado y `snt` desplegado:

```bash
make demo-core
```

El comando ejecuta y muestra nueve pasos:
`RegisterUnit`; despacho y recepción laboratorio → droguería; despacho y
recepción droguería → farmacia; `Dispense`; `ReadUnit`;
`GetUnitHistory`; y `QueryUnitsByGTIN`.

Los destinos se derivan de `network/organizations-manifest.json`. La demo usa
por defecto el GTIN `07791234567898`, la serie `DEMO-CORE-0001`, el lote
`LOTE-DEMO-2026` y vencimiento `2099-12-31`. El resultado es reproducible
contra un ledger recién creado. Para repetirla sin reiniciar el ledger se debe
usar otra serie, porque el contrato rechaza correctamente una unidad duplicada:

```bash
make demo-core DEMO_ARGS="--serial DEMO-CORE-0002"
```

Se pueden sobrescribir también `--gtin`, `--lot`, `--expiry`, `--channel`,
`--chaincode`, `--repo-root`, `--timeout` y `--retry-interval`.

### Acceso genérico

La interfaz de CLI-1 permanece disponible para cualquier función pública:

```text
snt-client query  --org <org> --function <name> [--arg <value> ...]
snt-client invoke --org <org> --function <name> [--arg <value> ...]
```

`--arg` es repetible y conserva el orden exigido por la función pública.
Los valores complejos se pasan como un único argumento JSON.

Ejemplos desde `client/`:

```bash
go run ./cmd/snt-client query \
  --org anmat \
  --function QueryUnitsByGTIN \
  --arg 07791234567898

go run ./cmd/snt-client query \
  --org farmacia \
  --function ReadUnit \
  --arg 07791234567898 \
  --arg SN-0001-ABCD

go run ./cmd/snt-client invoke \
  --org lab \
  --function RegisterUnit \
  --arg '{"gtin":"07791234567898","numeroSerie":"SN-0001-ABCD","lote":"L2026-014","fechaVencimiento":"2027-12-31"}'
```

## Organizaciones e identidades

`--org` admite las cuatro identidades requeridas por CLI-1:

| Valor | MSP | Gateway local | Identidad |
|---|---|---|---|
| `anmat` | `AnmatMSP` | `localhost:7051` | `User1@anmat.snt.local` |
| `lab` | `LabMSP` | `localhost:8051` | `User1@lab.snt.local` |
| `drogueria` | `DrogueriaMSP` | `localhost:9051` | `User1@drogueria.snt.local` |
| `farmacia` | `FarmaciaMSP` | `localhost:11051` | `User1@farmacia.snt.local` |

El MSP y el hostname del peer se leen de
`network/organizations-manifest.json`. El certificado, la clave privada
y la CA TLS se resuelven desde el layout generado por NET-1. La conexión valida
TLS contra la CA del peer de la organización seleccionada.

El Gateway descubre y coordina los endosos requeridos por las políticas vigentes.
Para entornos que no usan los puertos locales se pueden sobrescribir
`--gateway-endpoint` y `--tls-server-name`. También están
disponibles `--channel`, `--chaincode`, `--timeout`
y `--repo-root`.

## Transient data

Las claves privadas definidas por DES-5 (`commercial`,
`destinatario` y `devolucion`) se suministran mediante un
objeto JSON en un archivo:

```json
{
  "destinatario": {
    "destino": "GLN:7791234500048"
  },
  "commercial": {
    "numeroRemito": "R-0001-2026",
    "numeroFactura": "A-0001-00001234",
    "cantidad": 1
  }
}
```

```bash
go run ./cmd/snt-client invoke \
  --org drogueria \
  --function DispatchTransfer \
  --arg '{"gtin":"07791234567898","numeroSerie":"SN-0001-ABCD"}' \
  --transient-file /ruta/segura/transient.json
```

El valor `--transient-file -` lee el objeto desde stdin. No existe una
opción para incluir transient data directamente como argumento de proceso.

## Errores

`receive-transfer` reintenta únicamente el error contractual
`INTERNAL_ERROR` con `details.reintentable=true` y
`details.causa=PRIVATE_DATA_NOT_DISSEMINATED`, hasta consumir su timeout. Cada
reintento se informa en stderr como un evento JSON. Los demás errores, incluido
`RECEIVER_MISMATCH`, se devuelven inmediatamente.

Los errores del chaincode se conservan en stderr con el envelope de DES-5:

```json
{
  "code": "UNIT_NOT_FOUND",
  "message": "La unidad no existe.",
  "details": {
    "key": "..."
  }
}
```

Los fallos de transporte o plataforma no clasificables se expresan como
`INTERNAL_ERROR`. En esos casos, `details` incluye la etapa,
una clasificación operativa y la causa original normalizada y truncada a 1024
caracteres. Los rechazos de validación de Fabric incluyen además
`validationCode` y `transactionId`.

Los códigos de salida son `0` para éxito,
`1` para fallos de ejecución y `2` para uso inválido de la CLI.

## Generador de dataset sintético (CLI-3)

El comando `datasetgen` produce una única receta de carga para ambos
backends. La semilla `20260727` es fija y no se expone como parámetro,
de modo que una misma versión del generador y de las fuentes de dominio produce
el mismo archivo.

Desde `client/`:

```bash
make generate-dataset
```

El comando equivalente y sus parámetros explícitos son:

```bash
go run ./cmd/datasetgen --units 50000 --output-dir ../build/dataset
```

- `--units`: cantidad de unidades; el mínimo aceptado es 50.000.
- `--output-dir`: directorio del bundle; por defecto,
  `../build/dataset`.

El bundle contiene:

| Archivo | Contenido |
|---|---|
| `dataset.json` | Recetas de registro, transferencias y dispensación o rechazo esperado. |
| `manifest.json` | Semilla, parámetros, versión del generador, versiones de las fuentes, organizaciones y hash del dataset. |
| `dataset.sha256` | Hash SHA-256 verificable con herramientas convencionales. |

Cada camino feliz comienza con `RegisterUnit`, representa cada
transferencia como el par `DispatchTransfer` +
`ReceiveTransfer` y termina con `Dispense`. Los casos inválidos
preparan primero un custodio alcanzable mediante pares válidos y luego describen
un `DispatchTransfer` que debe rechazarse con
`TRANSFER_NOT_AUTHORIZED`. Se incluyen tanto prohibiciones explícitas
como decisiones por ausencia de regla.

Las fechas de vencimiento se generan deliberadamente entre 2099-01-01 y
2101-12-31 para que todas las unidades permanezcan vigentes durante las rondas
de CLI-3. Los rechazos por unidad vencida no forman parte de este dataset:
corresponden a EXT-2 (#28).

Los pares se derivan en tiempo de generación mediante
`domain.DecideTransfer`; no existe una segunda matriz en el cliente.
Las organizaciones se leen de la copia embebida y verificada del manifiesto
fundacional.

Los formatos cerrados, versión `1.0.0`, están definidos con JSON Schema
Draft 2020-12 en:

- `dataset/schema/dataset.schema.json`
- `dataset/schema/manifest.schema.json`

## Validación

```bash
make test
make vet
make build
```

Las pruebas verifican, entre otras invariantes, GTIN-14 con dígito de control,
seriales válidos y únicos, cobertura de ambos tipos de rechazo, pares
despacho-recepción y reproducibilidad byte a byte.
