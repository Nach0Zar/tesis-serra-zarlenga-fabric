# Resultados comparativos Fabric y baseline

Sólo entran mediciones de series citables con el estado inicial acreditado por el manifiesto de su snapshot.

Procesado de `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric`, `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline`. Cada fila agrega repeticiones medidas válidas de un mismo SUT, escenario, variante, tasa y condiciones de carga: no hay promedios entre workloads ni cargas distintas. Desvío y coeficiente de variación son poblacionales; los percentiles son nearest-rank por ronda y se promedian entre repeticiones.

## Artefactos medidos

- **Baseline**: commit `389ca160b2a490cd66bb0194f012e2b75b3598e8`, imagen `sha256:c9dc6ed2e16e61bd0e5cca5582f7069b1657c32741ee86d118cc9f71ef30be09`.
- **Fabric**: contrato `2.11.2`, package ID `snt_1.0:a4d9315a304ac10b467c096f37f4960f86e4ce2d828310aed58d5ad34082d9a9`, Fabric 2.5.16.
- **Dataset**: `de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d`.

## Rendimiento estable por SUT

| SUT | Escenario | Tasa | Serie | Repeticiones | Throughput (op/s) | CV thr. | p50 (ms) | p95 (ms) | CV p95 | p99 (ms) | tx confirmadas/s | intentos tx/s |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| baseline | Carga mixta | 20 | original | 1 2 3 4 5 | 20 ± 0 | 0 % | 7,8 | 11,2 | 13,12 % | 15,8 | 31 | 31 |
| fabric | Carga mixta | 20 | original | 1 2 3 4 5 | 19,46 ± 0,08 | 0,41 % | 2.605,8 | 4.336 | 0,71 % | 4.557,8 | 30,16 | 30,45 |
| baseline | Consulta de historial | 10 | original | 1 2 3 4 5 | 10 ± 0 | 0 % | 2 | 3 | 0 % | 5,8 | 10 | 10 |
| fabric | Consulta de historial | 10 | original | 1 2 3 4 5 | 10,03 ± 0 | 0 % | 5 | 9,2 | 8,13 % | 14,6 | 10,03 | 10,03 |
| baseline | Consulta de historial | 25 | original | 1 2 3 4 5 | 25 ± 0 | 0 % | 1 | 2,4 | 20,41 % | 4,6 | 25 | 25 |
| baseline | Consulta de historial | 25 | extendida (p95) | 1 2 3 4 5 6 7 8 | 25 ± 0 | 0 % | 1 | 2,3 | 19,25 % | 4,1 | 25 | 25 |
| fabric | Consulta de historial | 25 | original | 1 2 3 4 5 | 25,03 ± 0 | 0 % | 4,4 | 7,2 | 16,2 % | 12,4 | 25,03 | 25,03 |
| fabric | Consulta de historial | 25 | extendida (p95) | 1 2 3 4 5 6 7 8 | 25,03 ± 0 | 0 % | 4,4 | 7,3 | 16,54 % | 12,4 | 25,03 | 25,03 |
| baseline | Consulta de historial | 50 | original | 1 2 3 4 5 | 50 ± 0 | 0 % | 1 | 2 | 0 % | 3,8 | 50 | 50 |
| fabric | Consulta de historial | 50 | original | 1 2 3 4 5 | 50,03 ± 0 | 0 % | 4 | 6,8 | 11 % | 11 | 50,03 | 50,03 |
| baseline | Consulta de unidad | 10 | original | 1 2 3 4 5 | 10 ± 0 | 0 % | 2 | 3 | 0 % | 6 | 10 | 10 |
| fabric | Consulta de unidad | 10 | original | 1 2 3 4 5 | 10,03 ± 0 | 0 % | 4,4 | 8,6 | 5,7 % | 12,6 | 10,03 | 10,03 |
| baseline | Consulta de unidad | 25 | original | 1 2 3 4 5 | 25 ± 0 | 0 % | 1 | 2 | 0 % | 4 | 25 | 25 |
| fabric | Consulta de unidad | 25 | original | 1 2 3 4 5 | 25,03 ± 0 | 0 % | 3,4 | 5,2 | 7,69 % | 8,8 | 25,03 | 25,03 |
| baseline | Consulta de unidad | 50 | original | 1 2 3 4 5 | 50 ± 0 | 0 % | 1 | 2 | 0 % | 3,8 | 50 | 50 |
| baseline | Consulta de unidad | 50 | extendida (mantener pares con el otro SUT) | 1 2 3 4 5 6 7 8 | 50 ± 0 | 0 % | 1 | 2 | 0 % | 3,9 | 50 | 50 |
| fabric | Consulta de unidad | 50 | original | 1 2 3 4 5 | 50,03 ± 0 | 0,01 % | 3,4 | 5,8 | 20,11 % | 8,8 | 50,03 | 50,03 |
| fabric | Consulta de unidad | 50 | extendida (p95) | 1 2 3 4 5 6 7 8 | 50,03 ± 0 | 0,01 % | 3,3 | 5,5 | 18,18 % | 8,9 | 50,03 | 50,03 |
| baseline | Dispensación | 5 | original | 1 2 3 4 5 | 5 ± 0 | 0 % | 5 | 6,4 | 7,65 % | 9,2 | 5 | 5 |
| fabric | Dispensación | 5 | original | 1 2 3 4 5 | 4,99 ± 0,03 | 0,64 % | 1.260,8 | 2.079,4 | 0,71 % | 2.115,2 | 4,99 | 4,99 |
| baseline | Dispensación | 10 | original | 1 2 3 4 5 | 10 ± 0 | 0 % | 4 | 6,6 | 12,12 % | 13,4 | 10 | 10 |
| fabric | Dispensación | 10 | original | 1 2 3 4 5 | 9,94 ± 0,04 | 0,4 % | 1.119,8 | 2.100,6 | 1,01 % | 2.248,4 | 9,94 | 9,94 |
| baseline | Dispensación | 20 | original | 1 2 3 4 5 | 20 ± 0 | 0 % | 4 | 5,8 | 6,9 % | 14,2 | 20 | 20 |
| fabric | Dispensación | 20 | original | 1 2 3 4 5 | 19,85 ± 0,06 | 0,33 % | 1.208,4 | 2.141 | 2,55 % | 2.505 | 19,85 | 19,85 |
| baseline | Registro | 5 | original | 1 2 3 4 5 | 5 ± 0 | 0 % | 4,8 | 8 | 11,18 % | 16,8 | 5 | 5 |
| fabric | Registro | 5 | original | 1 2 3 4 5 | 4,94 ± 0,01 | 0,24 % | 1.284,8 | 2.108,2 | 0,65 % | 2.130,8 | 4,94 | 4,94 |
| baseline | Registro | 10 | original | 1 2 3 4 5 | 10 ± 0 | 0 % | 4,8 | 7,4 | 18,33 % | 13,8 | 10 | 10 |
| baseline | Registro | 10 | extendida (p95) | 1 2 3 4 5 6 7 8 | 10 ± 0 | 0 % | 4,5 | 7,1 | 16,36 % | 13,5 | 10 | 10 |
| fabric | Registro | 10 | original | 1 2 3 4 5 | 9,98 ± 0,04 | 0,39 % | 1.119,6 | 2.096,6 | 0,78 % | 2.152,4 | 9,98 | 9,98 |
| fabric | Registro | 10 | extendida (mantener pares con el otro SUT) | 1 2 3 4 5 6 7 8 | 9,97 ± 0,05 | 0,51 % | 1.119,6 | 2.098,1 | 0,86 % | 2.208,8 | 9,97 | 9,97 |
| baseline | Registro | 20 | original | 1 2 3 4 5 | 20 ± 0 | 0 % | 4,4 | 6,6 | 18,18 % | 12,6 | 20 | 20 |
| baseline | Registro | 20 | extendida (p95) | 1 2 3 4 5 6 7 8 | 20 ± 0 | 0 % | 4,3 | 6,5 | 15,38 % | 13,6 | 20 | 20 |
| fabric | Registro | 20 | original | 1 2 3 4 5 | 19,78 ± 0,07 | 0,34 % | 1.213,4 | 2.149,2 | 1,59 % | 2.245 | 19,78 | 19,78 |
| fabric | Registro | 20 | extendida (mantener pares con el otro SUT) | 1 2 3 4 5 6 7 8 | 19,81 ± 0,08 | 0,42 % | 1.238,3 | 2.254,3 | 11,38 % | 2.685,5 | 19,81 | 19,81 |
| baseline | Transferencia (pares/s) | 5 | original | 1 2 3 4 5 | 5 ± 0 | 0 % | 10 | 15,4 | 6,62 % | 25,6 | 10 | 10 |
| fabric | Transferencia (pares/s) | 5 | original | 1 2 3 4 5 | 4,88 ± 0,02 | 0,49 % | 3.378,2 | 4.378,2 | 0,66 % | 4.528,2 | 9,76 | 10,14 |
| baseline | Transferencia (pares/s) | 10 | original | 1 2 3 4 5 | 10 ± 0 | 0 % | 9,6 | 14 | 11,07 % | 21,2 | 20 | 20 |
| fabric | Transferencia (pares/s) | 10 | original | 1 2 3 4 5 | 9,72 ± 0,05 | 0,5 % | 3.557 | 4.555,4 | 0,36 % | 4.714,2 | 19,44 | 21,18 |
| baseline | Transferencia (pares/s) | 20 | original | 1 2 3 4 5 | 20 ± 0 | 0 % | 8,4 | 12,4 | 6,45 % | 20,4 | 40 | 40 |
| fabric | Transferencia (pares/s) | 20 | original | 1 2 3 4 5 | 19,46 ± 0,05 | 0,25 % | 3.708,4 | 4.767,6 | 2,74 % | 5.218,8 | 38,91 | 39,5 |

