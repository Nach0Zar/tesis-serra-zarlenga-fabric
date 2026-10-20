# Benchmark de la baseline REST

Este proyecto ejecuta contra la baseline centralizada los mismos perfiles,
selecciones de dataset y operaciones conceptuales que el benchmark de Fabric.
Importa directamente el planificador, los perfiles, la validación del bundle y
el agregador de [`../caliper/`](../caliper/); las tasas, bandas y reglas de la
mezcla no se mantienen en una segunda copia.

El plan golden también se importa de `../caliper/src/snapshot-plan.js`.
`logical-plan.json` conserva exactamente la misma serialización y hash para
ambos SUT: recetas normalizadas, presencia, estado, custodio y unidad de smoke.
Fabric restaura volúmenes de ledger y PostgreSQL un dump; son representaciones
físicas distintas del mismo estado lógico. Los identificadores de transacción
y timestamps pertenecen a cada SUT y no se exigen iguales al comparar el historial.

El runner no cambia la API ni prepara datos por SQL. Construye un snapshot
golden mediante el seed atómico existente y completa sus precondiciones por la
API REST. Cada ronda restaura ese snapshot en un proyecto Compose aislado,
ejecuta un preflight de lectura y recién después abre la ventana medida.

## Requisitos y credenciales

- Node.js 22 y npm 11;
- Go 1.23 para generar el dataset y validar `metadata.json`;
- Docker Engine y Compose;
- checkout confirmado en un commit, sin la red Fabric en ejecución;
- bundle canónico en `build/dataset`.

La contraseña y las API keys se pasan únicamente por variables de entorno. El
cliente conserva las keys en memoria, las usa sólo en `X-Org-Key` y nunca las
escribe en configuración, logs ni resultados:

```bash
export SNT_BASELINE_DB_PASSWORD='secreto-efimero-apto-para-url'
export SNT_BASELINE_API_KEYS='[
  {"mspId":"AnmatMSP","role":"regulatory-admin","key":"reemplazar-anmat"},
  {"mspId":"LabMSP","role":"operator","key":"reemplazar-lab"},
  {"mspId":"DrogueriaMSP","role":"operator","key":"reemplazar-drogueria"},
  {"mspId":"DistribuidorMSP","role":"operator","key":"reemplazar-distribuidor"},
  {"mspId":"FarmaciaMSP","role":"operator","key":"reemplazar-farmacia"},
  {"mspId":"CentroMedicoMSP","role":"operator","key":"reemplazar-centro"},
  {"mspId":"FinanciadorMSP","role":"financier-auditor","key":"reemplazar-financiador"}
]'
```

Por defecto se usan el proyecto `snt_baseline_benchmark`, el puerto API 18080
y el puerto PostgreSQL 15432. Pueden cambiarse con
`SNT_BASELINE_BENCHMARK_PROJECT`, `SNT_BASELINE_API_PORT` y
`SNT_BASELINE_DB_PORT`. Nunca debe reutilizarse el proyecto Compose de
desarrollo.

## Instalación y controles rápidos

```bash
npm ci --prefix benchmarks/caliper
cd benchmarks/baseline
npm ci
npm run check
npm test
npm run test:metadata
```

La baseline reutiliza módulos del proyecto hermano `benchmarks/caliper`: el
plan determinístico, la agregación y, para las series, el orquestador, que
valida el índice con `ajv`. Node resuelve esas dependencias desde
`benchmarks/caliper/node_modules`, así que hay que instalarlas ahí. Instalarlas
sólo en la baseline no alcanza. Si faltan, `npm run series` termina con un
mensaje que indica el comando.

`npm test` requiere el dataset canónico. Se genera desde su fuente única con:

```bash
make -C client generate-dataset
```

## Snapshot golden

La unión determinística de las veinte combinaciones de perfil produce:

| Grupo | Unidades |
|---|---:|
| Ausentes, reservadas para altas medidas | 3.080 |
| Presentes con la precondición requerida | 22.103 |
| Presentes como relleno | 24.817 |
| Total | 50.000 |

Una unidad del relleno se reserva para el smoke de sólo lectura y queda fuera
de todas las bandas medidas. Para construir el snapshot:

```bash
npm run snapshot:build -- --dataset-dir build/dataset --snapshot-token golden-local
```

El comando valida seed, hash y fuentes canónicas; hace el seed selectivo en una
transacción; ejecuta las preparaciones por REST; verifica las 50.000
precondiciones y los conteos de las tablas; exige cero filas en
`lab_interventions` y `lab_intervention_events`; y genera un `pg_dump` custom
format. El resultado queda en
`build/benchmarks/baseline/snapshots/golden-local/` con hashes, precondiciones y
un manifiesto ligado al commit, imagen, PostgreSQL, contrato, package ID y
versión Fabric de referencia. El manifiesto y el contexto de cada corrida
registran `logicalPlanSHA256`; el verificador compara también el historial
lógico completo con cada paso de la receta, conservando su orden.

