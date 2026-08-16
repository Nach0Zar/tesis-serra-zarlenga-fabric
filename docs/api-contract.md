# Contrato de interfaz del chaincode `snt`

- **Versión del contrato**: `2.0.0` (breaking change respecto de `1.0.0` — ver "Política de versionado y congelamiento" y la nota de conflicto con ADR-004 al final de esta sección)
- **Estado**: Congelado. Los cambios se rigen por la política de versionado (última sección): un cambio incompatible exige un PR etiquetado `breaking-change` y aprobación explícita de B.

> **Nota de este cambio (pendiente de aprobación explícita de B, no mergeado)**: la versión `1.0.0` de este contrato definía `destino` como argumento público de `DispatchTransfer` y `destinatarioPendiente` como campo público de `MedicationUnitView`. ADR-004 (revisión posterior a la primera versión de este contrato) decidió que el destinatario declarado durante el tránsito es un dato privado que debe viajar por `transient` y persistirse en PDC, no en estado público. Esta versión `2.0.0` alinea el contrato con esa decisión. Al tratarse de un cambio incompatible sobre un documento marcado "Congelado", requiere el mismo PR `breaking-change` y aprobación de B que exige cualquier otro cambio de firma — no se considera aprobado por el solo hecho de estar escrito acá.
- **Fecha**: 2026-08-13
- **Autores**: Serra, Zarlenga

---

Este documento define la superficie pública del chaincode `snt`: el nombre y la firma de cada función invocable, el JSON de request y response de cada operación, y el formato estandarizado de errores. Es el contrato contra el que se construyen el cliente (`client/`) y la baseline (`baseline/`), por lo que su estabilidad es un requisito: hasta que esté en la rama principal, esos componentes no pueden avanzar.

No define: la implementación interna del chaincode, el `configtx.yaml`, el material criptográfico, los nombres definitivos de colecciones privadas (NET-5), ni la lógica de la baseline. Fija únicamente el contrato observable desde el cliente.

## Relación con las decisiones existentes

| Fuente | Qué aporta al contrato |
|---|---|
| [ADR-001](adr/001-maquina-estados-medicamento.md) | Estados y transiciones (T01–T33). Cada operación de escritura corresponde a una o más transiciones; el chaincode rechaza cualquier transición no declarada. |
| [ADR-002](adr/002-topologia-canales.md) | Separación estado público / datos comerciales. Los datos comerciales y documentales viajan por el campo `transient` y van a PDC, nunca como argumentos públicos. |
| [ADR-003](adr/003-establishment-identity-gln-cufe.md) | Identidad por `cid.GetMSPID()` resuelta contra el registro. El custodio no viaja como parámetro cuando el invocador actúa como custodio; el destino de una transferencia sí. |
| [ADR-004](adr/004-transfer-dispatch-reception.md) | La transferencia son dos operaciones: `DispatchTransfer` y `ReceiveTransfer`, más `RejectTransfer`. El destinatario declarado viaja por `transient` y se valida contra PDC, nunca como argumento público ni campo de `MedicationUnitView`. **Dependencia de merge**: ADR-004 (DES-9) todavía no está en `develop`; este contrato lo asume decidido. |
| [ADR-005](adr/005-rol-organismo-financiador.md) | El organismo financiador solo invoca operaciones de lectura. **Dependencia de merge**: ADR-005 (DES-10) todavía no está en `develop`. |
| [modelo-datos.md](modelo-datos.md) | Struct `MedicationUnit`, clave compuesta GTIN+serie, `fechaVencimiento` en ISO 8601, `ultimaActualizacion` por `GetTxTimestamp()`. |
| [organizations-roles-endorsement.md (DES-6)](organizations-roles-endorsement.md) | Autorización por `agentType`, `active` y `snt.role`, y política de endoso por operación. |
| [domain/authorized-transfers.json (DES-3)](../domain/authorized-transfers.json) | Matriz origen → destino que valida `DispatchTransfer`. |