La serie original son las repeticiones 1 a 5; la extendida, las 1 a 8 cuando el CV de throughput o p95 de la original superó 15 % (sección 7). En transferencia el throughput está en pares/s. `tx confirmadas/s` cuenta las transacciones confirmadas de ambos pasos; `intentos tx/s` cuenta todos los intentos enviados al SUT, incluidos los reintentos transitorios de la sección 3.4, que son parte del costo observado.

### Transferencia: pasos del par

| SUT | Escenario | Tasa | p95 despacho (ms) | p95 recepción (ms) | p95 par (ms) | Pares con reintento |
|---|---|---|---|---|---|---|
| baseline | Carga mixta | 20 | 6,8 | 5,4 | 12,2 | 0 % |
| fabric | Carga mixta | 20 | 2.330 | 2.290,2 | 4.436 | 2,71 % |
| baseline | Transferencia (pares/s) | 5 | 9,2 | 7,4 | 15,4 | 0 % |
| fabric | Transferencia (pares/s) | 5 | 2.184,4 | 2.279,2 | 4.378,2 | 7,84 % |
| baseline | Transferencia (pares/s) | 10 | 8,4 | 6,4 | 14 | 0 % |
| fabric | Transferencia (pares/s) | 10 | 2.346,4 | 2.351,4 | 4.555,4 | 18 % |
| baseline | Transferencia (pares/s) | 20 | 7 | 6,2 | 12,4 | 0 % |
| fabric | Transferencia (pares/s) | 20 | 2.579,6 | 2.411,6 | 4.767,6 | 3,05 % |

