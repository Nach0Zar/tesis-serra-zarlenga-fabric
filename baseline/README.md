# Baseline centralizada

Este directorio contiene la API REST en Go y el esquema PostgreSQL de la línea
base centralizada definida por [ADR-012](../docs/adr/012-baseline-design.md).
La implementación cubre el subconjunto de procesos core de M2 y el empaquetado
reproducible de BASE-4 (#40). La API y el seed consumen el paquete compartido
[`domain`](../domain/README.md) para la máquina de estados y la matriz de
transferencias.

## Requisitos y configuración

Para ejecutar la baseline sólo se requieren Docker y el plugin Compose. Los
targets de desarrollo y CI también requieren Bash, GNU Make, Go 1.23 y
`shellcheck`.

La contraseña y las API keys son obligatorias y no deben guardarse en el
repositorio. La contraseña debe ser apta para una URL; `openssl rand -hex`
produce un valor seguro para este uso.

```bash
export SNT_BASELINE_DB_PASSWORD="$(openssl rand -hex 32)"
export SNT_BASELINE_API_KEYS='[
  {"key":"reemplazar-por-un-secreto-efimero","mspId":"LabMSP","role":"operator"}
]'
```

| Variable | Valor por defecto | Uso |
|---|---|---|
| `SNT_BASELINE_DB_PASSWORD` | — | Contraseña PostgreSQL obligatoria. |
| `SNT_BASELINE_API_KEYS` | — | Array JSON de credenciales estáticas obligatorio. |
| `SNT_BASELINE_DB_NAME` | `snt_baseline` | Base de datos creada por PostgreSQL. |
| `SNT_BASELINE_DB_USER` | `snt_baseline` | Usuario de la baseline y las migraciones. |
| `SNT_BASELINE_DB_PORT` | `5432` | Puerto PostgreSQL publicado sólo en `127.0.0.1`. |
| `SNT_BASELINE_API_PORT` | `8080` | Puerto HTTP publicado sólo en `127.0.0.1`. |
| `SNT_BASELINE_DATABASE_URL` | calculada por Compose | DSN interna compartida por migraciones, API y seed. |
| `SNT_BASELINE_DATASET_DIR` | `../build/dataset` | Bundle CLI-3 montado en el servicio de seed. |
| `SNT_BASELINE_IMAGE` | `snt-baseline:local` | Nombre local de la imagen compartida por API y seed. |

## Arranque

Desde la raíz, cualquiera de estos comandos construye la imagen, inicia
PostgreSQL y la API, aplica las migraciones y espera sus healthchecks:

```bash
docker compose -f baseline/compose.yaml up -d --build --wait
# equivalente:
make -C baseline up
```

El contenedor corre con un usuario sin privilegios, filesystem de sólo lectura y
puertos ligados a loopback. `make -C baseline down` detiene los servicios y
conserva los datos; para eliminar también el snapshot:

```bash
docker compose -f baseline/compose.yaml --profile seed \
  down --volumes --remove-orphans
```

Los targets `db-up`, `db-down`, `migrate-up`, `migrate-down` y
`migrate-version` permanecen disponibles para trabajar sólo con PostgreSQL.
`migrate-down` revierte únicamente la última migración aplicada.

## Dataset y seed

El dataset no se duplica dentro de la baseline: se genera con CLI-3 y su seed
fija `20260727`. Luego el servicio one-shot valida el manifiesto, el sidecar
SHA-256, las versiones de las fuentes embebidas, las organizaciones, el orden y
la unicidad antes de abrir la transacción de carga.

```bash
make -C baseline generate-dataset
make -C baseline seed
```

El resultado correcto informa seed, hash, siete organizaciones y al menos
50.000 unidades. El snapshot ejecuta exclusivamente `RegisterUnit`: cada
unidad queda en `EN_LABORATORIO`, con el laboratorio como custodio y un único
evento de secuencia 1. Las recetas de transferencia, rechazo y dispensa quedan
sin ejecutar para EVAL-3.

La carga es atómica y exige que las seis tablas de dominio estén vacías. Una
segunda ejecución falla sin modificar filas. Para usar otro directorio:

```bash
make -C baseline generate-dataset DATASET_DIR=/ruta/absoluta
make -C baseline seed DATASET_DIR=/ruta/absoluta
```

Una comprobación rápida del snapshot puede hacerse con:

```bash
docker compose -f baseline/compose.yaml exec -T postgres \
  psql -U "${SNT_BASELINE_DB_USER:-snt_baseline}" \
  -d "${SNT_BASELINE_DB_NAME:-snt_baseline}" \
  -c 'SELECT count(*) FROM public.medication_units;'
curl 'http://127.0.0.1:8080/v1/units?gtin=07791234567898'
```

La configuración admite exactamente una key por par `mspId`+rol y conserva
en memoria solamente su SHA-256. Las escrituras requieren `X-Org-Key`; las
lecturas no requieren credencial. Una key ausente o desconocida devuelve
`UNAUTHORIZED_ROLE`, asimetría documentada de la identidad emulada.

## Endpoints core

| Método | Path | Operación equivalente |
|---|---|---|
| `POST` | `/v1/units` | `RegisterUnit` |
| `POST` | `/v1/units/{gtin}/{numeroSerie}/dispatch` | `DispatchTransfer` |
| `POST` | `/v1/units/{gtin}/{numeroSerie}/receive` | `ReceiveTransfer` |
| `POST` | `/v1/units/{gtin}/{numeroSerie}/reject` | `RejectTransfer` |
| `POST` | `/v1/units/{gtin}/{numeroSerie}/dispense` | `Dispense` |
| `GET` | `/v1/units/{gtin}/{numeroSerie}` | `ReadUnit` |
| `GET` | `/v1/units/{gtin}/{numeroSerie}/history` | `GetUnitHistory` |
| `GET` | `/v1/units?gtin={gtin}` | `QueryUnitsByGTIN` |
| `POST` | `/v1/organizations` | `RegisterOrganization` |
| `PATCH` | `/v1/organizations/{mspId}` | `SetOrganizationActive` |

El body de `dispatch` reúne el destino y los datos documentales que en Fabric
viajan por `transient`: `destino`, `numeroRemito`, `numeroFactura` y `cantidad`.
El body de `receive` es opcional; si se envía, debe contener los tres campos
documentales completos. `dispense` no recibe datos del paciente.

Los errores conservan `{code,message,details}`. El mapeo HTTP exhaustivo es:

- `400`: `INVALID_REQUEST`, `INVALID_DESTINATION`, `INVALID_LAB_INTERVENTION`;
- `404`: `UNIT_NOT_FOUND`, `LAB_INTERVENTION_NOT_FOUND`;
- `403`: fallas de identidad, autorización, contraparte o regla de transferencia;
- `409`: duplicados, estado/transición, tránsito, último regulador e intervención inactiva;
- `500`: `INTERNAL_ERROR` y códigos desconocidos.

## Esquema

La migración inicial crea exactamente seis tablas de dominio en `public`:

- `organizations`: espejo del registro organización-establecimiento;
- `medication_units`: estado público vigente de cada unidad;
- `unit_events`: historial append-only con el snapshot público completo;
- `lab_interventions`: autorización vigente de intervención de laboratorio;
- `transfer_operations`: ciclo activo/cerrado de cada transferencia;
- `return_operations`: historial inmutable de devoluciones T21-T24.

El runner mantiene su tabla técnica `schema_migrations` en el esquema separado
`baseline_meta`. No forma parte del modelo de dominio.

La migración `000002` agrega `unit_events.event_sequence`. La API bloquea la
fila de la unidad hasta confirmar la escritura y asigna el siguiente ordinal en
esa misma transacción. El historial tiene así un orden total por unidad que no
depende del timestamp ni de su resolución bajo concurrencia. La migración
también alinea la recepción documental opcional con el contrato del chaincode.

Conforme ADR-012 §5, `unit_events` es append-only por convención de aplicación:
la API sólo inserta eventos, pero un administrador de PostgreSQL puede
alterarlos con SQL directo. `return_operations`, cuyo histórico sí fue definido
como inmutable por ADR-012 §2, rechaza `UPDATE`, `DELETE` y `TRUNCATE` mediante
triggers.

## Validación

El gate local y de CI ejecuta formato, `shellcheck`, `go vet`, pruebas Go,
migraciones `up → test → down → up` y el recorrido real
API + PostgreSQL + generación CLI-3 + seed de 50.000 unidades. También verifica
el rechazo de un segundo seed y que no se hayan ejecutado operaciones de
workload. Cada script instala su cleanup antes de iniciar contenedores; el
workflow repite la limpieza bajo `always()`.

```bash
make -C baseline test
```

## Troubleshooting

- Si Compose informa una variable ausente, definir
  `SNT_BASELINE_DB_PASSWORD` y `SNT_BASELINE_API_KEYS` en la misma shell.
- Si el seed informa `ALREADY_INITIALIZED`, eliminar deliberadamente el volumen
  o usar un proyecto Compose nuevo; no mezcla snapshots existentes.
- Si falla el hash o una versión fuente, regenerar el bundle con el checkout
  actual mediante `make -C baseline generate-dataset`.
- Para inspeccionar fallos de arranque: `docker compose -f baseline/compose.yaml logs api postgres`.

## Fuera de alcance de BASE-4

- operaciones extraordinarias y verificaciones de BASE-3 (#39);
- ejecución de workloads, benchmarks y análisis de disponibilidad de EVAL-2 a
  EVAL-5;
- carga del snapshot de Fabric, emulación de MSP/PKI, políticas de endoso,
  canales o Private Data Collections;
- cambios al contrato REST, al modelo relacional o a los ADRs.
