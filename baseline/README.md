# Baseline centralizada

Este directorio contiene el esquema PostgreSQL de la línea base centralizada
definida por [ADR-012](../docs/adr/012-baseline-design.md). BASE-1 (#37) cubre
únicamente persistencia, migraciones y ejecución de PostgreSQL en Docker. La API
REST, sus credenciales, la lógica de dominio y el seed pertenecen a BASE-2,
BASE-3 y BASE-4.

## Requisitos

- Docker con el plugin Compose;
- Bash y GNU Make;
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

## Esquema

La migración inicial crea exactamente cinco tablas de dominio en `public`:

- `organizations`: espejo del registro organización-establecimiento;
- `medication_units`: estado público vigente de cada unidad;
- `unit_events`: historial append-only con el snapshot público completo;
- `transfer_operations`: ciclo activo/cerrado de cada transferencia;
- `return_operations`: historial inmutable de devoluciones T21-T24.

El runner mantiene su tabla técnica `schema_migrations` en el esquema separado
`baseline_meta`. No forma parte del modelo de dominio.

Conforme ADR-012 §5, `unit_events` es append-only por convención de aplicación:
la futura API sólo insertará eventos, pero un administrador de PostgreSQL puede
alterarlos con SQL directo. `return_operations`, cuyo histórico sí fue definido
como inmutable por ADR-012 §2, rechaza `UPDATE`, `DELETE` y `TRUNCATE` mediante
triggers.

## Validación

La prueba usa un proyecto Compose y un volumen aislados, aplica la migración,
verifica estructura y restricciones, comprueba la inmutabilidad de las
devoluciones, ejecuta `down`, reaplica `up` y elimina todos sus recursos al
finalizar. El target ejecuta `shellcheck` antes de la integración:

```bash
make -C baseline test
```

## Fuera de alcance de BASE-1

- API REST y resolución de `X-Org-Key` a organización y rol;
- seed del dataset y contenedor de la API;
- ejecución de transiciones o validaciones del paquete Go `domain`;
- emulación de MSP, endoso, canales o Private Data Collections.
