# Procesamiento comparativo

Este módulo procesa los crudos de Fabric y de la baseline y produce las tablas,
figuras y el reporte de la sección 10 de
[`docs/measurement-protocol.md`](../../docs/measurement-protocol.md) (EVAL-7,
#47). Requiere Node.js 22, la única dependencia fijada en el lockfile (AJV
8.17.1, para validar el manifiesto del snapshot y el índice de serie contra los
schemas de `benchmarks/schema/`) y, para validar la metadata, Go según
`client/go.mod`.

## Uso

```bash
cd benchmarks/analysis
npm ci
npm test
npm run process -- \
  --input ../../build/benchmarks/fabric/series/<serie> \
  --input ../../build/benchmarks/baseline/<serie> \
  --snapshot ../../build/benchmarks/fabric/snapshots/<snapshot-id> \
  --snapshot ../../build/benchmarks/baseline/snapshots/<token> \
  --output ../../build/analysis/<corrida>
```

`--input` se puede repetir y se recorre en profundidad: cada directorio con un
`metadata.json` es una ronda, y se leen su `summary.json`, su `raw.json` y su
`snapshot-context.json`. Antes de procesar, todos los `metadata.json` se validan
contra DES-20 con `client/cmd/runmeta`; un documento inválido detiene el
procesamiento. `--no-validate` omite ese paso y sirve sólo para desarrollo.

`--snapshot` se repite con el directorio de cada snapshot usado (el que contiene
`manifest.json`). Un manifiesto con conteos que no son los canónicos detiene la
acreditación de las rondas que lo usan.

## Acreditación de cada medición

Una ronda medida, de rendimiento o de disponibilidad, sólo se usa si se cumplen
dos condiciones:

1. **Estado inicial acreditado.** Se exige, para cada SUT:
   - un `snapshot-context.json` con el id del snapshot y el SHA-256 de su
     `manifest.json`, y `verifiedPreconditions` que cubra todas las unidades
     que la ronda tocó;
   - el manifiesto provisto con `--snapshot`, con ese mismo SHA-256, conteos
     canónicos (50.000 / 46.920 / 3.080 / 22.103 / 24.817 / 25.183) y el mismo
     dataset. El de Fabric debe cumplir `fabric-snapshot-manifest.schema.json`
     de #141; el de la baseline, declarar `logicalPlanSHA256` igual a
     `files.logicalPlan` y un `baselineImage` con digest inmutable;
   - los archivos que acreditan el estado (plan lógico, precondiciones y, en
     Fabric, la verificación semántica; en la baseline, el dump) presentes
     junto al manifiesto y con el hash que éste declara.
2. **Serie citable.** La ronda figura en un `series-index.json` válido contra
   el schema de #141, como intento `measurement` `completed` con la misma
   repetición, y la serie declara el mismo snapshot y el mismo SHA-256 de
   manifiesto, con `citable: true` (`plan: full`).

Un `trial`, o una ronda sin índice de serie, queda fuera con motivo «no
citable». `--allow-noncitable` la admite para diagnóstico: entonces el reporte y
todas las figuras que la usan, incluidas las de disponibilidad, se rotulan
**NO CITABLE**, y la comparación exige que ambos lados tengan la misma
condición. La baseline todavía no tiene un orquestador de series que escriba su
índice, así que sus rondas sólo entran como no citables hasta que exista
(EVAL-7).

Los snapshots físicos de cada SUT difieren; lo que debe coincidir para
comparar es el **plan lógico**: Fabric y baseline se comparan sólo si ambos
grupos declaran el mismo `logical-plan.json`.

## Qué entra en las estadísticas

Además de la acreditación, quedan fuera, con su motivo en `excluded.csv` y en el
reporte:

- warm-ups, smoke y la preparación del snapshot;
- rondas con `discarded.reason`;
- rondas sin `summary.json` o sin `raw.json`: sin crudo no se pueden verificar
  la mezcla, los pares ni los errores;
- rondas de carga mixta cuyo `raw.json` no sigue, worker por worker, el ciclo
  exacto 2/11/2/5 de la sección 6.4;