## Convenciones

### Firma y estilo

- El chaincode se implementa con `contractapi`. Las funciones públicas son métodos de un contrato `SNTContract` cuyo primer parámetro es `ctx contractapi.TransactionContextInterface`.
- Los nombres de función son **inglés PascalCase** (`RegisterUnit`, `DispatchTransfer`, `ReadUnit`).
- Los nombres de campo JSON son **español lowerCamelCase** (`gtin`, `numeroSerie`, `custodioActual`), coherentes con `modelo-datos.md`.
- Cada operación de escritura recibe su request como un único objeto JSON (parámetro `req`, deserializado por `contractapi`) y devuelve la vista pública resultante de la unidad. Las lecturas reciben parámetros escalares.

### Identidad y determinismo

- La identidad del invocador se resuelve con `cid.GetMSPID()` contra el registro organización-establecimiento (ADR-003). El cliente **no** envía el GLN/CUFE del invocador en operaciones donde este actúa como custodio.
- El identificador canónico persistido es `GLN:<13 dígitos>` o `CUFE:<13 dígitos>`.
- El timestamp de toda escritura se obtiene de `ctx.GetStub().GetTxTimestamp()`, nunca de `time.Now()` (determinismo de endoso — `modelo-datos.md` §3.5).

### Datos privados (ADR-002, ADR-004)

- La información comercial y documental (número de remito, número de factura, cantidades, condiciones) **nunca** viaja como argumento público. Se envía por el campo `transient` de la propuesta, bajo la clave `commercial`, con un objeto JSON.
- El identificador del destinatario declarado en una transferencia (`DispatchTransfer`) **tampoco** viaja como argumento público, por la misma razón: revela una relación emisor→receptor no consumada (ADR-004). Se envía por `transient`, bajo la clave `destinatario`, con un objeto JSON `{ "destino": "GLN:..." }`.
- El contrato fija que esos datos van a PDC; el nombre y la política de la(s) colección(es) los define NET-5, con membresía emisor + receptor declarado + `AnmatMSP` para la operación de transferencia (ADR-004). El estado público nunca contiene datos comerciales ni el destinatario declarado.

### Vista pública de la unidad (`MedicationUnitView`)

Es el response de todas las operaciones de escritura sobre una unidad y de `ReadUnit`. Refleja el estado público del canal (`modelo-datos.md` §3). **No incluye** el destinatario declarado durante una transferencia en curso: ADR-004 decidió que ese dato es privado y vive en PDC, no en el struct público — ver "Datos privados (ADR-002, ADR-004)".

```json
{
  "gtin": "07791234567890",
  "numeroSerie": "SN-0001-ABCD",
  "lote": "L2026-014",
  "fechaVencimiento": "2027-12-31",
  "custodioActual": "GLN:7791234500017",
  "estado": "EN_CUSTODIA",
  "ultimaActualizacion": "2026-08-13T14:32:10Z"
}
```

## Formato de errores

Toda operación que falla devuelve un `error` cuyo mensaje es un objeto JSON con esta forma:

```json
{
  "code": "TRANSFER_NOT_AUTHORIZED",
  "message": "El par LABORATORY -> PHARMACY no está autorizado por la matriz de transferencias.",
  "details": { "origen": "LABORATORY", "destino": "PHARMACY" }
}
```

- `code`: identificador estable del catálogo de abajo. El cliente y la baseline deben ramificar sobre `code`, no sobre `message`.
- `message`: texto legible en español, no estable entre versiones.
- `details`: objeto opcional con contexto estructurado; su ausencia es válida.

### Catálogo de códigos de error

