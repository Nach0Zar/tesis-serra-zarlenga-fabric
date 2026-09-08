# Baseline centralizada

Este directorio contiene la API REST en Go y el esquema PostgreSQL de la línea
base centralizada definida por [ADR-012](../docs/adr/012-baseline-design.md).
La implementación de BASE-2 (#38) cubre los procesos core de M2 y consume el
paquete compartido [`domain`](../domain/README.md) para la máquina de estados y
la matriz de transferencias.

## Requisitos

- Docker con el plugin Compose;
- Bash y GNU Make;
- Go 1.23;
- `shellcheck` para la validación estática de los scripts.

La contraseña no tiene un valor por defecto y no debe guardarse en el
repositorio. Para una sesión local puede generarse una credencial efímera:

```bash
export SNT_BASELINE_DB_PASSWORD="$(openssl rand -hex 32)"
```

Variables opcionales:

| Variable | Valor por defecto | Uso |
|---|---|---|
| `SNT_BASELINE_DB_NAME` | `snt_baseline` | Base de datos creada por PostgreSQL. |
| `SNT_BASELINE_DB_USER` | `snt_baseline` | Usuario administrador de las migraciones. |
| `SNT_BASELINE_DB_PORT` | `5432` | Puerto publicado exclusivamente en `127.0.0.1`. |
| `SNT_BASELINE_DATABASE_URL` | — | DSN PostgreSQL de la API. Obligatorio al iniciar el servidor. |
| `SNT_BASELINE_API_KEYS` | — | Array JSON de credenciales estáticas. Obligatorio al iniciar el servidor. |
| `SNT_BASELINE_LISTEN_ADDR` | `:8080` | Dirección HTTP de escucha. |

## Uso

Desde la raíz del repositorio:

```bash
make -C baseline db-up
make -C baseline migrate-up
make -C baseline migrate-version
```

`migrate-down` revierte únicamente la última migración aplicada. `db-down`
detiene PostgreSQL pero conserva el volumen de datos.

```bash
make -C baseline migrate-down
make -C baseline db-down
```

La API se inicia después de aplicar las migraciones:

```bash
export SNT_BASELINE_DATABASE_URL="postgres://snt_baseline:$SNT_BASELINE_DB_PASSWORD@127.0.0.1:5432/snt_baseline?sslmode=disable"
export SNT_BASELINE_API_KEYS='[
  {"key":"valor-no-versionado","mspId":"LabMSP","role":"operator"},
  {"key":"otro-valor-no-versionado","mspId":"AnmatMSP","role":"regulatory-admin"}
]'
cd baseline && go run ./cmd/snt-baseline
```

La configuración admite exactamente una key por par `mspId`+rol y conserva en
memoria solamente su SHA-256. Las operaciones de escritura requieren el header
`X-Org-Key`; la organización se resuelve después contra `organizations` y se
validan `active`, `agentType`, custodio y rol. Las lecturas no exigen esa key,
igual que `ReadUnit`, `GetUnitHistory` y `QueryUnitsByGTIN` en el chaincode.

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

La prueba usa un proyecto Compose y un volumen aislados, aplica las migraciones,
ejecuta los tests Go —incluido el recorrido HTTP→PostgreSQL y el orden bajo
concurrencia—, verifica estructura y restricciones, comprueba la inmutabilidad
de las devoluciones, ejecuta `down`, reaplica `up` y elimina todos sus recursos
al finalizar. El target ejecuta `gofmt`, `shellcheck` y `go vet` antes de la
integración:

```bash
make -C baseline test
```

## Fuera de alcance de BASE-2

- eventos extraordinarios, devoluciones T21–T24, intervención de laboratorio y
  `VerifyTrace` (BASE-3, #39);
- `VerifyUnit`, agregado al contrato después de la versión 2.6.1 que gobierna
  esta issue;
- seed del dataset, contenedor de la API y CI de la baseline (BASE-4, #40);
- emulación de MSP, PKI, endoso, canales o Private Data Collections.
