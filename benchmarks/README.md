# Benchmarks

Este directorio contiene el contrato de metadatos de las corridas
experimentales y los workloads de Hyperledger Caliper para Fabric. Los
resultados finales de la comparación con la baseline se incorporarán en las
issues de ejecución y procesamiento, conforme a
[`docs/measurement-protocol.md`](../docs/measurement-protocol.md).

Puntos fijados por el protocolo:

- dataset sintético compartido, generado con seed `20260727`, mínimo 50.000 unidades;
- estructura de evidencia cruda y resultados definida en la sección 9 del protocolo.

## Contrato de metadatos

Cada corrida deja un `metadata.json` que identifica de forma inequívoca qué se midió y sobre qué artefactos. Ese documento no es una lista libre de campos: se valida contra un contrato único y versionado, de modo que Fabric y la baseline no puedan producir evidencia con campos, unidades o identificadores distintos.

| Artefacto | Ruta |
|---|---|
| Contrato | [`schema/run-metadata.schema.json`](schema/run-metadata.schema.json) |
| Identificador | `urn:pfi-snt:run-metadata:schema:1.0.0` |
| Ejemplos | [`examples/`](examples/) |

Los siete ejemplos validan contra el mismo contrato y cada uno ejercita una rama condicional distinta:

| Ejemplo | Qué cubre |
|---|---|
| `run-metadata.fabric.json` | Escritura core en Fabric: `write-transfer`, repetición medida. |
| `run-metadata.baseline.json` | La misma ronda en la baseline. Sólo cambia el bloque `environment`. |
| `run-metadata.mixed-fabric.json` | Carga mixta y warm-up descartado. |
| `run-metadata.rejections-fabric.json` | Ronda de rechazo esperado, donde la transferencia vale **una** transacción. |
| `run-metadata.availability-fabric.json` | Escenario de disponibilidad con sus tres ventanas. |
| `run-metadata.preparation-fabric.json` | Construcción del snapshot inicial y marcadores de participación. |
| `run-metadata.smoke-baseline.json` | Smoke, acotado por cantidad y no por duración. |

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
- una ventana entre `startedAt` y `endedAt` más corta que la duración que la corrida dice haber medido;
- una ronda de rechazo que no declare su familia, o que la declare sobre una operación que el dataset no invoca para ella;
- un escenario de disponibilidad sin ventana de falla, o con una inyección fuera de la corrida o incoherente con los relojes;
- una preparación de snapshot en Fabric sin sus marcadores de participación, un desglose que no sume lo observado, o una tasa o proporción de marcadores que no derive de la duración y de las escrituras exitosas;
- marcadores observados sobre cero escrituras exitosas, que describe una corrida imposible;
- un contador por encima de 2^53-1, que un consumidor de doble precisión leería alterado;
- cualquier documento que no se pueda decodificar para evaluar las reglas semánticas: si no se pudieron comprobar, no se certifica.

Tres reglas merecen mención porque corrigen ambigüedades concretas del protocolo previo:

- **La transferencia vale dos transacciones sólo en el camino feliz.** En una ronda de rechazo esperado el dataset invoca `DispatchTransfer` y el rechazo se resuelve ahí: el par nunca se completa. El contrato exige `transactionsPerOperation: 1` en esas rondas, para que nadie declare una recepción que no se envió.
- **El smoke se acota por cantidad.** La sección 6.1 lo define en 30 transacciones por operación core, no en segundos, así que el contrato le exige `transactions` y le prohíbe `durationSeconds`.
- **Los escenarios de disponibilidad exigen la ventana de falla.** Sin `injectedAt` el crudo no se puede segmentar en pre-falla, falla y recuperación, y el tiempo de recuperación no es calculable.

Las reglas puramente estructurales están en el schema, de modo que un workload escrito en otro lenguaje pueda validar contra el mismo archivo sin depender del binario Go. Las reglas aritméticas y temporales, que JSON Schema no puede expresar, las agrega el validador.

### Uso en el flujo de evaluación

| Etapa | Uso del contrato |
|---|---|
| Smoke de Caliper | El workload de Fabric emite `metadata.json` conforme al contrato al cerrar cada ronda, tomando `contractVersion` de `docs/api-contract.md` y `packageID` de `network/chaincode-package.lock`. |
| Workloads de Fabric | Cada ronda individual produce crudos por operación, un resumen procesable, el reporte Caliper y metadata validada. |
| Mediciones sobre la baseline | El runner de la baseline emite el mismo documento con `sut: baseline`, `baselineCommit` y `baselineImage`, y con el mismo `dataset.sha256` que la corrida de Fabric con la que se compara. |
| Corridas finales de Fabric | Ninguna corrida se considera definitiva si su `metadata.json` no valida. `repetition` distingue el warm-up descartado de las cinco mediciones y de la serie extendida. |
| Corridas finales de baseline y procesamiento | El procesamiento agrupa por `scenario`, `phase` y `repetition`, y usa `rate.targetOperationsPerSecond` frente a `rate.targetTransactionsPerSecond` para no comparar pares de transferencia contra transacciones sueltas. |
| Disponibilidad de Raft y baseline | `faultInjection` fija el instante de la falla y las tres ventanas, que es lo que permite calcular tasa de éxito antes, durante y después, y el tiempo de recuperación de la sección 3.3. |
| Análisis cualitativo y estructural | `rejectionFamily` mantiene separadas las tres familias del dataset, de modo que el costo de validación de cada una sea comparable en lugar de promediarse. |

La paridad entre ambos SUT es verificable de forma automática: dos corridas comparables deben coincidir en `scenario`, `dataset.sha256`, `workers`, `durationSeconds` y el bloque `rate` completo, y diferir únicamente en `sut` y `environment`.

## Caliper local

El proyecto aislado [`caliper/`](caliper/) instala Caliper 0.7.1 con el binding
`fabric:fabric-gateway` y ejecuta una única ronda diagnóstica de `ReadUnit`:
1 worker, 30 consultas y tasa fija de 1 TPS. Consume una red ya desplegada y el
bundle determinístico de 50.000 unidades; no crea canales, no instala
chaincode y no sustituye las rondas medibles.

El mismo proyecto se reutiliza para las cinco operaciones core, la mezcla
determinística y las rondas separadas de rechazo. Las salidas procesables son
`raw.json`, `summary.json` y `metadata.json`; el HTML de Caliper es
complementario. El runner ejecuta una sola ronda y no adelanta la orquestación
de repeticiones ni el análisis estadístico final.

La configuración efectiva, el reporte HTML, el contexto y `metadata.json` se
guardan bajo `build/benchmarks/caliper/`, que está ignorado por Git. Los
prerrequisitos y comandos reproducibles están documentados en
[`caliper/README.md`](caliper/README.md).