- escenarios de disponibilidad, que se procesan por ventanas y no se mezclan
  con el rendimiento estable (#44).

No se imputan ni se reemplazan repeticiones faltantes.

## Agregación y comparación

Cada grupo reúne las repeticiones de un mismo SUT, escenario, variante de
rechazo, tasa objetivo **y condiciones de carga** (operación, workers, duración,
transacciones por operación y mezcla): dos repeticiones con distinta carga nunca
se promedian juntas. Por grupo se informan la media y el desvío
**poblacional**, y el coeficiente de variación de throughput y p95. Siempre se
reporta la serie original (repeticiones 1 a 5) y, si hubo extensión, también la
extendida (1 a 8) junto con la métrica que la disparó (sección 7).

- **Throughput**: operaciones conceptuales exitosas por segundo de la ventana
  observada. En transferencia son pares/s.
- **Tasa física**: dos tasas con denominadores distintos. `tx confirmadas/s`
  cuenta las transacciones confirmadas; `intentos tx/s` cuenta todos los
  intentos enviados al SUT, incluidos los reintentos transitorios de la sección
  3.4, que son parte del costo observado de Fabric.
- **Rechazos esperados**: se cuentan como rechazos por segundo, en su propia
  tabla, con los códigos observados. No se suman al throughput exitoso.
- **Transferencia**: p95 del despacho, de la recepción y del par completo. En la
  carga mixta el p95 del par se calcula sólo sobre las transferencias.
- **Latencias**: percentiles nearest-rank de cada ronda, promediados entre
  repeticiones.
- **Marcadores de participación** (sección 3.5): se conservan por repetición y
  por grupo cuando la metadata los declara; las rondas que no los declaran
  figuran como «no informado», y una diferencia entre esperados y observados
  invalida el grupo. La preparación del snapshot va a `preparation.csv`,
  separada del rendimiento.

Un grupo de Fabric se compara con uno de baseline sólo si cumplen la paridad de
la sección 9 (mismo dataset, workers, duración, operación, transacciones por
operación y mezcla, misma condición de citabilidad y sin inconsistencias
internas) y **sólo sobre las repeticiones emparejadas por número**. Las que
faltan de cada lado se listan y nunca se completan; sin repeticiones
emparejadas no hay comparación.

## Disponibilidad

Para las rondas con `faultInjection` se calculan la tasa de éxito, el
throughput, el p95 y los errores por código en las ventanas previa, de falla y
de recuperación, y el tiempo de recuperación de la sección 3.3: el comienzo del
primer tramo de 30 s cuya tasa de éxito vuelve al 95 % de la previa a la falla.

En las caídas de peer (`peer-1a` a `peer-1d`, sección 8.1) cada operación del
crudo debe declarar `availabilityClass`: `target` si requiere el peer caído o
`control` si no. Las ventanas y la figura se calculan por clase, además del
agregado global en la tabla, para que el 0 % de la clase afectada no quede
oculto en el promedio. Si alguna operación no trae clase, el resultado se marca
bloqueado y no se exporta ninguna figura. El workload de EVAL-4 (#44) debe
emitir ese campo.

## Salidas

| Archivo | Contenido |
|---|---|
| `repetitions.csv` | Una fila por repetición incluida. |
| `aggregate.csv` | Una fila por grupo y serie (original y, si existe, extendida), con media, desvío, CV y marcadores. |
| `comparison.csv` | Fabric frente a baseline por serie, con repeticiones emparejadas y faltantes, cocientes, artefactos y snapshots de cada lado. |
| `excluded.csv` | Rondas fuera de las estadísticas y su motivo. |
| `unexpected.csv` | Errores inesperados y timeouts de todas las rondas con crudo, incluidas las excluidas, con su estado. |
| `preparation.csv` | Preparación del snapshot y sus marcadores, separada del rendimiento. |
| `availability.json` | Ventanas y tiempo de recuperación de los escenarios de falla. |
| `figures/throughput.svg` | Throughput efectivo por tasa, media ± desvío, con la tasa objetivo como referencia. |
| `figures/latency.svg` | p95 con rango p50–p99 por tasa, en escala logarítmica. |
| `figures/availability-*.svg` | Tasa de éxito por intervalo alrededor de la falla. |
| `report.md` | Artefactos medidos, tablas, rondas excluidas, asimetrías estructurales declaradas y alcance de la interpretación. |

Las figuras son SVG estáticos para el documento del PFI. Usan la paleta
categórica validada para daltonismo y distinguen además Fabric y baseline por
forma de marcador y trazo, de modo que se leen también en escala de grises.
