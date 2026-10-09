# Workloads de Hyperledger Caliper

Este módulo fija Hyperledger Caliper `0.7.1` y el binding
`fabric:fabric-gateway`. El binding usa Fabric Gateway y es compatible con la
red Fabric `2.5.16` del repositorio. Caliper se ejecuta en modo
`--caliper-flow-only-test`: la creación del canal y el lifecycle del chaincode
siguen perteneciendo a los scripts de `network/`.

La ronda de smoke es deliberadamente mínima y diagnóstica:

- canal `snt-channel`, chaincode `snt` e identidad `User1` de `LabMSP`;
- una sola operación `ReadUnit`;
- 1 worker, controlador `fixed-rate`, 1 TPS y 30 consultas;
- una unidad tomada del primer `RegisterUnit` de `LabMSP` del dataset
  compartido, registrada fuera de la ronda sólo si aún no existe.

El runner medible agrega una ronda individual por operación core, la carga
mixta y los rechazos esperados. La carga completa del snapshot, la serie de
repeticiones y el procesamiento comparativo continúan fuera de este módulo.

## Requisitos

- Node.js 22 y npm 11.5.1 o posterior dentro de la serie 11;
- Go según `client/go.mod`;
- Docker Engine y Docker Compose;
- red local levantada, canal creado, chaincode desplegado y verificación de red
  exitosa;
- bundle determinístico de 50.000 unidades en `build/dataset`.

Desde la raíz del repositorio:

```bash
./network/network.sh up
./network/network.sh createChannel
./network/network.sh deployCC
./network/network.sh verify

cd client
go run ./cmd/datasetgen --units 50000 --output-dir ../build/dataset

cd ../benchmarks/caliper
npm ci
npm run check
npm test
npm run smoke
```

La documentación de Caliper 0.7.1 enumera Node.js 20 y 22, pero los paquetes
NPM publicados para esa versión declaran `node >=22` y `npm >=11.5.1`. Este
proyecto usa la intersección soportada y fija npm `11.6.2` mediante
`packageManager`.

No es necesario repetir `caliper bind`: el `package.json` y el lockfile ya
registran las dependencias exactas que produjo el binding
`fabric:fabric-gateway`. El script `npm run bind:fabric` se conserva únicamente
para actualizar conscientemente ese binding junto con el lockfile.

## Salidas

Cada ejecución crea un directorio nuevo en
`build/benchmarks/caliper/<timestamp>/` con:

| Archivo | Contenido |
|---|---|
| `network-config.json` | Configuración efectiva de `LabMSP`, peer, canal y chaincode; referencia las credenciales generadas sin copiarlas. |
| `benchmark-config.json` | Ronda exacta de 30 `ReadUnit` a 1 TPS. |
| `run-context.json` | Fuentes, versiones y host usados para construir el metadata. |
| `report.html` | Reporte Caliper con throughput y latencias. |
| `metadata.json` | Evidencia validada contra el contrato de metadata al cerrar la ronda. |

El runner valida el hash del dataset, lee la versión contractual desde
`docs/api-contract.md`, lee el `packageID` desde
`network/chaincode-package.lock` y los registra en `info`, en el contexto y en
`metadata.json`. Al terminar vuelve a validar el metadata con
`client/cmd/runmeta` y falla si no hubo exactamente 30 lecturas exitosas o si
el reporte no contiene throughput y latencias.

Las salidas, el dataset, certificados, claves y demás material generado están
ignorados por Git. El runner nunca imprime ni incorpora contenido PEM en sus
archivos; la configuración efectiva contiene solamente rutas locales.

Para elegir un identificador explícito y repetible del directorio de salida se
puede definir `SNT_CALIPER_RUN_TOKEN` con letras, números, punto, guion o guion
bajo. Para usar otro bundle generado, se puede definir
`SNT_CALIPER_DATASET_DIR` con su ruta absoluta o relativa a la raíz.

## Rondas medibles

