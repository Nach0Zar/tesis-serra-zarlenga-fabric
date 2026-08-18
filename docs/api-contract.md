# Contrato de interfaz del chaincode `snt`

- **Versión del contrato**: `2.2.0`
- **Estado**: Congelado. Los cambios se rigen por la política de versionado (última sección): un cambio incompatible exige un PR etiquetado `breaking-change` y aprobación explícita de B (el integrante responsable de cliente y baseline, conforme la issue #11 / DES-5).
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
| [ADR-004](adr/004-transfer-dispatch-reception.md) | Decisión vigente (integrada en `develop`). La transferencia son dos operaciones: `DispatchTransfer` y `ReceiveTransfer`, más `RejectTransfer`. El destinatario declarado viaja por `transient` y se valida contra el registro de la operación activa en PDC, nunca como argumento público ni campo de `MedicationUnitView`. |
| [ADR-005](adr/005-rol-organismo-financiador.md) | Decisión vigente (integrada en `develop`). El organismo financiador solo invoca operaciones de lectura. |
| [ADR-009](adr/009-return-and-recovery-semantics.md) | La devolución es un evento único que no cambia `CustodioActual`; `RejectTransfer` cubre T05 y `ReturnProduct` T21–T24. El receptor de la devolución, cuando se declara, viaja por `transient` y va a PDC. `RECOVERY_OR_DISPOSAL_AGENT` se resuelve como el custodio actual registrado. |
| [ADR-010](adr/010-non-custodial-identity.md) | La identidad de ANMAT y de los financiadores se resuelve por el registro (`agentType` `REGULATOR`/`FINANCIER`, `idType` `REG`), nunca por el nombre de la MSP. Las operaciones `REGULATORY_ONLY` exigen `agentType=REGULATOR` activo con `snt.role=regulatory-admin`. |
| [ADR-011](adr/011-financier-trace-verification.md) | Semántica de `VerifyTrace`: checklist determinística de cinco comprobaciones y veredicto estructurado. |
| [ADR-006](adr/006-private-data-collections.md) | Colección explícita por par de organizaciones, resuelta determinísticamente por el chaincode. Su política de endoso `OR(org A, org B)` hace que un cierre regulatorio del tránsito exija también el endoso de una de las dos partes. |
| [ADR-007](adr/007-network-topology.md) | Materialización del endoso por SBE y sus límites. De ahí salen dos operaciones de este contrato: `Init` (bootstrap regulatorio en dos secuencias de lifecycle) y `AuthorizeLabIntervention` (autorización previa que un laboratorio no custodio debe consumir). |
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
- El campo `motivo` de `UnitEventRequest` es un argumento **público** del canal: no debe incluir datos personales, clínicos ni información comercial (precios, cantidades, referencias de facturación) — para eso existe el `transient` `commercial`. `motivo` documenta la causa regulatoria del evento en texto breve y neutro.

### Vista pública de la unidad (`MedicationUnitView`)

Es el response de todas las operaciones de escritura sobre una unidad y de `ReadUnit`. Refleja el estado público del canal (`modelo-datos.md` §3). **No incluye** el destinatario declarado durante una transferencia en curso: ADR-004 decidió que ese dato es privado y vive en PDC, no en el struct público — ver "Datos privados (ADR-002, ADR-004)".

```json
{
  "gtin": "07791234567898",
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
| `RECEIVER_MISMATCH` | El invocador de la recepción/rechazo no coincide con el destinatario declarado en el registro de la operación **activa** — la creada por el último despacho, mientras la unidad permanece en `EN_TRANSITO`; nunca se valida contra registros de operaciones cerradas (ADR-004, "Ciclo de vida del registro de operación"). |
| `REGULATORY_ONLY` | La operación exige `AnmatMSP` (o coendoso regulatorio) y el invocador no lo satisface. |
| `LAST_ACTIVE_REGULATOR` | Se intentó desactivar la única entrada `REGULATOR` activa del registro (ADR-010). |
| `ALREADY_INITIALIZED` | Se reinvocó `Init` sobre un chaincode cuyo registro ya contiene la entrada `REGULATOR` (ADR-010). |
| `INVALID_LAB_INTERVENTION` | La autorización de intervención solicitada no es válida: el laboratorio designado no tiene `agentType=LABORATORY`, la `operacion` está fuera del catálogo o `expiraEn` no es posterior al timestamp de la transacción (ADR-007, punto 6.e). |
| `LAB_INTERVENTION_REQUIRED` | Un laboratorio no custodio intentó un retiro, recupero o disposición final sin una autorización de intervención vigente para esa unidad, ese laboratorio y esa operación (DES-6; ADR-007, punto 6.e). |
| `INTERNAL_ERROR` | Error no clasificable atribuible al chaincode o a la plataforma. |

## Operación de inicialización

### `Init`

```go
func (c *SNTContract) Init(ctx contractapi.TransactionContextInterface) (*OrganizationView, error)
```

Siembra la primera entrada `REGULATOR` del registro organización-establecimiento. Es la operación que resuelve el arranque en frío que describe [ADR-010](adr/010-non-custodial-identity.md), punto 4: `RegisterOrganization` exige un regulador ya registrado, y esta entrada no puede haber sido registrada por nadie.

- **Se invoca una sola vez**, en la secuencia 1 del lifecycle, con `--init-required` y política de endoso `AND` de todas las organizaciones fundacionales del canal (ADR-007, punto 5).
- **No recibe argumentos.** En particular, **no** recibe el `mspId` regulatorio: aceptarlo como parámetro dejaría la identidad del regulador a criterio de quien envía la propuesta. El chaincode la resuelve exigiendo, de forma conjunta:
  1. que `cid.GetMSPID()` del invocador coincida con el `mspId` declarado como `REGULATOR` en el **manifiesto fundacional embebido** en el paquete del chaincode (`go:embed`, mismo mecanismo que la matriz de ADR-008);
  2. que el invocador porte `snt.role=regulatory-admin`;
  3. que no exista todavía ninguna entrada `REGULATOR` en el registro.
- **Efectos**: escribe la entrada `REGULATOR` (`idType=REG`) y le fija una política de endoso por clave (SBE) que exige a la organización regulatoria, de modo que nadie más pueda modificarla después del bootstrap.
- **Response**: `OrganizationView` de la entrada sembrada.
- **Errores**: `REGULATORY_ONLY` (el invocador no es el MSP declarado en el manifiesto, o no porta `snt.role=regulatory-admin`), `ALREADY_INITIALIZED`, `INTERNAL_ERROR`.

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
  "gtin": "07791234567898",
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
  "gtin": "07791234567898",
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
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `UNAUTHORIZED_CUSTODIAN`, `UNAUTHORIZED_ROLE`, `ORG_NOT_REGISTERED`, `ORG_INACTIVE`, `INVALID_STATE_TRANSITION`, `TRANSFER_NOT_AUTHORIZED`, `INVALID_DESTINATION`.

### `ReceiveTransfer`

```go
func (c *SNTContract) ReceiveTransfer(ctx contractapi.TransactionContextInterface, req UnitRefRequest) (*MedicationUnitView, error)
```

- **Transición**: T04. Estado resultante: `EN_CUSTODIA`.
- **Autorización**: invocador = destinatario declarado en el registro de la operación **activa** (ADR-004, "Ciclo de vida del registro de operación"; nunca contra operaciones cerradas), `active=true`, `snt.role=operator`.
- **Endoso**: origen y destino (DES-6).
- **Request**: `UnitRefRequest` (`gtin`, `numeroSerie`). Puede acompañarse de `transient` clave `commercial` con la confirmación documental de recepción.
- **Response**: `MedicationUnitView` con `custodioActual` = el receptor y `estado=EN_CUSTODIA`.
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `NOT_IN_TRANSIT`, `RECEIVER_MISMATCH`, `UNAUTHORIZED_ROLE`, `ORG_NOT_REGISTERED`, `ORG_INACTIVE`.

### `RejectTransfer`

```go
func (c *SNTContract) RejectTransfer(ctx contractapi.TransactionContextInterface, req UnitEventRequest) (*MedicationUnitView, error)
```

- **Transición**: T05. Estado resultante: `DEVUELTO`.
- **Autorización**: invocador = destinatario declarado en el registro de la operación **activa** (ADR-004; nunca contra operaciones cerradas) o custodio actual (emisor), `snt.role=operator`.
- **Endoso**: origen y destino (DES-6).
- **Request**: `UnitEventRequest` (`gtin`, `numeroSerie`, `motivo`).
- **Response**: `MedicationUnitView` con `estado=DEVUELTO` y `custodioActual` = el emisor, que permanece sin cambios por convención de ADR-004; T05 no registra que el retorno físico al remitente haya ocurrido (la resolución posterior de `DEVUELTO` se rige por ADR-001 y EXT-4).
- **Errores**: `UNIT_NOT_FOUND`, `NOT_IN_TRANSIT`, `RECEIVER_MISMATCH` (el invocador no es ni el destinatario declarado ni el emisor), `UNAUTHORIZED_ROLE`, `INVALID_REQUEST`.

### `Dispense`

```go
func (c *SNTContract) Dispense(ctx contractapi.TransactionContextInterface, req UnitRefRequest) (*MedicationUnitView, error)
```

- **Transición**: T06. Estado resultante: `DISPENSADO`.
- **Autorización**: invocador = custodio actual con `agentType=PHARMACY` o `HEALTHCARE_FACILITY`, `snt.role=operator`.
- **Endoso**: organización dispensadora (DES-6).
- **Request**: `UnitRefRequest` (`gtin`, `numeroSerie`). **No** se envían datos del paciente (Ley 25.326; ADR-005; CC-4).
- **Response**: `MedicationUnitView` con `estado=DISPENSADO`.
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `UNAUTHORIZED_CUSTODIAN`, `UNAUTHORIZED_AGENT_TYPE`, `UNAUTHORIZED_ROLE`, `INVALID_STATE_TRANSITION`.

## Operaciones de eventos extraordinarios y de resolución

Todas comparten la firma y el request `UnitEventRequest`, y devuelven `MedicationUnitView`:

```go
func (c *SNTContract) <Nombre>(ctx contractapi.TransactionContextInterface, req UnitEventRequest) (*MedicationUnitView, error)
```

```json
// UnitEventRequest
{
  "gtin": "07791234567898",
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
| `WithdrawFromMarket` | T17, T18, T19 | `RETIRADO_MERCADO` | ANMAT o laboratorio titular | Custodio actual; **laboratorio no custodio: previa `AuthorizeLabIntervention` + endoso de `AnmatMSP`** (DES-6; ADR-007, punto 6.e) |
| `ProhibitProduct` | T20 | `PROHIBIDO` | Solo ANMAT | `AnmatMSP` (`REGULATORY_ONLY`) |
| `ReturnProduct` | T21, T22, T23, T24 | `DEVUELTO` | Custodio actual (o ANMAT según origen) | Custodio actual |
| `Restock` | T25, T26, T27 | `EN_CUSTODIA` | Agente de recupero, custodio o ANMAT según origen | Custodio actual; ANMAT si el origen es `RETIRADO_MERCADO`; laboratorio no custodio: previa `AuthorizeLabIntervention` + endoso de `AnmatMSP` |
| `FinalDisposition` | T28, T29, T30, T31, T32, T33 | `DISPUESTO_FINAL` | Agente de recupero/disposición, ANMAT o laboratorio según origen | Custodio actual; ANMAT en disposiciones regulatorias; laboratorio no custodio: previa `AuthorizeLabIntervention` + endoso de `AnmatMSP` |

Notas:

- **`ReturnProduct` admite un `transient` opcional** con la clave `devolucion`, para declarar el receptor de la devolución (ADR-009). Como todo identificador de contraparte, **no** viaja como argumento público: revela una relación no consumada. Se persiste en la PDC del par junto al resto del registro de la operación (ADR-006) y **no** modifica `custodioActual`, que permanece en el custodio declarante.

```json
// transient, clave "devolucion"
{ "receptor": "GLN:7791234500017" }
```

  Cuando el `transient` viene, el chaincode valida el receptor **antes** de resolver el nombre de la colección (ADR-009, punto 2), en este orden:

  | # | Validación | Código |
  |---|---|---|
  | 1 | Identificador con forma canónica `GLN:`/`CUFE:`. | `INVALID_REQUEST` |
  | 2 | El receptor existe en el registro. | `ORG_NOT_REGISTERED` |
  | 3 | El receptor está activo. | `ORG_INACTIVE` |
  | 4 | Su `agentType` es custodial. | `INVALID_DESTINATION` |
  | 5 | No es la propia organización declarante. | `INVALID_DESTINATION` |
  | 6 | El par «`agentType` del receptor → `agentType` del custodio declarante» está autorizado por la matriz de DES-3. | `TRANSFER_NOT_AUTHORIZED` |

  La validación 6 es la que garantiza que la colección del par exista (ADR-006, punto 1). Sin ella el chaincode resolvería el nombre de una colección inexistente y la operación fallaría con un error de plataforma en lugar de con un código de este contrato. **No** se exige que el receptor sea el proveedor real de esa unidad: ver ADR-009, punto 2, "Qué no se exige en v1, y por qué".

- **Eventos extraordinarios sobre una unidad en `EN_TRANSITO`**: cuando la unidad está en tránsito, el evento cierra además el registro de operación en la PDC del par (`DelPrivateData`, ADR-006, punto 4). Esa escritura privada debe satisfacer la política de endoso de la colección, `OR(org emisora, org receptora)`, con lo que un evento iniciado por ANMAT en esa ventana **no** es una transacción de un solo endoso: exige también el peer de una de las dos organizaciones de la operación pendiente.

- **Intervención de un laboratorio no custodio** (`WithdrawFromMarket`, `Restock`, `FinalDisposition`): cuando el invocador es una organización con `agentType=LABORATORY` que **no** es el custodio actual, el chaincode exige una autorización de intervención vigente (`AuthorizeLabIntervention`) para esa unidad, ese laboratorio y esa operación, y la **consume** borrándola. Como esa clave está protegida por SBE que exige a la organización regulatoria, la transacción debe llevar su endoso: es así como se materializa el par «laboratorio invocante + `AnmatMSP`» que pide DES-6 (ADR-007, punto 6.e). Sin autorización vigente, `LAB_INTERVENTION_REQUIRED`.

- El chaincode valida internamente que el estado de origen de la unidad admita la transición pedida; si no, devuelve `INVALID_STATE_TRANSITION`. La misma función cubre varios estados de origen (por ejemplo `ReportStolen` aplica a `EN_LABORATORIO`, `EN_TRANSITO`, `EN_CUSTODIA`, `EN_CUARENTENA` o `DEVUELTO`).
- Las operaciones que exigen ANMAT devuelven `REGULATORY_ONLY` si el invocador no satisface el rol o coendoso regulatorio.
- Errores comunes a todas: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `INVALID_STATE_TRANSITION`, `UNAUTHORIZED_CUSTODIAN`/`REGULATORY_ONLY`, `UNAUTHORIZED_ROLE`.

### `AuthorizeLabIntervention`

```go
func (c *SNTContract) AuthorizeLabIntervention(ctx contractapi.TransactionContextInterface, req AuthorizeLabInterventionRequest) (*LabInterventionView, error)
```

Autoriza a un laboratorio titular a ejecutar **una** operación extraordinaria sobre una unidad que está bajo custodia de un tercero. Existe porque el par de endosos que DES-6 exige para ese caso no puede imponerse con SBE sobre la clave de la unidad: la política de una clave se evalúa contra el estado previo y no puede condicionarse a la operación intentada (ADR-007, punto 6).

- **Autorización**: `agentType=REGULATOR`, `active=true`, `snt.role=regulatory-admin`.
- **Request**:

```json
{
  "gtin": "07791234567898",
  "numeroSerie": "SN-0001-ABCD",
  "laboratorio": "GLN:7791234500017",
  "operacion": "WITHDRAW_FROM_MARKET",
  "motivo": "Retiro de lote solicitado por el titular, expediente ANMAT XXXX/2026.",
  "expiraEn": "2026-09-30T00:00:00Z"
}
```

- **Valores de `operacion`**: `WITHDRAW_FROM_MARKET`, `RESTOCK`, `FINAL_DISPOSITION` — las tres operaciones que DES-6 habilita a un laboratorio no custodio.
- **Validaciones**: la unidad existe; `laboratorio` está registrado, activo y con `agentType=LABORATORY`; `operacion` pertenece al catálogo; `expiraEn` es ISO 8601 y **posterior** al `GetTxTimestamp()` de la transacción.
- **Efectos**: escribe la clave pública `LabIntervention`+[`gtin`,`numeroSerie`] y le fija una política de endoso por clave (SBE) que exige a la organización regulatoria. Si ya existía una autorización para esa unidad, la reemplaza.
- **Consumo**: la operación del laboratorio la lee, verifica que coincidan laboratorio y operación y que no haya expirado, y la **borra**. Como la clave está protegida por SBE regulatoria, ese borrado obliga a que la transacción del laboratorio lleve el endoso de la organización regulatoria.
- **Response** (`LabInterventionView`): los campos persistidos más el `mspId` regulatorio que la emitió y el timestamp de emisión.
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `ORG_NOT_REGISTERED`, `ORG_INACTIVE`, `INVALID_LAB_INTERVENTION`, `REGULATORY_ONLY`.
- **Fuera de alcance en v1**: no existe operación de revocación anticipada. El riesgo de una autorización pendiente se acota con `expiraEn`, que la autoridad regulatoria fija tan corto como el caso lo requiera. Agregar `RevokeLabIntervention` sería un cambio MINOR si un requisito lo pidiera.

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

- **Valores admitidos** (ADR-003 para custodiales, ADR-010 para no custodiales):
  - `idType`: `GLN` o `CUFE` para organizaciones custodiales — `id` de 13 dígitos con dígito verificador GS1 válido —; `REG` para organizaciones no custodiales, donde `id` es un slug estable del organismo (`ANMAT`, `INSSJP-PAMI`).
  - `agentType`: `LABORATORY`, `DISTRIBUTOR`, `LOGISTICS_OPERATOR`, `DRUGSTORE`, `PHARMACY`, `HEALTHCARE_FACILITY` (custodiales, catálogo de DES-3) y `REGULATOR`, `FINANCIER` (no custodiales, ADR-010). Los `agentType` no custodiales nunca son origen ni destino válidos de una transferencia ni pueden persistirse como `custodioActual`.
  - La combinación `idType=REG` solo es válida con `agentType` no custodial, y viceversa.
- **Invariante de unicidad** (ADR-010): se rechaza el alta de una segunda entrada `REGULATOR` mientras exista una activa. La primera entrada `REGULATOR` no se crea con esta operación sino en la inicialización del chaincode, bajo la política de endoso estricta de la secuencia de bootstrap.
- **Response** (`OrganizationView`): los mismos campos persistidos.
- **Errores**: `INVALID_REQUEST` (por ejemplo `idType` fuera de `GLN`/`CUFE`/`REG`, dígito verificador inválido, `agentType` fuera del catálogo, combinación `idType`/`agentType` incoherente, o segunda entrada `REGULATOR`), `REGULATORY_ONLY`.

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
- **Invariante** (ADR-010): no puede desactivarse la única entrada `REGULATOR` activa; la red no debe quedar sin autoridad capaz de administrar el registro.
- **Errores**: `ORG_NOT_REGISTERED`, `REGULATORY_ONLY`, `INVALID_REQUEST`, `LAST_ACTIVE_REGULATOR`.

## Operaciones de lectura

No mutan estado, no generan endoso de escritura y se rigen por las políticas de visibilidad de lectura del canal y de las PDC (ADR-002). El organismo financiador (ADR-005) opera exclusivamente con operaciones de lectura; su flujo de verificación claim-driven por serial usa `VerifyTrace`, que encapsula la checklist de ADR-011, y puede apoyarse en `ReadUnit` y `GetUnitHistory` para inspeccionar el detalle. `QueryUnitsByGTIN` no forma parte de ese flujo, aunque le resulta técnicamente accesible: ADR-005 reconoce que el acceso de lectura al estado público del canal no puede restringirse por chaincode (supuesto de confianza del prototipo).

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

### `VerifyTrace`

```go
func (c *SNTContract) VerifyTrace(ctx contractapi.TransactionContextInterface, gtin string, numeroSerie string) (*TraceVerdict, error)
```

Verificación de trazabilidad de una unidad dispensada, con la semántica que fija [ADR-011](adr/011-financier-trace-verification.md). Es la operación con la que el organismo financiador satisface su condición de pago, y la misma que ANMAT puede usar para auditoría.

- **Autorización**: `agentType=FINANCIER` con `snt.role=financier-auditor`, o `agentType=REGULATOR` con `snt.role=auditor` o `regulatory-admin` (ADR-010). No muta estado.
- **Comprobaciones**, evaluadas en orden: existencia de la unidad, estado `DISPENSADO`, dispensador con `agentType` habilitado, secuencia de estados que corresponde a un camino válido de ADR-001, y cada cambio de custodio autorizado por la matriz de DES-3.
- **Response** (`TraceVerdict`): `legitima` es `true` solo si las cinco comprobaciones pasan; `motivo` nombra la primera que falla.

```json
{
  "legitima": false,
  "motivo": "TRANSFERENCIA_NO_AUTORIZADA",
  "verificaciones": [
    { "check": "EXISTENCIA", "resultado": "OK", "detalle": "" },
    { "check": "ESTADO_DISPENSADO", "resultado": "OK", "detalle": "" },
    { "check": "DISPENSADOR_HABILITADO", "resultado": "OK", "detalle": "" },
    { "check": "SECUENCIA_ESTADOS", "resultado": "OK", "detalle": "" },
    { "check": "PARES_AUTORIZADOS", "resultado": "FALLO", "detalle": "PHARMACY -> DRUGSTORE" }
  ]
}
```

- **Valores de `motivo`**: `NO_ENCONTRADA`, `NO_DISPENSADA`, `DISPENSADOR_INVALIDO`, `SECUENCIA_INVALIDA`, `TRANSFERENCIA_NO_AUTORIZADA`; vacío cuando `legitima` es `true`.
- **Valores de `resultado`**: `OK`, `FALLO`, `NO_EVALUADO` (comprobaciones posteriores a la que falló).
- **Nota**: la inexistencia de la unidad **no** es un error sino el veredicto `NO_ENCONTRADA`, porque para el financiador es una respuesta legítima de su consulta, no una falla de invocación.
- **Límites declarados** (ADR-011): la verificación no valida la habilitación histórica de los actores, no distingue versiones históricas de la matriz, no ve transacciones rechazadas y no puede comprobar que el serial corresponda a un afiliado del financiador invocante.
- **Errores**: `INVALID_REQUEST`, `UNAUTHORIZED_ROLE`, `UNAUTHORIZED_AGENT_TYPE` (el invocador está registrado y activo pero su `agentType` no es `FINANCIER` ni `REGULATOR`), `ORG_NOT_REGISTERED`, `ORG_INACTIVE`.

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

// ReturnProductTransientDevolucion (transient, clave "devolucion", opcional)
{ "receptor": "string (GLN:/CUFE:)" }

// AuthorizeLabInterventionRequest
{
  "gtin": "string",
  "numeroSerie": "string",
  "laboratorio": "string (GLN:/CUFE:)",
  "operacion": "WITHDRAW_FROM_MARKET | RESTOCK | FINAL_DISPOSITION",
  "motivo": "string",
  "expiraEn": "string (ISO 8601, posterior al timestamp de la transacción)"
}

// LabInterventionView
{
  "gtin": "string",
  "numeroSerie": "string",
  "laboratorio": "string (GLN:/CUFE:)",
  "operacion": "string",
  "motivo": "string",
  "expiraEn": "string (ISO 8601)",
  "emitidaPor": "string (mspId de la organización regulatoria)",
  "emitidaEn": "string (ISO 8601)"
}
```

## Política de versionado y congelamiento

- Este contrato es la fuente de verdad de la interfaz pública del chaincode `snt`. Mientras no esté integrado a la rama principal, el cliente y la baseline no deben construirse contra él.
- `Versión del contrato` sigue semver:
  - **PATCH**: correcciones de redacción o de `message`/`details` de errores, sin alterar firmas, `code`s ni esquemas.
  - **MINOR**: agregados compatibles (nueva operación, nuevo campo opcional en un request, nuevo `code`).
  - **MAJOR**: cambio incompatible (renombrar o quitar una función o campo, cambiar un tipo, cambiar la semántica de un `code`). Exige un PR etiquetado `breaking-change`.
- Todo cambio a este documento requiere aprobación explícita de B antes del merge, según la story DES-5.
- Este contrato implementa ADR-004 (transferencia en dos operaciones, destinatario declarado en PDC) y ADR-005 (financiador de solo lectura), ambas decisiones vigentes integradas en `develop`. Si alguna se revisara mediante un ADR posterior, las operaciones de transferencia o la nota del financiador deben revisarse aquí.
- **Historial de cambios incompatibles**: `2.0.0` — el destino de `DispatchTransfer` pasa de argumento público a `transient` (clave `destinatario`), y `destinatarioPendiente` se elimina de `MedicationUnitView`, para alinear el contrato con la revisión de ADR-004 que clasifica el destinatario declarado como dato privado (PDC), no público.
- **Historial de cambios compatibles**: `2.2.0` — incorpora la superficie que exige la corrección de ADR-007 sobre los límites del endoso basado en estado, y cierra las reglas que ADR-009 dejaba abiertas: nueva operación `Init`, sin argumentos, que siembra la entrada `REGULATOR` resolviendo la identidad del regulador contra el manifiesto fundacional embebido en el paquete (ADR-010); nueva operación `AuthorizeLabIntervention`, sin la cual el par de endosos que DES-6 exige a un laboratorio no custodio no es materializable; seis validaciones tipificadas del receptor declarado en el `transient` `devolucion` de `ReturnProduct`; nota de endoso para los eventos extraordinarios sobre una unidad en `EN_TRANSITO`, que cierran el registro privado y por lo tanto exigen también el endoso de una de las dos partes; `UNAUTHORIZED_AGENT_TYPE` en la lista de errores de `VerifyTrace`; y los códigos nuevos `ALREADY_INITIALIZED`, `INVALID_LAB_INTERVENTION` y `LAB_INTERVENTION_REQUIRED`. Agregados compatibles: ninguna firma existente cambia y ningún `code` previo altera su semántica. `2.1.0` — incorpora al contrato la superficie pública que introdujeron ADR-009, ADR-010 y ADR-011, en lugar de diferirla a las issues de implementación: nueva operación de lectura `VerifyTrace` con su veredicto estructurado; `transient` opcional `devolucion` en `ReturnProduct`; valores admitidos de `agentType`/`idType` para organizaciones no custodiales en `RegisterOrganization`, con la invariante de unicidad del regulador; invariante de último regulador activo y nuevo `code` `LAST_ACTIVE_REGULATOR` en `SetOrganizationActive`. Agregados compatibles: ninguna firma existente cambia ni se altera la semántica de un `code` previo. `2.0.1` — corrige el dígito verificador GS1 del GTIN de los ejemplos (`07791234567890` → `07791234567898`; el valor anterior habría sido rechazado por la propia validación de `INVALID_REQUEST`), completa las listas de errores por operación para que toda condición de autorización declarada tenga su código (`DispatchTransfer`/`ReceiveTransfer`: `ORG_NOT_REGISTERED`/`ORG_INACTIVE`; `RejectTransfer`: `RECEIVER_MISMATCH`; `Dispense`: `UNAUTHORIZED_ROLE`), agrega el lineamiento sobre `motivo` y aclara quién es B. Sin cambios de firmas, `code`s del catálogo ni esquemas. `2.0.2` — la autorización de `ReceiveTransfer`/`RejectTransfer` y el error `RECEIVER_MISMATCH` validan contra el registro de la operación **activa** (nunca contra operaciones cerradas, conforme el ciclo de vida de ADR-004); la respuesta de `RejectTransfer` deja de afirmar un retorno físico consumado; se precisa que el flujo del financiador usa `ReadUnit`/`GetUnitHistory`; las notas de dependencia de merge pasan a describir ADR-004/ADR-005 como decisiones vigentes en `develop`. Sin cambios de firmas, `code`s ni esquemas.