| `code` | Cuándo |
|---|---|
| `INVALID_REQUEST` | JSON malformado, campo obligatorio ausente, o formato inválido (dígito verificador GTIN, longitud/prefijo de número de serie, `fechaVencimiento` no ISO 8601). |
| `UNIT_NOT_FOUND` | La unidad (GTIN+serie) no existe en el world state. |
| `UNIT_ALREADY_EXISTS` | Alta de una unidad cuya clave compuesta ya existe. |
| `INVALID_STATE_TRANSITION` | El estado actual de la unidad no admite la transición solicitada (ADR-001). |
| `UNAUTHORIZED_CUSTODIAN` | El invocador no es el custodio actual de la unidad. |
| `UNAUTHORIZED_ROLE` | Falta el atributo `snt.role` o su valor no habilita la operación (DES-6). |
| `UNAUTHORIZED_AGENT_TYPE` | El `agentType` del invocador no puede ejecutar esta operación. |
| `ORG_NOT_REGISTERED` | El `mspId` del invocador o del destino no existe en el registro (ADR-003). |
| `ORG_INACTIVE` | La organización existe en el registro pero `active` es falso. |
| `TRANSFER_NOT_AUTHORIZED` | El par origen → destino no está permitido por la matriz de DES-3. |
| `INVALID_DESTINATION` | El destino declarado no existe, está inactivo o su `agentType` es incompatible. |
| `NOT_IN_TRANSIT` | Se intentó recibir o rechazar una unidad que no está en `EN_TRANSITO`. |
| `RECEIVER_MISMATCH` | El invocador de la recepción/rechazo no coincide con el destinatario declarado en la PDC de la operación (ADR-004). |
| `REGULATORY_ONLY` | La operación exige `AnmatMSP` (o coendoso regulatorio) y el invocador no lo satisface. |
| `INTERNAL_ERROR` | Error no clasificable atribuible al chaincode o a la plataforma. |

## Operaciones de escritura ordinarias

### `RegisterUnit`

```go
func (c *SNTContract) RegisterUnit(ctx contractapi.TransactionContextInterface, req RegisterUnitRequest) (*MedicationUnitView, error)
```

- **Transición**: T01. Estado resultante: `EN_LABORATORIO`.
- **Autorización**: `agentType=LABORATORY`, `active=true`, `snt.role=operator`.
- **Endoso**: laboratorio invocante (DES-6).
- **Request**:

```json
{
  "gtin": "07791234567890",
  "numeroSerie": "SN-0001-ABCD",
  "lote": "L2026-014",
  "fechaVencimiento": "2027-12-31"
}
```

- **Response**: `MedicationUnitView` con `custodioActual` resuelto al laboratorio invocante y `estado=EN_LABORATORIO`.
- **Errores**: `INVALID_REQUEST`, `UNIT_ALREADY_EXISTS`, `UNAUTHORIZED_AGENT_TYPE`, `UNAUTHORIZED_ROLE`, `ORG_NOT_REGISTERED`, `ORG_INACTIVE`.

### `DispatchTransfer`

```go
func (c *SNTContract) DispatchTransfer(ctx contractapi.TransactionContextInterface, req DispatchTransferRequest) (*MedicationUnitView, error)
```

- **Transición**: T02 (desde `EN_LABORATORIO`) o T03 (desde `EN_CUSTODIA`). Estado resultante: `EN_TRANSITO`.
- **Autorización**: invocador = custodio actual, `active=true`, `snt.role=operator`; par origen → destino permitido por DES-3.
- **Endoso**: origen y destino de la transferencia (DES-6).
- **Request** (público): solo referencia la unidad. El destino **no** viaja acá — ver `transient` abajo (ADR-004).

```json
{
  "gtin": "07791234567890",
  "numeroSerie": "SN-0001-ABCD"
}
```

- **Transient** (privado → PDC), clave `destinatario`: el destino se identifica por su identificador canónico o su `mspId`.

```json
{
  "destino": "GLN:7791234500017"
}
```

- **Transient** (privado → PDC), clave `commercial`:

```json
{
  "numeroRemito": "R-0001-2026",
  "numeroFactura": "A-0001-00001234",
  "cantidad": 1
}
```

