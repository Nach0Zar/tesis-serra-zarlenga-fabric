# Workloads de Hyperledger Caliper

Este módulo fija Hyperledger Caliper `0.7.1` y el binding
`fabric:fabric-gateway`. El binding usa Fabric Gateway y es compatible con la
red Fabric `2.5.16` del repositorio. Caliper se ejecuta en modo
`--caliper-flow-only-test`: la creación del canal y el lifecycle del chaincode
siguen perteneciendo a los scripts de `network/`.

La ronda de smoke es deliberadamente mínima, diagnóstica y de solo lectura:

- canal `snt-channel`, chaincode `snt` e identidad `User1` de `LabMSP`;
- una sola operación `ReadUnit`;
- 1 worker, controlador `fixed-rate`, 1 TPS y 30 consultas;
- una unidad de relleno reservada por el plan lógico, ya existente en el
  snapshot y fuera de las bandas de todos los escenarios.

El runner medible agrega una ronda individual por operación core, la carga
mixta y los rechazos esperados. Este módulo también construye, verifica y
restaura el snapshot golden de Fabric y orquesta las series completa y de
ensayo. El procesamiento comparativo continúa fuera de este módulo.

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

## Snapshot golden

El plan canónico contiene 50.000 secuencias: 46.920 registradas —22.103
preparadas para escenarios y 24.817 de relleno— y 3.080 ausentes porque su alta
se mide. `logical-plan.json` es independiente del SUT y es el contrato que debe
consumir la baseline. La construcción ejecuta pasos seriales por receta con
concurrencia entre unidades, 64 por defecto; registra un journal durable y
reconcilia un timeout ambiguo con `ReadUnit` e historial antes de decidir si
continúa o reintenta.