Una restauración aislada puede comprobarse con:

```bash
npm run snapshot:restore -- --snapshot-dir build/benchmarks/baseline/snapshots/golden-local
```

El servicio se detiene y el volumen aislado se elimina al terminar. Para una
inspección manual se puede definir `SNT_BASELINE_RESTORE_KEEP_RUNNING=1`.
La imagen exacta declarada por el manifiesto debe permanecer disponible en el
host: la restauración no la reconstruye ni acepta otra imagen con el mismo tag.

## Smoke y rondas

El smoke restaura el snapshot y ejecuta 30 `ReadUnit` a 1 TPS:

```bash
npm run smoke -- \
  --snapshot-dir build/benchmarks/baseline/snapshots/golden-local \
  --run-token smoke-local
```

Cada invocación de `round` ejecuta exactamente una ronda:

```bash
npm run round -- \
  --snapshot-dir build/benchmarks/baseline/snapshots/golden-local \
  --scenario write-transfer \
  --phase warmup \
  --repetition 0 \
  --rate 5
```

Escenarios y tasas admitidos:

| Escenario | Workers | Duración | Tasas (operaciones/s) |
|---|---:|---:|---|
| `write-register` | 2 | 120 s | 5, 10, 20 |
| `write-transfer` | 2 | 120 s | 5, 10, 20 pares/s |
| `write-dispense` | 2 | 120 s | 5, 10, 20 |
| `read-unit` | 4 | 120 s | 10, 25, 50 |
| `read-history` | 4 | 120 s | 10, 25, 50 |
| `mixed` | 2 | 120 s | 20 |
| `expected-rejections` | 2 | 60 s | 5 |

La mezcla repite ciclos exactos de veinte operaciones: 2 altas, 11
transferencias, 2 dispensaciones y 5 lecturas de unidad. Las rondas de rechazo
se ejecutan por separado:

```bash
npm run round -- --snapshot-dir <snapshot> --scenario expected-rejections \
  --phase warmup --repetition 0 --rate 5 --family UNAUTHORIZED_TRANSFER
npm run round -- --snapshot-dir <snapshot> --scenario expected-rejections \
  --phase warmup --repetition 0 --rate 5 --family DUPLICATE_IDENTITY
npm run round -- --snapshot-dir <snapshot> --scenario expected-rejections \
  --phase warmup --repetition 0 --rate 5 --family BLOCKING_STATE --operation transfer
npm run round -- --snapshot-dir <snapshot> --scenario expected-rejections \
  --phase warmup --repetition 0 --rate 5 --family BLOCKING_STATE --operation dispense
```

Una transferencia feliz es una operación conceptual y dos requests
secuenciales. Los crudos conservan las latencias de despacho y recepción, más
la latencia end-to-end del par. La respuesta se lee por completo: en
escrituras, la latencia termina después del commit PostgreSQL confirmado por la
API. La baseline no incorpora los reintentos propios de Fabric.

El generador usa lazo abierto: cada worker agenda envíos a su fracción de la
tasa conceptual, independientemente de las respuestas en vuelo. Al cerrar la
ventana deja de admitir operaciones y espera las respuestas pendientes.
`scheduledAt` y `submissionDelayMs` permiten auditar la cadencia. La tasa
ofrecida usa el número real de operaciones enviadas en la ventana; la efectiva
incluye el tiempo de drenaje. Una ronda incompleta se descarta.

La preparación del snapshot queda fuera de la ventana medida y aborta ante
cualquier error. No reintenta `INTERNAL_ERROR`: la API no publica la causa
interna y no permite atribuirla con seguridad al reloj del host.

Antes de medir, el runner verifica las unidades de la ronda contra las
precondiciones hasheadas y ejecuta el smoke fuera de la ventana. Un timeout,
código de rechazo distinto, éxito inesperado, precondición contaminada o par
incompleto conserva la evidencia, marca la ronda como descartada y hace fallar
el comando.

## Evidencia

Cada ronda escribe bajo `build/benchmarks/baseline/runs/<token>/`:

- `effective-config.json`, sin credenciales;
- `work-plan.json`;
- `snapshot-context.json` y `run-context.json`;
- `preflight/`, con evidencia separada del smoke;
- partes `raw-worker-*.jsonl` y `raw.json`;
- `summary.json`;
- `metadata.json`, validado con `client/cmd/runmeta`.