- **Response**: `MedicationUnitView` con `estado=EN_TRANSITO` y `custodioActual` sin cambios (el emisor). El destino declarado no se refleja en la vista pública (ADR-004); el cliente que lo declaró ya lo conoce.
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `UNAUTHORIZED_CUSTODIAN`, `UNAUTHORIZED_ROLE`, `INVALID_STATE_TRANSITION`, `TRANSFER_NOT_AUTHORIZED`, `INVALID_DESTINATION`.

### `ReceiveTransfer`

```go
func (c *SNTContract) ReceiveTransfer(ctx contractapi.TransactionContextInterface, req UnitRefRequest) (*MedicationUnitView, error)
```

- **Transición**: T04. Estado resultante: `EN_CUSTODIA`.
- **Autorización**: invocador = destinatario declarado en la PDC de la operación (ADR-004), `active=true`, `snt.role=operator`.
- **Endoso**: origen y destino (DES-6).
- **Request**: `UnitRefRequest` (`gtin`, `numeroSerie`). Puede acompañarse de `transient` clave `commercial` con la confirmación documental de recepción.
- **Response**: `MedicationUnitView` con `custodioActual` = el receptor y `estado=EN_CUSTODIA`.
- **Errores**: `UNIT_NOT_FOUND`, `NOT_IN_TRANSIT`, `RECEIVER_MISMATCH`, `UNAUTHORIZED_ROLE`.

### `RejectTransfer`

```go
func (c *SNTContract) RejectTransfer(ctx contractapi.TransactionContextInterface, req UnitEventRequest) (*MedicationUnitView, error)
```

- **Transición**: T05. Estado resultante: `DEVUELTO`.
- **Autorización**: invocador = destinatario declarado en la PDC de la operación (ADR-004) o custodio actual (emisor), `snt.role=operator`.
- **Endoso**: origen y destino (DES-6).
- **Request**: `UnitEventRequest` (`gtin`, `numeroSerie`, `motivo`).
- **Response**: `MedicationUnitView` con `estado=DEVUELTO` y `custodioActual` = el emisor (la unidad rechazada vuelve al remitente).
- **Errores**: `UNIT_NOT_FOUND`, `NOT_IN_TRANSIT`, `UNAUTHORIZED_ROLE`, `INVALID_REQUEST`.

### `Dispense`

```go
func (c *SNTContract) Dispense(ctx contractapi.TransactionContextInterface, req UnitRefRequest) (*MedicationUnitView, error)
```

- **Transición**: T06. Estado resultante: `DISPENSADO`.
- **Autorización**: invocador = custodio actual con `agentType=PHARMACY` o `HEALTHCARE_FACILITY`, `snt.role=operator`.
- **Endoso**: organización dispensadora (DES-6).
- **Request**: `UnitRefRequest` (`gtin`, `numeroSerie`). **No** se envían datos del paciente (Ley 25.326; ADR-005; CC-4).
- **Response**: `MedicationUnitView` con `estado=DISPENSADO`.
- **Errores**: `UNIT_NOT_FOUND`, `UNAUTHORIZED_CUSTODIAN`, `UNAUTHORIZED_AGENT_TYPE`, `INVALID_STATE_TRANSITION`.

## Operaciones de eventos extraordinarios y de resolución

Todas comparten la firma y el request `UnitEventRequest`, y devuelven `MedicationUnitView`:

```go
func (c *SNTContract) <Nombre>(ctx contractapi.TransactionContextInterface, req UnitEventRequest) (*MedicationUnitView, error)
```

```json
// UnitEventRequest
{
  "gtin": "07791234567890",
  "numeroSerie": "SN-0001-ABCD",
  "motivo": "Texto libre que documenta la causa del evento."
}
```

La diferencia entre operaciones está en la transición ADR-001, el estado resultante, el actor habilitado y el endoso:

