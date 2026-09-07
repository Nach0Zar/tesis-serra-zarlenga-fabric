# Cliente

Este directorio contiene los artefactos compartidos por los consumidores de
Fabric y de la línea base. El cliente operativo con Fabric Gateway pertenece a
CLI-1 y CLI-2; CLI-3 incorpora aquí el generador reproducible del dataset.

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
corresponden a EXT-2 (#61).

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