Un `ABORTED` sólo es transitorio en la fase de endoso y si todos sus detalles
son timeouts o errores de transporte reconocidos, o el error específico de
simulación de Fabric `Failed disseminating N out of M private dissemination plans`
ligado al transaction ID de la propuesta actual. Este último permite recuperación
acotada antes del submit; no incluye errores de configuración de colecciones o ACL.
Fabric agrega los fallos de envío sin exponer sus causas al cliente
([distribuidor 2.5.16](https://github.com/hyperledger/fabric/blob/v2.5.16/gossip/privdata/distributor.go),
[endorser](https://github.com/hyperledger/fabric/blob/v2.5.16/core/endorser/endorser.go));
por eso el journal lo identifica como `disseminationFailure`, **no** como timeout
demostrado. La causa se comprueba en los logs del peer. Códigos sin detalles,
errores contractuales, respuestas incompatibles y detalles mixtos abortan.
Se comprueban estado e historial contra el prefijo durable antes del reintento,
con un máximo de cinco intentos por paso. Si ya hubo endoso, una ausencia
temporal del historial **no** permite reenviar: la construcción se detiene
si no puede confirmar el commit, y conserva esa guarda también en `--resume`.
Los eventos `failed`/`retrying` del journal y `failure.json` conservan fase,
código, transaction ID, códigos de los endpoints e indicador de fallo de
diseminación, sin argumentos ni propuestas.
La diseminación privada y el timeout interno de endoso de 30 s no cambian.

Las altas del snapshot pasan por `normalizeSteps`, igual que las recetas del
planner: los endosantes se derivan de cada transacción, no de un MSP fijo para
todo el plan. `RegisterUnit` apunta al laboratorio invocante; las recepciones
conservan el par emisor–receptor calculado por el planner. Esto evita delegar
el cálculo automático del plan de endoso de las altas al Gateway, sin cambiar
argumentos, políticas ni estado lógico. No elimina la necesidad de discovery
para localizar peers. El plan lógico y su hash cambian: los artefactos anteriores
se conservan como evidencia diagnóstica y no se reanudan como canónicos;
la siguiente construcción debe regenerar el plan y comenzar desde cero.

La ampliación autorizada de #137 reconoce además el detalle exacto de endoso
`Canceled: grpc: the client connection is closing`: Fabric puede cerrar la
conexión compartida tras otro timeout y cancelar una propuesta vecina.
La recuperación sigue exigiendo reconciliación y cinco intentos como máximo.
Cancelación explícita del operador, detalles mixtos/permanentes y ambigüedad
posterior al submit no autorizan nuevas escrituras. Las rondas medidas no
incorporan estos reintentos de preparación.

El diagnóstico del gate de #137 separa dos fallos: cierres gRPC por pings y
un timeout de endoso coincidente con una escritura de estado de Lab de ~31 s.
La causa física de esa pausa sigue pendiente de evidencia; no se atribuye a
CPU, memoria o almacenamiento sin muestras. El controlador persistente del
gate se guarda bajo su directorio ignorado en `build/benchmarks/fabric/`,
captura logs permitidos antes de limpiar contenedores y registra recursos
(CPU, memoria, I/O y throttling) cada 15 s **sólo durante la construcción**.
Antes de las rondas medidas se detiene ese muestreo adicional.
No contiene hooks de apagado: tanto éxito como fallo dejan la PC encendida.
Después del arranque/restauración no basta con `/healthz` ni con lecturas
locales: gossip/discovery puede todavía omitir una organización remota. Antes
de construir o medir, una guarda acotada a 180 s exige dos pasadas completas
desde los siete Gateways hacia los siete peers. Usa propuestas de lectura
`org.hyperledger.fabric:GetMetadata`, comprueba las siete MSP en los endosos
y **nunca hace submit**. No modifica unidades, historial ni marcadores y no
añade reintentos a la ventana medida. Si no converge, bloquea el inicio.
La restauración diagnóstica local reprodujo lecturas locales exitosas con
discovery remoto incompleto, que convergió tras 52,324 s; esto explica ese
fallo de arranque, no demuestra la causa física de las pausas de Lab.
La prueba de transporte requiere 15 minutos sin consultas de chaincode entre
dos lecturas contra los siete peers; véase [configuración](../../network/compose.md).

Desde `benchmarks/caliper`:

```bash
npm run snapshot:build -- --concurrency 64 --max-attempts 5
# Después de una interrupción, usando el directorio .building-* informado:
npm run snapshot:build -- --resume --snapshot-dir <ruta> --concurrency 64

npm run snapshot:restore -- --snapshot-dir <ruta-del-snapshot>
npm run snapshot:verify -- --snapshot-dir <ruta-del-snapshot> --concurrency 64
npm run warm:peers -- --snapshot-dir <ruta-del-snapshot>
npm run smoke -- --snapshot-dir <ruta-del-snapshot>
```

El snapshot terminado se guarda en
`build/benchmarks/fabric/snapshots/<snapshot-id>/`. Su manifiesto estricto liga
el dataset, fuentes, contrato, package ID, red, verificación semántica y los
diez volúmenes de tres orderers y siete peers. La restauración acepta únicamente
esa allowlist y verifica tanto el SHA-256 del tar como el digest ordenado del
árbol restaurado. No usa `docker prune` ni borrados globales.

## Rondas medibles

El runner `npm run round` consume exclusivamente recetas del bundle compartido
con seed `20260727`. Mantiene `--state-mode prepare` como valor predeterminado
compatible: prepara fuera de la ventana medida sólo los prefijos requeridos.
Con `--state-mode snapshot`, exige `--snapshot-dir`, valida el manifiesto y las
precondiciones de todas las unidades de la ronda y no ejecuta preparación.
Encontrar contaminación experimental hace fallar la ronda antes de Caliper.

La interfaz es:

```text
npm run round -- \
  --scenario <write-register|write-transfer|write-dispense|read-unit|read-history|mixed|expected-rejections> \
  --phase <warmup|measurement> \
  --repetition <0..8> \
  --rate <tasa permitida> \
  [--family <UNAUTHORIZED_TRANSFER|DUPLICATE_IDENTITY|BLOCKING_STATE>] \
  [--operation <transfer|dispense>] \
  [--state-mode <prepare|snapshot>] \
  [--snapshot-dir <ruta>] \
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

## Series reproducibles

El [procedimiento del gate Docker](../../docs/fabric-gate.md) distingue la
calificación completa de un snapshot de los controles obligatorios por ronda,
describe sus prerrequisitos y la evidencia que debe registrarse en PR #141.
No hace falta reconstruir el snapshot para cada ronda; sí restaurarlo y
verificar su compatibilidad. El artefacto actual está ligado al HEAD exacto,
por lo que un nuevo commit exige una nueva construcción.

`npm run series` restaura el snapshot antes de cada intento y luego ejecuta la
verificación, el calentamiento dirigido a los siete peers, la medición de
recursos, el smoke y la ronda. Un warm-up fallido se repite exactamente una vez
sobre una nueva restauración; un segundo fallo bloquea ese escenario. Los
intentos fallidos o descartados conservan crudos, logs y motivo y no participan
del CV.

```bash
npm run series -- --plan full --snapshot-dir <ruta-del-snapshot>
npm run series -- --plan trial --snapshot-dir <ruta-del-snapshot>
```

`full` recorre las 20 combinaciones, ejecuta cinco mediciones y agrega las
repeticiones 6–8 cuando el CV poblacional del throughput efectivo o de p95
supera 15 %. `trial` ejecuta `read-unit` a 50 TPS, `write-transfer` a 20
pares/s y `mixed` a 20 TPS, con dos mediciones cada uno; queda marcado como no
citable. El índice estricto se guarda en
`build/benchmarks/fabric/series/<token>/series-index.json`.

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

## Disponibilidad (EVAL-4)

`run-availability.js` ejecuta los seis escenarios de la sección 8.1 del
protocolo. Cada ronda corre la carga mixta de referencia (20 operaciones
conceptuales/s, ciclo 2/11/2/5) durante tres ventanas de 60 s y, desde fuera de
los workloads, da de baja el componente del escenario con `docker kill` a los
60 s de la primera operación y lo vuelve a levantar con `docker start` a los
120 s.

| Escenario | Componente dado de baja | Restauración |
|---|---|---|
| `raft-1` | El orderer líder del canal, según los logs de etcdraft | a los 120 s |
| `raft-2` | El líder y el siguiente orderer en el orden de consenters: pérdida de quórum | después de la ronda, fuera de la medición |
| `peer-1a` | `peer0.centromedico`: custodio de las unidades que dispensa y receptor de sus transferencias | a los 120 s |
| `peer-1b` | `peer0.distribuidor`: receptor de los tránsitos laboratorio → distribuidor | a los 120 s |
| `peer-1c` | `peer0.anmat`: organización regulatoria | a los 120 s |
| `peer-1d` | `peer0.lab`: laboratorio que registra unidades | a los 120 s |

**Qué simula.** Fallas de un contenedor en un único host. Miden la tolerancia a
la caída de nodos asignados a organizaciones distintas, no la caída física ni
administrativa de una organización completa ni una infraestructura distribuida
(ADR-007). La metadata y las notas de cada ronda lo declaran.

**Clases de operación.** En los escenarios de peer, cada operación del crudo
lleva `availabilityClass`: `target` si necesita el peer caído y `control` si no.
La clase se deriva del plan determinístico, sin tocar el workload. Una operación
necesita un peer cuando es el Gateway de su invocador o uno de sus endosantes;
`availabilityRequirement` registra cuál de las dos cosas aplica. En la carga
mixta cada organización entra por el Gateway de su propio peer, de modo que las
operaciones de la organización caída fallan ya en su Gateway. Las que la
necesitan sólo como endosante son, por ejemplo, las recepciones de otras
organizaciones cuando cae el laboratorio emisor (`peer-1d`).

Reparto de la carga mixta por organización, que explica la elección de cada
peer:

| Operación | Invocador | Endosantes |
|---|---|---|
| Registro | Laboratorio | Laboratorio |
| Transferencia | Laboratorio (despacho) y receptor (recepción) | Laboratorio; laboratorio y receptor |
| Dispensación | Centro médico o farmacia | El mismo |
| Lectura | Laboratorio | — |

La clase refleja lo que la operación exige, no todo lo que puede afectarla. En
`peer-1c`, con el peer de ANMAT caído, una parte de las transferencias de control
falla igual: ANMAT es miembro de todas las colecciones de transferencia, y el
peer que endosa a veces no logra diseminar los datos privados antes de endosar
(«failed to distribute private collection» en el log del laboratorio). En la
ronda de prueba fue cerca del 6 % de las transferencias, durante la falla y los
primeros segundos de la recuperación. Es un acople de disponibilidad de las PDC,
no un error de clasificación, y por eso se reporta aparte.

**Sonda regulatoria de `peer-1c`.** La carga mixta no tiene eventos regulatorios,
así que la caída de ANMAT no la afecta. Para observar la clase objetivo, la
ronda agrega una sonda de 1 cuarentena por segundo invocada por ANMAT sobre
unidades filler del snapshot (registradas, en custodia del laboratorio y fuera
de todos los planes). Cada cuarentena exige el endoso de ANMAT, por su marcador
de participación, y el del laboratorio custodio. Es una desviación declarada de
la carga: queda en `probe.jsonl`, en `raw.json` con `type: regulatory-event` y
en las notas de la metadata.

**Plan de 180 s.** El snapshot canónico prepara las unidades de la mixta de
120 s. El plan de disponibilidad conserva, worker por worker, las 2800
operaciones de esa mixta y continúa con el mismo ciclo sobre unidades que el
snapshot preparó para los escenarios de una sola operación (`write-register`,
`write-transfer`, `write-dispense` y `read-unit`). Una ronda de disponibilidad
parte de su propia restauración y no usa esas unidades. `work-plan.json`
registra cuántas operaciones vienen de cada origen, y la verificación previa
cubre todas las unidades, incluidas las de la sonda.

**Reconexión del cliente.** Con el backoff por defecto de grpc-js, un cliente
que perdió la conexión con un peer tardaba unos 9 s más que un cliente nuevo en
volver a usarlo después de restaurarlo (10,7 s frente a 1,7 s). El tiempo de
recuperación habría medido al cliente. En las rondas de disponibilidad la
configuración de red acota ese backoff a 1 s; las demás rondas no cambian.

**Descarte.** En estas rondas los fallos durante la falla son el resultado que
se mide. La ronda sólo se descarta si alguna operación **terminó** fallando antes
de inyectar la falla, porque entonces el estado previo no era estable. Las que
estaban en vuelo al inyectarla y fallan por ella no cuentan: en Raft-1 son unas
50 operaciones iniciadas en los últimos segundos antes de la caída del líder.

**Evidencia por ronda.** Además de las salidas de una ronda normal:

| Archivo | Contenido |
|---|---|
| `fault-events.json` | Contenedores dados de baja, instantes de `kill` y `start`, líder Raft antes y después, ventanas y las líneas de log que explican la falla |
| `logs/<contenedor>.log` | Logs con timestamps de los tres orderers, del peer caído y del peer del laboratorio, acotados a la ronda con 15 s de margen |
| `probe.jsonl` | Operaciones de la sonda regulatoria (`peer-1c`) |
| `metadata.json` | `faultInjection` con los instantes registrados y las duraciones de cada ventana derivadas de ellos |

Las series usan los mismos controles por intento que las de rendimiento
(restauración, verificación, calentamiento de peers, recursos y smoke):

```bash
npm run series -- --plan availability --snapshot-dir <ruta-del-snapshot>
npm run series -- --plan availability-trial --snapshot-dir <ruta-del-snapshot>
```

`availability` es citable: un warm-up y cinco mediciones por escenario.
`availability-trial` hace un warm-up y una medición por escenario y queda
marcado como no citable. No se extiende la serie por CV: la variación entre
ventanas la produce la falla a propósito. Estas rondas no entran en las
estadísticas de rendimiento estable; el procesamiento (`benchmarks/analysis`)
calcula, por ventana y por clase, la tasa de éxito, el throughput, el p95 y el
tiempo de recuperación de la sección 3.3.
