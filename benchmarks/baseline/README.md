# Benchmark de la baseline REST

Este proyecto ejecuta contra la baseline centralizada los mismos perfiles,
selecciones de dataset y operaciones conceptuales que el benchmark de Fabric.
Importa directamente el planificador, los perfiles, la validación del bundle y
el agregador de [`../caliper/`](../caliper/); las tasas, bandas y reglas de la
mezcla no se mantienen en una segunda copia.

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
cd benchmarks/baseline
npm ci
npm run check
npm test
npm run test:metadata
```

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
versión Fabric de referencia.

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

La preparación del snapshot queda fuera de la ventana medida. Si el reloj del
host retrocede entre un despacho de preparación y su recepción, el constraint
temporal de PostgreSQL rechaza el cierre con `INTERNAL_ERROR`; el constructor
espera 1,1 segundos y reintenta esa recepción una única vez. El manifiesto
contabiliza esos reintentos técnicos. Ninguna ronda medida usa este mecanismo.

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
referencias de equivalencia con Fabric. Todo `build/` está ignorado por Git.

## Asimetrías preservadas

- La identidad REST se emula con API keys; no existen MSP, PKI ni certificados.
- El commit durable es una transacción PostgreSQL única; no hay endoso,
  ordering, canal ni colección privada.
- La transferencia sigue siendo bifásica, pero no necesita esperar
  diseminación entre peers ni reintenta estado privado.
- Los timestamps provienen del servidor de la API y no de una propuesta
  Fabric.
- Los historiales son append-only por convención de aplicación y un
  administrador de base puede alterarlos sin las garantías criptográficas de
  un ledger distribuido.
- `lab_intervention_events` no participa en ninguna ruta medida y debe
  permanecer sin escrituras en el snapshot y las rondas.
- El runner no mide operaciones incorporadas posteriormente ni disponibilidad,
  y no orquesta repeticiones o procesamiento estadístico.
