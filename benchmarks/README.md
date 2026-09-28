# Benchmarks

Este directorio contiene el contrato de metadatos de las corridas experimentales y, más adelante, los workloads de Hyperledger Caliper y los resultados de la comparación entre el prototipo Fabric y la baseline centralizada, conforme [`docs/measurement-protocol.md`](../docs/measurement-protocol.md).

Puntos fijados por el protocolo:

- dataset sintético compartido, generado con seed `20260727`, mínimo 50.000 unidades;
- estructura de evidencia cruda y resultados definida en la sección 9 del protocolo.

## Contrato de metadatos (DES-20)

Cada corrida deja un `metadata.json` que identifica de forma inequívoca qué se midió y sobre qué artefactos. Ese documento no es una lista libre de campos: se valida contra un contrato único y versionado, de modo que Fabric y la baseline no puedan producir evidencia con campos, unidades o identificadores distintos.

| Artefacto | Ruta |
|---|---|
| Contrato | [`schema/run-metadata.schema.json`](schema/run-metadata.schema.json) |
| Identificador | `urn:pfi-snt:run-metadata:schema:1.0.0` |
| Ejemplo Fabric | [`examples/run-metadata.fabric.json`](examples/run-metadata.fabric.json) |
| Ejemplo baseline | [`examples/run-metadata.baseline.json`](examples/run-metadata.baseline.json) |
| Ejemplo de carga mixta | [`examples/run-metadata.mixed-fabric.json`](examples/run-metadata.mixed-fabric.json) |

Los tres ejemplos validan contra el mismo contrato. Los dos primeros describen la misma ronda (`write-transfer`, repetición 1, 2 workers, 5 pares por segundo durante 120 s) en cada SUT, para que se vea qué cambia entre uno y otro: sólo el bloque `environment`.

### Validación

El validador vive en el módulo `client`, junto al generador del dataset:

```bash
cd client
go run ./cmd/runmeta ../benchmarks/examples/run-metadata.fabric.json
go run ./cmd/runmeta ../benchmarks/results/<corrida>/**/metadata.json
```

Devuelve `0` si todos los documentos cumplen el contrato y `1` en caso contrario, listando por documento cada incumplimiento con su ubicación dentro del JSON. Es apto para usarse como paso de verificación antes de procesar resultados.

Rechaza, entre otras cosas:

- cualquier campo no previsto, en la raíz o anidado (`additionalProperties: false` en todos los niveles);
- una corrida de Fabric sin `contractVersion` o sin `packageID`, y una de la baseline sin `baselineCommit` o sin `baselineImage`;
- una corrida que declare los identificadores del otro SUT, o un escenario que no le corresponde;
- una tasa de transacciones que no derive de la tasa de operaciones conceptuales por las transacciones que cada operación implica;
- una mezcla que no sume 100 o cuyo promedio ponderado no coincida con lo declarado;
- una ventana entre `startedAt` y `endedAt` más corta que la duración que la corrida dice haber medido.

Las reglas puramente estructurales están en el schema, de modo que un workload escrito en otro lenguaje pueda validar contra el mismo archivo sin depender del binario Go. Las reglas aritméticas y temporales, que JSON Schema no puede expresar, las agrega el validador.

### Consumo por las issues de evaluación

| Issue | Uso del contrato |
|---|---|
| EVAL-1 · Setup de Caliper (#41) | El workload de Fabric emite `metadata.json` conforme al contrato al cerrar cada ronda, tomando `contractVersion` de `docs/api-contract.md` y `packageID` de `network/chaincode-package.lock`. |
| EVAL-3 · Mediciones equivalentes sobre la baseline (#43) | El runner de la baseline emite el mismo documento con `sut: baseline`, `baselineCommit` y `baselineImage`, y con el mismo `dataset.sha256` que la corrida de Fabric con la que se compara. |
| EVAL-6 · Corridas finales Fabric (#46) | Ninguna corrida se considera definitiva si su `metadata.json` no valida. `repetition` distingue el warm-up descartado de las cinco mediciones y de la serie extendida. |
| EVAL-7 · Corridas finales baseline y procesamiento (#47) | El procesamiento agrupa por `scenario`, `phase` y `repetition`, y usa `rate.targetOperationsPerSecond` frente a `rate.targetTransactionsPerSecond` para no comparar pares de transferencia contra transacciones sueltas. |

La paridad entre ambos SUT es verificable de forma automática: dos corridas comparables deben coincidir en `scenario`, `dataset.sha256`, `workers`, `durationSeconds` y el bloque `rate` completo, y diferir únicamente en `sut` y `environment`.