El runner `npm run round` consume exclusivamente recetas del bundle compartido
con seed `20260727`. Con `--snapshot-dir` la ronda parte del snapshot inicial
(ver [Snapshot inicial](#snapshot-inicial)): no prepara nada y, antes de abrir la
ventana medida, verifica contra `preconditions.json` el estado previo de cada
unidad que va a usar; una sola unidad contaminada hace fallar la ronda. Sin
`--snapshot-dir` se conserva el modo de desarrollo: prepara fuera de la ventana
medida sólo los prefijos de receta requeridos por la ronda, sobre un ledger
casi vacío que no es comparable con la baseline. En ningún modo crea el canal,
instala chaincode ni reinicia la red.

La interfaz es:

```text
npm run round -- \
  --scenario <write-register|write-transfer|write-dispense|read-unit|read-history|mixed|expected-rejections> \
  --phase <warmup|measurement> \
  --repetition <0..8> \
  --rate <tasa permitida> \
  [--family <UNAUTHORIZED_TRANSFER|DUPLICATE_IDENTITY|BLOCKING_STATE>] \
  [--snapshot-dir <build/benchmarks/fabric-snapshots/<token>>] \
  [--operation <transfer|dispense>] \
  [--dataset-dir <ruta>] \
  [--run-token <identificador>]
```

`warmup` exige repetición `0`; `measurement`, una repetición entre `1` y `8`.
Las tasas admitidas son las fijadas por el protocolo de medición:

| Escenario | Workers | Operaciones/s | Duración |
|---|---:|---:|---:|
| `write-register`, `write-transfer`, `write-dispense` | 2 | 5, 10 o 20 | 120 s |
| `read-unit`, `read-history` | 4 | 10, 25 o 50 | 120 s |
| `mixed` | 2 | 20 | 120 s |
| `expected-rejections` | 2 | 5 | 60 s |

La transferencia se agenda en pares/s: cada operación conceptual ejecuta
`DispatchTransfer` y, apenas se confirma su commit, envía `ReceiveTransfer` sin
sondeo ni espera de sincronización previa. Registra ambas transacciones, cada
intento fallido y la latencia end-to-end completa. El controlador Caliper se
configura con un módulo `fixed-rate` local que cuenta llamadas al workload a la
tasa conceptual. Los eventos de cada intento siguen alimentando las métricas de
Caliper, pero los reintentos no retrasan la oferta del siguiente par. La tasa
nominal de transacciones permanece derivada de cada operación (dos por par; el
factor de la mezcla se deriva de sus porcentajes).

La recepción correlacionada se reintenta cuando el peer receptor aún no ve los
datos privados (`INTERNAL_ERROR` reintentable con causa
`PRIVATE_DATA_NOT_DISSEMINATED`) o todavía observa el estado público anterior
(`NOT_IN_TRANSIT`). Este último código sólo es transitorio dentro del par,
inmediatamente después de un despacho confirmado; fuera de esa ruta conserva su
semántica contractual final. La preparación, que está fuera de la ventana
medida, sí puede esperar a que los peers observen el estado requerido. Cada
intento conserva la causa contractual y la espera posterior; el resumen las
desglosa mediante `retryByCause` y `retryTimeMs`. Si se agota el máximo de 60
intentos, el par se registra como `unexpected-failure`, la ronda se descarta y
los crudos permanecen disponibles. En la carga mixta, el ciclo determinístico
contiene 2 registros, 11 transferencias, 2 dispensaciones y 5 `ReadUnit` por
cada 20 operaciones.

Los rechazos usan las familias y códigos esperados del dataset. Se ejecutan en
rondas separadas: `UNAUTHORIZED_TRANSFER`, `DUPLICATE_IDENTITY` y
`BLOCKING_STATE`; esta última admite una ronda `transfer` y otra `dispense`.
Un rechazo con otro código, o un éxito inesperado, descarta la ronda.

Ejemplos:

```bash
npm run round -- --scenario write-transfer --phase warmup --repetition 0 --rate 5
npm run round -- --scenario mixed --phase measurement --repetition 1 --rate 20
npm run round -- --scenario expected-rejections --phase measurement --repetition 1 \
  --rate 5 --family BLOCKING_STATE --operation dispense
```

Además de `network-config.json`, `benchmark-config.json`, `report.html` y
`metadata.json`, el runner conserva:

| Archivo | Contenido |
|---|---|
| `work-plan.json` | Secuencia determinística, particionada por worker, y preparaciones fuera de medición. |
| `raw.json` | Una entrada por operación conceptual, con las transacciones, timestamps, latencias, reintentos y códigos de rechazo. |
| `summary.json` | Conteos, tasas y latencias por función en JSON procesable. |
| `raw-worker-*.jsonl` | Partes append-only producidas por cada proceso worker. |

Los percentiles del resumen usan nearest-rank sobre las observaciones de la
ronda. `offeredOperationsPerSecond` registra la carga conceptual enviada durante
la ventana nominal. Las tasas efectivas dividen los conteos por la ventana
observada entre la primera operación iniciada y la última finalizada, sin usar
una duración nominal menor; `summary.json` expone además los pares transferidos,
los pares que requirieron reintento, su proporción y el desglose de tiempo por
causa. Producir menos operaciones que el objetivo es un resultado de throughput,
no por sí solo una causa de descarte. El HTML de Caliper es complementario:
`raw.json`, `summary.json` y `metadata.json` son la evidencia procesable. El
runner conserva los crudos, marca
`discarded.reason` y termina con error ante fallos operativos, resultados
inesperados o pares incompletos. Los crudos y resúmenes no contienen transient data ni material
PEM; las configuraciones efectivas sólo referencian rutas locales.
`work-plan.json` sí contiene las recetas transient sintéticas provenientes del
dataset para que los workers puedan ejecutarlas, por lo que permanece bajo el
directorio `build/` ignorado por Git y no debe publicarse como resultado.

## Snapshot inicial

La serie citable no prepara estado por ronda: restaura antes de cada ronda un
snapshot del ledger con las 50.000 unidades del bundle en su estado previo
(sección 4 del protocolo y decisiones D1–D5 de #138). El plan del snapshot es la
unión determinística de las 20 combinaciones de la sección 6 y coincide byte a
byte con el de la baseline:

| Grupo | Unidades |
|---|---:|
| Ausentes, cuya alta es la operación medida | 3.080 |
| Presentes en el estado previo de su escenario | 22.103 |
| Presentes como relleno (incluye la unidad del smoke) | 24.817 |
| Total | 50.000 |

Construirlo cuesta 129.878 transacciones, una sola vez por artefacto, sobre una
red recién desplegada:

```bash
npm run snapshot:build -- --snapshot-token golden-local --concurrency 128
```

El constructor ejecuta las recetas con concurrencia configurable porque las
unidades son disjuntas y la construcción está fuera de toda medición. Es
reanudable: ante cualquier error consulta el historial de la unidad, que tiene
exactamente una escritura por paso confirmado, y sigue desde ahí; nunca reenvía
un paso sin saber si el anterior quedó. `progress.jsonl` registra las recetas
completas, de modo que una nueva invocación con el mismo token retoma donde
quedó. `BatchTimeout` y el resto de la configuración del SUT no se tocan.

Al terminar, y antes de guardar nada:

1. verifica las 50.000 unidades contra el estado derivado de la máquina de
   estados (no contra lo que devolvió el ledger) y deja `verification.json`;
2. recorre los bloques confirmados durante la construcción y exige que las
   transacciones válidas por función coincidan exactamente con las recetas;
3. cuenta los marcadores de participación como escrituras con hash en las
   colecciones implícitas y exige 46.924: 46.920 altas y 4 eventos iniciados
   por ANMAT (sección 3.5);
4. emite el `metadata.json` de `dataset-preparation` con
   `participationMarkers` (sección 9.6) y lo valida con `runmeta`;
5. detiene la red, guarda los diez volúmenes del ledger como archivos tar y
   escribe el manifiesto con sus SHA-256.

El manifiesto ata el snapshot al contrato, al `packageID`, a las versiones de
Fabric y Fabric CA, al hash del dataset, a los archivos de red que fijan canal,
políticas y colecciones, y a la huella de las identidades generadas en
`network/organizations`. La restauración se niega si cualquiera difiere.

```bash
npm run snapshot:restore -- --snapshot-dir build/benchmarks/fabric-snapshots/golden-local
```

La restauración verifica el SHA-256 de cada volumen, elimina los volúmenes y
los contenedores de chaincode, extrae los archivos con la red detenida, levanta
la red y evalúa una lectura contra cada uno de los siete peers para que ningún
contenedor de chaincode arranque en frío dentro de la ventana medida.

Con `SNT_CALIPER_SNAPSHOT_DIR` definido, `npm run smoke` es de solo lectura:
usa la unidad de relleno reservada por el manifiesto y falla si no existe, en
lugar de registrarla.

## Serie de Fabric

`npm run series` ejecuta cada escenario de la sección 6 con un warm-up y cinco
repeticiones, y extiende a ocho si el coeficiente de variación de throughput o
p95 supera 15 %. Con `--snapshot-dir`, antes de cada ronda restaura el snapshot,
corre `network.sh verify`, mide recursos y ejecuta el smoke de solo lectura; la
ronda verifica sus unidades antes de medir. El id del snapshot queda en
`series.json`, en cada entrada de `index.jsonl` y en las notas del
`metadata.json` de cada ronda.

```bash
npm run series -- --snapshot-dir build/benchmarks/fabric-snapshots/golden-local
```

Un warm-up que no valida en `--max-attempts` intentos marca el escenario como
`not-executed` con su motivo y no se miden sus repeticiones (sección 7).

