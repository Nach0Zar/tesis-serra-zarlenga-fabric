# Procesamiento comparativo

Este módulo procesa los crudos de Fabric y de la baseline y produce las tablas,
figuras y el reporte de la sección 10 de
[`docs/measurement-protocol.md`](../../docs/measurement-protocol.md) (EVAL-7,
#47). No tiene dependencias: requiere Node.js 22 y, para validar la metadata,
Go según `client/go.mod`.

## Uso

```bash
cd benchmarks/analysis
npm test
npm run process -- \
  --input ../../build/benchmarks/caliper \
  --input ../../build/benchmarks/baseline/runs \
  --output ../../build/analysis/<corrida>
```

`--input` se puede repetir y se recorre en profundidad: cada directorio con un
`metadata.json` es una ronda, y se leen su `summary.json` y su `raw.json`. Antes
de procesar, todos los `metadata.json` se validan contra DES-20 con
`client/cmd/runmeta`; un documento inválido detiene el procesamiento.
`--no-validate` omite ese paso y sirve sólo para desarrollo.

## Qué entra en las estadísticas

Entran las rondas `measurement` de los escenarios de las secciones 6.2 a 6.5
que no están descartadas. Quedan fuera, con su motivo en `excluded.csv` y en el
reporte:

- warm-ups, smoke y la preparación del snapshot;
- rondas con `discarded.reason`;
- rondas de carga mixta cuyo `raw.json` no sigue, worker por worker, el ciclo
  exacto 2/11/2/5 de la sección 6.4;
- escenarios de disponibilidad, que se procesan por ventanas y no se mezclan
  con el rendimiento estable (#44).

No se imputan ni se reemplazan repeticiones faltantes.

## Agregación y comparación

Cada grupo reúne las repeticiones de un mismo SUT, escenario, variante de
rechazo y tasa objetivo; nunca se promedia entre workloads. Por grupo se
informan la media y el desvío **poblacional** entre repeticiones, el
coeficiente de variación de throughput y p95, y si la serie se extendió a ocho
repeticiones y por cuál de las dos métricas.

- **Throughput**: operaciones conceptuales exitosas por segundo de la ventana
  observada. En transferencia son pares/s; las transacciones de ambos pasos se
  informan aparte (`tx/s`).
- **Rechazos esperados**: se cuentan como rechazos por segundo, en su propia
  tabla, con los códigos observados. No se suman al throughput exitoso.
- **Transferencia**: p95 del despacho, de la recepción y del par completo. En la
  carga mixta el p95 del par se calcula sólo sobre las transferencias.
- **Latencias**: percentiles nearest-rank de cada ronda, promediados entre
  repeticiones.

Un grupo de Fabric se compara con uno de baseline sólo si cumplen la paridad de
la sección 9: mismo dataset, workers, duración, operación, transacciones por
operación y mezcla, y sin inconsistencias internas (distinto dataset o
artefacto entre repeticiones, repeticiones duplicadas). Las combinaciones que no
la cumplen, o a las que les falta un SUT, se listan con el motivo en lugar de
compararse.

## Disponibilidad

Para las rondas con `faultInjection` se calculan la tasa de éxito, el
throughput, el p95 y los errores por código en las ventanas previa, de falla y
de recuperación, y el tiempo de recuperación de la sección 3.3: el comienzo del
primer tramo de 30 s cuya tasa de éxito vuelve al 95 % de la previa a la falla.

## Salidas

| Archivo | Contenido |
|---|---|
| `repetitions.csv` | Una fila por repetición incluida. |
| `aggregate.csv` | Una fila por SUT, escenario, variante y tasa, con media, desvío y CV. |
| `comparison.csv` | Fabric frente a baseline, con cocientes de throughput y percentiles y el artefacto de cada lado. |
| `excluded.csv` | Rondas fuera de las estadísticas y su motivo. |
| `unexpected.csv` | Errores inesperados y timeouts por repetición. |
| `availability.json` | Ventanas y tiempo de recuperación de los escenarios de falla. |
| `figures/throughput.svg` | Throughput efectivo por tasa, media ± desvío, con la tasa objetivo como referencia. |
| `figures/latency.svg` | p95 con rango p50–p99 por tasa, en escala logarítmica. |
| `figures/availability-*.svg` | Tasa de éxito por intervalo alrededor de la falla. |
| `report.md` | Artefactos medidos, tablas, rondas excluidas, asimetrías estructurales declaradas y alcance de la interpretación. |

Las figuras son SVG estáticos para el documento del PFI. Usan la paleta
categórica validada para daltonismo y distinguen además Fabric y baseline por
forma de marcador y trazo, de modo que se leen también en escala de grises.

## Pendiente

Cuando se integre #141, el procesamiento debe leer además el índice de serie
de Fabric y su equivalente de la baseline para excluir los ensayos `trial` y
emparejar por plan lógico y snapshot, como pide #47.