| Función | Transiciones | Estado resultante | Actor habilitado | Endoso (DES-6) |
|---|---|---|---|---|
| `Quarantine` | T07, T08, T09 | `EN_CUARENTENA` | Custodio actual o ANMAT | Custodio actual; ANMAT si lo inicia ANMAT |
| `ReleaseQuarantine` | T10 | `EN_CUSTODIA` | Custodio actual o ANMAT | Custodio actual; ANMAT si lo inicia ANMAT |
| `ReportExpired` | T11, T12, T13 | `VENCIDO` | Custodio actual o ANMAT | Custodio actual |
| `ReportStolen` | T14 | `ROBADO` | Custodio actual o ANMAT | Custodio actual |
| `ReportLost` | T15 | `EXTRAVIADO` | Custodio actual o ANMAT | Custodio actual |
| `ReportDamaged` | T16 | `DETERIORADO` | Custodio actual o ANMAT | Custodio actual |
| `WithdrawFromMarket` | T17, T18, T19 | `RETIRADO_MERCADO` | ANMAT o laboratorio titular | Custodio actual; **laboratorio no custodio + `AnmatMSP`** (DES-6) |
| `ProhibitProduct` | T20 | `PROHIBIDO` | Solo ANMAT | `AnmatMSP` (`REGULATORY_ONLY`) |
| `ReturnProduct` | T21, T22, T23, T24 | `DEVUELTO` | Custodio actual (o ANMAT según origen) | Custodio actual |
| `Restock` | T25, T26, T27 | `EN_CUSTODIA` | Agente de recupero, custodio o ANMAT según origen | Custodio actual; ANMAT si el origen es `RETIRADO_MERCADO` |
| `FinalDisposition` | T28, T29, T30, T31, T32, T33 | `DISPUESTO_FINAL` | Agente de recupero/disposición, ANMAT o laboratorio según origen | Custodio actual; ANMAT en disposiciones regulatorias |

Notas:

- El chaincode valida internamente que el estado de origen de la unidad admita la transición pedida; si no, devuelve `INVALID_STATE_TRANSITION`. La misma función cubre varios estados de origen (por ejemplo `ReportStolen` aplica a `EN_LABORATORIO`, `EN_TRANSITO`, `EN_CUSTODIA`, `EN_CUARENTENA` o `DEVUELTO`).
- Las operaciones que exigen ANMAT devuelven `REGULATORY_ONLY` si el invocador no satisface el rol o coendoso regulatorio.
- Errores comunes a todas: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `INVALID_STATE_TRANSITION`, `UNAUTHORIZED_CUSTODIAN`/`REGULATORY_ONLY`, `UNAUTHORIZED_ROLE`.

## Operaciones del registro organización-establecimiento

Gobernadas por ADR-003 y DES-6. Solo `AnmatMSP` con `snt.role=regulatory-admin`.

### `RegisterOrganization`

```go
func (c *SNTContract) RegisterOrganization(ctx contractapi.TransactionContextInterface, req RegisterOrganizationRequest) (*OrganizationView, error)
```

- **Autorización / endoso**: `AnmatMSP`, `snt.role=regulatory-admin`.
- **Request**:

```json
{
  "mspId": "FarmaciaDelSolMSP",
  "id": "7791234500017",
  "idType": "GLN",
  "agentType": "PHARMACY",
  "active": true
}
```

- **Response** (`OrganizationView`): los mismos campos persistidos.
- **Errores**: `INVALID_REQUEST` (por ejemplo `idType` distinto de `GLN`/`CUFE`, dígito verificador inválido, `agentType` fuera del catálogo), `REGULATORY_ONLY`.

### `SetOrganizationActive`

```go
func (c *SNTContract) SetOrganizationActive(ctx contractapi.TransactionContextInterface, req SetOrganizationActiveRequest) (*OrganizationView, error)
```

- **Autorización / endoso**: `AnmatMSP`, `snt.role=regulatory-admin`.
- **Request**:

```json
{ "mspId": "FarmaciaDelSolMSP", "active": false }
```

- **Response**: `OrganizationView` actualizado.
- **Errores**: `ORG_NOT_REGISTERED`, `REGULATORY_ONLY`, `INVALID_REQUEST`.