## Comparación Fabric frente a baseline

Sólo sobre repeticiones emparejadas por número; las faltantes de cada lado se listan y no se completan.

| Escenario | Tasa | Serie | Emparejadas | Faltan en Fabric | Faltan en baseline | Throughput Fabric | Throughput baseline | Fabric/baseline | p95 Fabric (ms) | p95 baseline (ms) | p95 Fabric/baseline |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Carga mixta | 20 | original | 1 2 3 4 5 | — | — | 19,46 | 20 | 0,97 | 4.336 | 11,2 | 387,14 |
| Consulta de historial | 10 | original | 1 2 3 4 5 | — | — | 10,03 | 10 | 1 | 9,2 | 3 | 3,07 |
| Consulta de historial | 25 | original | 1 2 3 4 5 | — | — | 25,03 | 25 | 1 | 7,2 | 2,4 | 3 |
| Consulta de historial | 25 | extendida | 1 2 3 4 5 6 7 8 | — | — | 25,03 | 25 | 1 | 7,3 | 2,3 | 3,22 |
| Consulta de historial | 50 | original | 1 2 3 4 5 | — | — | 50,03 | 50 | 1 | 6,8 | 2 | 3,4 |
| Consulta de unidad | 10 | original | 1 2 3 4 5 | — | — | 10,03 | 10 | 1 | 8,6 | 3 | 2,87 |
| Consulta de unidad | 25 | original | 1 2 3 4 5 | — | — | 25,03 | 25 | 1 | 5,2 | 2 | 2,6 |
| Consulta de unidad | 50 | original | 1 2 3 4 5 | — | — | 50,03 | 50 | 1 | 5,8 | 2 | 2,9 |
| Consulta de unidad | 50 | extendida | 1 2 3 4 5 6 7 8 | — | — | 50,03 | 50 | 1 | 5,5 | 2 | 2,75 |
| Dispensación | 5 | original | 1 2 3 4 5 | — | — | 4,99 | 5 | 1 | 2.079,4 | 6,4 | 324,91 |
| Dispensación | 10 | original | 1 2 3 4 5 | — | — | 9,94 | 10 | 0,99 | 2.100,6 | 6,6 | 318,27 |
| Dispensación | 20 | original | 1 2 3 4 5 | — | — | 19,85 | 20 | 0,99 | 2.141 | 5,8 | 369,14 |
| Registro | 5 | original | 1 2 3 4 5 | — | — | 4,94 | 5 | 0,99 | 2.108,2 | 8 | 263,53 |
| Registro | 10 | original | 1 2 3 4 5 | — | — | 9,98 | 10 | 1 | 2.096,6 | 7,4 | 283,32 |
| Registro | 10 | extendida | 1 2 3 4 5 6 7 8 | — | — | 9,97 | 10 | 1 | 2.098,1 | 7,1 | 294,47 |
| Registro | 20 | original | 1 2 3 4 5 | — | — | 19,78 | 20 | 0,99 | 2.149,2 | 6,6 | 325,64 |
| Registro | 20 | extendida | 1 2 3 4 5 6 7 8 | — | — | 19,81 | 20 | 0,99 | 2.254,3 | 6,5 | 346,81 |
| Transferencia (pares/s) | 5 | original | 1 2 3 4 5 | — | — | 4,88 | 5 | 0,98 | 4.378,2 | 15,4 | 284,3 |
| Transferencia (pares/s) | 10 | original | 1 2 3 4 5 | — | — | 9,72 | 10 | 0,97 | 4.555,4 | 14 | 325,39 |
| Transferencia (pares/s) | 20 | original | 1 2 3 4 5 | — | — | 19,46 | 20 | 0,97 | 4.767,6 | 12,4 | 384,48 |