El metadata usa exclusivamente los identificadores de la baseline. El contexto
adjunto conserva la versión contractual y el package ID completo como
referencias de equivalencia con Fabric. `environment.baselineImage` usa el
digest inmutable de la imagen para respetar el límite del schema; la referencia
completa con tag y digest permanece en el manifiesto y el contexto.
Todo `build/` está ignorado por Git.

## Series reproducibles

`npm run series` le da a la baseline el mismo índice de serie que Fabric
(`benchmarks/schema/fabric-series-index.schema.json`). Sin ese índice el
procesamiento no puede acreditar sus rondas como citables. Reutiliza el
orquestador de Caliper: un warm-up por escenario, que se reintenta una vez si se
descarta; cinco mediciones, que se extienden a ocho por CV en `full`; y los
intentos fallidos o descartados quedan registrados con su motivo. Cada intento
es una ronda de la baseline, que ya restaura el snapshot, verifica las
precondiciones y corre su smoke, y se guarda dentro del directorio de la serie.

```bash
npm run series -- --plan full --snapshot-dir <snapshot>
npm run series -- --plan availability --snapshot-dir <snapshot>
```

| Plan | Escenarios | Citable |
|---|---|---|
| `full` | Las 20 combinaciones de rendimiento | sí |
| `trial` | `read-unit` 50, `write-transfer` 20 y `mixed` 20, dos mediciones | no |
| `availability` | `db-1` y `api-1`, cinco mediciones, extendidas a ocho por CV como en `full` | sí |
| `availability-trial` | `db-1` y `api-1`, una medición | no |

El índice queda en `build/benchmarks/baseline/series/<token>/series-index.json`.

## Disponibilidad (EVAL-5)

`npm run availability` ejecuta los dos escenarios de la sección 8.2 de forma
simétrica a EVAL-4 (#44). Usa la misma carga mixta de 20 operaciones
conceptuales/s durante 180 s, con el mismo plan determinístico, y las mismas
tres ventanas de 60 s. A los 60 s de iniciada la carga hace `docker compose
kill` del servicio y a los 120 s lo vuelve a levantar con `docker compose
start`.

| Escenario | Servicio dado de baja |
|---|---|
| `db-1` | `postgres`: la única base de datos del sistema |
| `api-1` | `api`: el único punto de entrada de los clientes |

Las fallas se simulan con el mismo nivel que en Fabric: un contenedor caído en
un único host. Lo que se demuestra no depende de ese nivel. En la baseline no
hay componente redundante, de modo que cualquiera de los dos deja sin servicio a
todos los actores; es una propiedad de la topología centralizada, no del montaje.
La metadata y las notas de cada ronda lo declaran.

Como en EVAL-4, la ronda sólo se descarta si alguna operación terminó fallando
antes de la falla, y conserva `fault-events.json`, los logs de `postgres` y
`api` acotados a la ronda y `faultInjection` en la metadata. No hay clases de
operación: todas dependen de los dos servicios. Estas rondas no entran en las
estadísticas de rendimiento estable.

## Asimetrías preservadas

- La identidad REST se emula con API keys; no existen MSP, PKI ni certificados.
- El commit durable es una transacción PostgreSQL única; no hay endoso,
  ordering, canal ni colección privada.
- La transferencia sigue siendo bifásica, pero no necesita esperar
  diseminación entre peers ni reintenta estado privado.
- Los timestamps provienen del servidor de la API y no de una propuesta
  Fabric. Un retroceso del reloj entre despacho y recepción puede violar el
  constraint temporal de PostgreSQL: la preparación falla, o la ronda medida
  se descarta, con evidencia preservada.
- Los historiales son append-only por convención de aplicación y un
  administrador de base puede alterarlos sin las garantías criptográficas de
  un ledger distribuido.
- `lab_intervention_events` no participa en ninguna ruta medida y debe
  permanecer sin escrituras en el snapshot y las rondas.
- El runner de rondas no mide operaciones incorporadas posteriormente. La
  disponibilidad y las series tienen sus propios comandos (ver abajo).

Los límites efectivos de CPU/memoria de API y PostgreSQL se registran en el
manifiesto y el contexto, fuera del schema de metadata. Este Compose no impone
cuotas individuales; Fabric conserva sus límites por contenedor. La igualdad
de host y estado lógico no acredita un presupuesto agregado idéntico. La
decisión de comparabilidad de recursos y el entorno canónico quedan en #54
antes de resultados citables. Los ensayos diagnósticos previos no pueden
compararse con Fabric cuando usaba prefijos sobre un ledger casi vacío.