## Operaciones de lectura

No mutan estado, no generan endoso de escritura y se rigen por las políticas de visibilidad de lectura del canal y de las PDC (ADR-002). El organismo financiador (ADR-005) opera exclusivamente con estas operaciones sobre el estado público.

### `ReadUnit`

```go
func (c *SNTContract) ReadUnit(ctx contractapi.TransactionContextInterface, gtin string, numeroSerie string) (*MedicationUnitView, error)
```

- **Response**: `MedicationUnitView`.
- **Errores**: `UNIT_NOT_FOUND`, `INVALID_REQUEST`.

### `GetUnitHistory`

```go
func (c *SNTContract) GetUnitHistory(ctx contractapi.TransactionContextInterface, gtin string, numeroSerie string) ([]UnitHistoryEntry, error)
```

- **Response**: lista ordenada de entradas del historial (`GetHistoryForKey`):

```json
[
  {
    "txId": "a1b2c3...",
    "timestamp": "2026-08-13T14:32:10Z",
    "isDelete": false,
    "value": { "...": "MedicationUnitView en ese punto del historial" }
  }
]
```

- **Errores**: `UNIT_NOT_FOUND`, `INVALID_REQUEST`.

### `QueryUnitsByGTIN`

```go
func (c *SNTContract) QueryUnitsByGTIN(ctx contractapi.TransactionContextInterface, gtin string) ([]MedicationUnitView, error)
```

- Usa `GetStateByPartialCompositeKey` (`modelo-datos.md` §2.2). Recupera todas las unidades registradas bajo un GTIN.
- **Response**: lista de `MedicationUnitView` (posiblemente vacía).
- **Errores**: `INVALID_REQUEST`.

## Tipos de request compartidos

```json
// UnitRefRequest
{ "gtin": "string", "numeroSerie": "string" }

// UnitEventRequest
{ "gtin": "string", "numeroSerie": "string", "motivo": "string" }

// DispatchTransferRequest (público — no incluye destino, ver transient clave "destinatario")
{ "gtin": "string", "numeroSerie": "string" }

// DispatchTransferTransientDestinatario (transient, clave "destinatario")
{ "destino": "string (GLN:/CUFE:/mspId)" }

// RegisterUnitRequest
{ "gtin": "string", "numeroSerie": "string", "lote": "string", "fechaVencimiento": "string (ISO 8601)" }
```

## Política de versionado y congelamiento

- Este contrato es la fuente de verdad de la interfaz pública del chaincode `snt`. Mientras no esté integrado a la rama principal, el cliente y la baseline no deben construirse contra él.
- `Versión del contrato` sigue semver:
  - **PATCH**: correcciones de redacción o de `message`/`details` de errores, sin alterar firmas, `code`s ni esquemas.
  - **MINOR**: agregados compatibles (nueva operación, nuevo campo opcional en un request, nuevo `code`).
  - **MAJOR**: cambio incompatible (renombrar o quitar una función o campo, cambiar un tipo, cambiar la semántica de un `code`). Exige un PR etiquetado `breaking-change`.
- Todo cambio a este documento requiere aprobación explícita de B antes del merge, según la story DES-5.
- Dependencias de merge pendientes: este contrato asume ADR-004 (transferencia en dos operaciones, destinatario declarado en PDC) y ADR-005 (financiador de solo lectura), cuyos PRs aún no están en `develop`. Si alguna de esas decisiones cambiara antes de integrarse, las operaciones de transferencia o la nota del financiador deben revisarse aquí.
- **Historial de cambios incompatibles**: `2.0.0` — el destino de `DispatchTransfer` pasa de argumento público a `transient` (clave `destinatario`), y `destinatarioPendiente` se elimina de `MedicationUnitView`, para alinear el contrato con la revisión de ADR-004 que clasifica el destinatario declarado como dato privado (PDC), no público. Pendiente de aprobación explícita de B antes de mergear, conforme a esta misma política.
```