Todas las combinaciones tienen contraparte comparable.

## Rechazos esperados

| SUT | Variante | n | Rechazos/s | p95 (ms) | Códigos observados |
|---|---|---|---|---|---|
| baseline | BLOCKING_STATE/dispense | 5 | 5 | 3,4 | INVALID_STATE_TRANSITION 1500 |
| fabric | BLOCKING_STATE/dispense | 5 | 5,03 | 7 | INVALID_STATE_TRANSITION 1510 |
| baseline | BLOCKING_STATE/transfer | 5 | 5 | 3,8 | INVALID_STATE_TRANSITION 1500 |
| fabric | BLOCKING_STATE/transfer | 5 | 5,03 | 8,6 | INVALID_STATE_TRANSITION 1510 |
| baseline | DUPLICATE_IDENTITY/register | 5 | 5 | 4 | UNIT_ALREADY_EXISTS 1500 |
| fabric | DUPLICATE_IDENTITY/register | 5 | 5,03 | 7 | UNIT_ALREADY_EXISTS 1510 |
| baseline | UNAUTHORIZED_TRANSFER/transfer | 5 | 5 | 3,8 | TRANSFER_NOT_AUTHORIZED 1500 |
| fabric | UNAUTHORIZED_TRANSFER/transfer | 5 | 5,03 | 9,6 | TRANSFER_NOT_AUTHORIZED 1510 |

Los rechazos se miden en rondas propias y no se suman al throughput exitoso (sección 6.5).

## Marcadores de participación

| SUT | Escenario | Tasa | Rondas que los declaran | Esperados (media) | Observados (media) | Altas | Eventos regulatorios | Por segundo | Proporción de escrituras |
|---|---|---|---|---|---|---|---|---|---|
| fabric | expected-rejections BLOCKING_STATE/dispense | 5 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | expected-rejections BLOCKING_STATE/transfer | 5 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | expected-rejections DUPLICATE_IDENTITY/register | 5 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | expected-rejections UNAUTHORIZED_TRANSFER/transfer | 5 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Carga mixta | 20 | 5/5 | 240 | 240 | 240 | 0 | 2 | 7,69 % |
| fabric | Consulta de historial | 10 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Consulta de historial | 25 | 8/8 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Consulta de historial | 50 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Consulta de unidad | 10 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Consulta de unidad | 25 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Consulta de unidad | 50 | 8/8 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Dispensación | 5 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Dispensación | 10 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Dispensación | 20 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Registro | 5 | 5/5 | 602 | 602 | 602 | 0 | 5,02 | 100 % |
| fabric | Registro | 10 | 8/8 | 1.202 | 1.202 | 1.202 | 0 | 10,02 | 100 % |
| fabric | Registro | 20 | 8/8 | 2.402 | 2.402 | 2.402 | 0 | 20,02 | 100 % |
| fabric | Transferencia (pares/s) | 5 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Transferencia (pares/s) | 10 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |
| fabric | Transferencia (pares/s) | 20 | 5/5 | 0 | 0 | 0 | 0 | 0 | 0 % |

La sección 3.5 pide atribuir el costo de los marcadores por escenario. Una ronda que no declara `participationMarkers` en su metadata queda como «no informado»: su costo está dentro de la latencia y el throughput, pero no puede atribuirse. Una diferencia entre esperados y observados invalida el grupo.


## Errores inesperados y timeouts

_Sin filas._

Incluye las rondas excluidas: son justamente las que contienen los fallos.

## Rondas excluidas de las estadísticas

| SUT | Escenario | Fase | Repetición | Motivo | Directorio |
|---|---|---|---|---|---|
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-dispense-5/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-dispense-5/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-dispense-5/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-dispense-5/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-dispense-5/measurements/run-05/smoke` |
| fabric | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-dispense-5/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-dispense-5/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-transfer-5/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-transfer-5/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-transfer-5/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-transfer-5/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-transfer-5/measurements/run-05/smoke` |
| fabric | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-transfer-5/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-blocking_state-transfer-5/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-duplicate_identity-5/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-duplicate_identity-5/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-duplicate_identity-5/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-duplicate_identity-5/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-duplicate_identity-5/measurements/run-05/smoke` |
| fabric | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-duplicate_identity-5/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-duplicate_identity-5/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-unauthorized_transfer-5/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-unauthorized_transfer-5/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-unauthorized_transfer-5/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-unauthorized_transfer-5/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-unauthorized_transfer-5/measurements/run-05/smoke` |
| fabric | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-unauthorized_transfer-5/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/expected-rejections-unauthorized_transfer-5/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/mixed-20/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/mixed-20/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/mixed-20/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/mixed-20/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/mixed-20/measurements/run-05/smoke` |
| fabric | mixed | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/mixed-20/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/mixed-20/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-10/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-10/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-10/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-10/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-10/measurements/run-05/smoke` |
| fabric | read-history | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-10/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-10/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-05/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-06/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-07/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/measurements/run-08/smoke` |
| fabric | read-history | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-25/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-50/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-50/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-50/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-50/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-50/measurements/run-05/smoke` |
| fabric | read-history | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-50/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-history-50/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-10/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-10/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-10/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-10/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-10/measurements/run-05/smoke` |
| fabric | read-unit | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-10/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-10/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-25/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-25/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-25/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-25/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-25/measurements/run-05/smoke` |
| fabric | read-unit | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-25/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-25/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-05/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-06/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-07/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/measurements/run-08/smoke` |
| fabric | read-unit | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/read-unit-50/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-10/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-10/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-10/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-10/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-10/measurements/run-05/smoke` |
| fabric | write-dispense | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-10/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-10/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-20/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-20/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-20/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-20/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-20/measurements/run-05/smoke` |
| fabric | write-dispense | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-20/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-20/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-5/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-5/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-5/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-5/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-5/measurements/run-05/smoke` |
| fabric | write-dispense | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-5/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-dispense-5/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-05/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-06/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-07/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/measurements/run-08/smoke` |
| fabric | write-register | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-10/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-05/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-06/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-07/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/measurements/run-08/smoke` |
| fabric | write-register | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-20/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-5/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-5/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-5/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-5/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-5/measurements/run-05/smoke` |
| fabric | write-register | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-5/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-register-5/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-10/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-10/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-10/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-10/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-10/measurements/run-05/smoke` |
| fabric | write-transfer | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-10/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-10/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-20/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-20/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-20/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-20/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-20/measurements/run-05/smoke` |
| fabric | write-transfer | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-20/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-20/warmup/try-1/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-5/measurements/run-01/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-5/measurements/run-02/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-5/measurements/run-03/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-5/measurements/run-04/smoke` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-5/measurements/run-05/smoke` |
| fabric | write-transfer | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-5/warmup/try-1/round` |
| fabric | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/fabric/write-transfer-5/warmup/try-1/smoke` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-dispense-5/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-dispense-5/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-dispense-5/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-dispense-5/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-dispense-5/measurements/run-05/round/preflight` |
| baseline | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-dispense-5/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-dispense-5/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-transfer-5/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-transfer-5/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-transfer-5/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-transfer-5/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-transfer-5/measurements/run-05/round/preflight` |
| baseline | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-transfer-5/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-blocking_state-transfer-5/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-duplicate_identity-5/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-duplicate_identity-5/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-duplicate_identity-5/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-duplicate_identity-5/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-duplicate_identity-5/measurements/run-05/round/preflight` |
| baseline | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-duplicate_identity-5/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-duplicate_identity-5/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-unauthorized_transfer-5/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-unauthorized_transfer-5/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-unauthorized_transfer-5/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-unauthorized_transfer-5/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-unauthorized_transfer-5/measurements/run-05/round/preflight` |
| baseline | expected-rejections | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-unauthorized_transfer-5/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/expected-rejections-unauthorized_transfer-5/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/mixed-20/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/mixed-20/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/mixed-20/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/mixed-20/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/mixed-20/measurements/run-05/round/preflight` |
| baseline | mixed | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/mixed-20/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/mixed-20/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-10/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-10/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-10/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-10/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-10/measurements/run-05/round/preflight` |
| baseline | read-history | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-10/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-10/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-05/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-06/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-07/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/measurements/run-08/round/preflight` |
| baseline | read-history | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-25/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-50/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-50/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-50/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-50/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-50/measurements/run-05/round/preflight` |
| baseline | read-history | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-50/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-history-50/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-10/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-10/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-10/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-10/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-10/measurements/run-05/round/preflight` |
| baseline | read-unit | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-10/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-10/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-25/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-25/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-25/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-25/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-25/measurements/run-05/round/preflight` |
| baseline | read-unit | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-25/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-25/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-05/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-06/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-07/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/measurements/run-08/round/preflight` |
| baseline | read-unit | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/read-unit-50/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-10/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-10/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-10/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-10/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-10/measurements/run-05/round/preflight` |
| baseline | write-dispense | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-10/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-10/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-20/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-20/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-20/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-20/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-20/measurements/run-05/round/preflight` |
| baseline | write-dispense | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-20/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-20/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-5/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-5/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-5/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-5/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-5/measurements/run-05/round/preflight` |
| baseline | write-dispense | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-5/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-dispense-5/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-05/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-06/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-07/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/measurements/run-08/round/preflight` |
| baseline | write-register | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-10/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-05/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-06/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-07/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/measurements/run-08/round/preflight` |
| baseline | write-register | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-20/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-5/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-5/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-5/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-5/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-5/measurements/run-05/round/preflight` |
| baseline | write-register | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-5/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-register-5/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-10/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-10/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-10/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-10/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-10/measurements/run-05/round/preflight` |
| baseline | write-transfer | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-10/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-10/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-20/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-20/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-20/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-20/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-20/measurements/run-05/round/preflight` |
| baseline | write-transfer | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-20/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-20/warmup/try-1/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-5/measurements/run-01/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-5/measurements/run-02/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-5/measurements/run-03/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-5/measurements/run-04/round/preflight` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-5/measurements/run-05/round/preflight` |
| baseline | write-transfer | warmup | 0 | fase warmup | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-5/warmup/try-1/round` |
| baseline | smoke | measurement | 1 | escenario smoke | `/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/eval67-389ca16-20261021e-full/baseline/write-transfer-5/warmup/try-1/round/preflight` |

Se conservan sus crudos; no se imputan ni se reemplazan repeticiones.

## Asimetrías estructurales declaradas

| Aspecto | Diferencia | Efecto sobre lo medido |
|---|---|---|
| Identidad | La baseline emula organizaciones con API keys; Fabric usa MSP y PKI. | Dentro de lo medido: es parte del costo comparado. |
| Confirmación | La baseline confirma en una transacción PostgreSQL; Fabric endosa, ordena con Raft y valida en siete peers. | Dentro de lo medido: es la diferencia que se evalúa. |
| Endoso, PDC y marcadores | Solo Fabric paga políticas de endoso, colecciones privadas y marcadores de participación (sección 3.5). | Dentro de lo medido. |
| Visibilidad entre peers | Solo Fabric reintenta la recepción por datos privados no diseminados o estado público atrasado (sección 3.4). | Dentro de lo medido: queda en la latencia del par y en los reintentos. |
| Reloj | Los timestamps de la baseline vienen del servidor de la API; los de Fabric, de la propuesta. | No altera las métricas de cliente. |
| Historial de intervención | La baseline paga un INSERT en `lab_intervention_events` por cada escritura de autorización. | Fuera de lo medido: esas operaciones no aparecen en las secciones 6.2 ni 6.3. |
| Inmutabilidad | El historial de la baseline es append-only por convención; un administrador de base puede alterarlo. | Fuera de lo medido: es una propiedad cualitativa. |

## Alcance de la interpretación

- Los valores son indicativos del comportamiento **relativo** de ambos diseños en un host único y no se extrapolan a producción.
- Este reporte contiene resultados cuantitativos medidos. Las propiedades cualitativas (integridad, auditabilidad) y su interpretación pertenecen al análisis de EVAL-8 y al capítulo de resultados.
- Ninguna dimensión aislada permite concluir la superioridad de una arquitectura (sección 10).

## Campaña alternada verificada

Commit experimental: 389ca160b2a490cd66bb0194f012e2b75b3598e8. Orden y repeticiones pareadas validados.

- write-register-10, fabric: mantener pares con el otro SUT; activadores: baseline/p95 CV=0.18330621576014236
- write-register-10, baseline: variabilidad propia; activadores: baseline/p95 CV=0.18330621576014236
- write-register-20, fabric: mantener pares con el otro SUT; activadores: baseline/p95 CV=0.18181818181818182
- write-register-20, baseline: variabilidad propia; activadores: baseline/p95 CV=0.18181818181818182
- read-unit-50, fabric: variabilidad propia; activadores: fabric/p95 CV=0.20106730671880346
- read-unit-50, baseline: mantener pares con el otro SUT; activadores: fabric/p95 CV=0.20106730671880346
- read-history-25, fabric: variabilidad propia; activadores: fabric/p95 CV=0.161970885967925, baseline/p95 CV=0.2041241452319315
- read-history-25, baseline: variabilidad propia; activadores: fabric/p95 CV=0.161970885967925, baseline/p95 CV=0.2041241452319315

Mismo host, sin presupuesto agregado equivalente demostrado; se conservan las cuotas originales y los recursos efectivos de cada SUT.
