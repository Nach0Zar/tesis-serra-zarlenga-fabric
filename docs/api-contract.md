# Contrato de interfaz del chaincode `snt`

- **Versión del contrato**: `2.6.1`
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
| [ADR-006](adr/006-private-data-collections.md) | Colección explícita por par de organizaciones, resuelta determinísticamente por el chaincode. Su política de endoso `OR(org A, org B)` es una barrera adicional sobre las escrituras privadas; quien exige a **ambas** partes en un cierre regulatorio del tránsito es la política de la clave pública, `AND(emisor, receptor)` (ADR-007, punto 6.b). |
| [ADR-007](adr/007-network-topology.md) | Materialización del endoso por SBE y sus límites. De ahí salen tres operaciones de este contrato: `Init` (bootstrap regulatorio en dos secuencias de lifecycle), `AuthorizeLabIntervention` (autorización previa que un laboratorio no custodio debe consumir) y `RevokeLabIntervention` (la deja sin efecto). De ahí sale también la política de endoso de chaincode, `OR(custodiales, regulatoria)`, y con ella la regla de que las operaciones del registro se confirman con el solo endoso del peer regulatorio. |
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

### Marcadores de participación (ADR-007, punto 6)

Algunas operaciones escriben, además de su estado público, un **marcador de participación** en la colección implícita de una organización (`_implicit_org_<MSPID>`). Es un mecanismo de endoso, no de negocio:

- lo calcula y escribe el chaincode; **no** viaja por `transient`, no requiere nada del cliente y no aparece en ningún request ni response;
- su clave es `Participacion`+[`<tipoObjeto>`, `<componentes del objeto>`, `txId`], con dos variantes: `Participacion`+[`Unidad`,`gtin`,`numeroSerie`,`txId`] para las operaciones sobre una unidad y `Participacion`+[`Organizacion`,`mspId`,`txId`] para las del registro organización-establecimiento. El `txId` va último: hace la clave única por transacción —sin contención MVCC— y deja los componentes anteriores disponibles para consultas por clave compuesta parcial;
- su única función es someter la transacción a la política de endoso de esa colección, que pertenece a la organización dueña y rige desde el despliegue — con lo que el endoso de su peer queda **exigido por la plataforma**, incluso en la primera escritura de una clave, donde el endoso basado en estado no puede aplicarse;
- **qué implica para el cliente**: debe recolectar el endoso de los peers de todas las organizaciones que la operación involucra. La columna «endoso» de cada operación las enumera.

Operaciones que lo escriben: `RegisterUnit` (colección del laboratorio invocante), todo evento extraordinario iniciado por la organización regulatoria (su colección), la intervención de un laboratorio no custodio (la del laboratorio y la regulatoria) y las operaciones que ejecuta la organización regulatoria — `RegisterOrganization`, `SetOrganizationActive`, `AuthorizeLabIntervention` y `RevokeLabIntervention` — en su colección.

### Política de endoso de chaincode

La definición operativa del chaincode lleva la política `OR(<organizaciones custodiales>, <organización regulatoria>)` (ADR-007, punto 6.j). Rige **solo la primera escritura de una clave pública**; a partir de ahí rige la política por clave que el chaincode fija, que tiene precedencia. Las tres operaciones que crean una clave pública nueva —`RegisterUnit`, `RegisterOrganization` y `AuthorizeLabIntervention`— escriben además el marcador de la organización responsable, que es lo que efectivamente exige su endoso en esa primera transacción. Consecuencia para el cliente: una operación del registro organización-establecimiento se confirma con el endoso del peer regulatorio **solo**, sin necesidad de recolectar el endoso de ninguna organización custodial.

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
| `LAB_INTERVENTION_NOT_FOUND` | Se intentó revocar una autorización de intervención que no existe para esa unidad (ADR-007, punto 6.f). |
| `LAB_INTERVENTION_NOT_ACTIVE` | Se intentó revocar una autorización cuyo `estado` ya es `CONSUMIDA` o `REVOCADA`; la revocación no es idempotente y no reabre una autorización cerrada (ADR-007, punto 6.f). |
| `LAB_INTERVENTION_REQUIRED` | Un laboratorio no custodio intentó un retiro, recupero o disposición final sin una autorización de intervención **`ACTIVA` y vigente** para esa unidad, ese laboratorio y esa operación — inexistente, vencida, ya consumida o revocada (DES-6; ADR-007, puntos 6.e y 6.f). |
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
- **Endoso**: **peer del laboratorio invocante, exigido por la plataforma** (DES-6; ADR-007, punto 6.g). La clave pública de la unidad no existe todavía y por eso no puede quedar cubierta por endoso basado en estado; la operación escribe además un **marcador de participación** en la colección implícita del laboratorio, cuya política de endoso pertenece a esa organización y rige desde el despliegue. Precisión: la política de chaincode admite endosantes adicionales, de modo que el laboratorio es **necesario**, no exclusivo.
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
- **Endoso**: peer del custodio actual (emisor). La operación fija sobre la clave de la unidad la política de tránsito `AND(emisor, receptor declarado)`, que rige las escrituras posteriores (ADR-007, punto 6.b).
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
- **Endoso**: `AND(org emisora, org receptora declarada)`, impuesto por la política de la clave que fijó el despacho. **Sin rama alternativa**: ninguna otra organización —tampoco `AnmatMSP`— puede sustituir a una de las dos partes (ADR-007, punto 6.b).
- **Request**: `UnitRefRequest` (`gtin`, `numeroSerie`). Puede acompañarse de `transient` clave `commercial` con la confirmación documental de recepción.
- **Response**: `MedicationUnitView` con `custodioActual` = el receptor y `estado=EN_CUSTODIA`.
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `NOT_IN_TRANSIT`, `RECEIVER_MISMATCH`, `UNAUTHORIZED_ROLE`, `ORG_NOT_REGISTERED`, `ORG_INACTIVE`.

### `RejectTransfer`

```go
func (c *SNTContract) RejectTransfer(ctx contractapi.TransactionContextInterface, req UnitEventRequest) (*MedicationUnitView, error)
```

- **Transición**: T05. Estado resultante: `DEVUELTO`.
- **Autorización**: invocador = destinatario declarado en el registro de la operación **activa** (ADR-004; nunca contra operaciones cerradas) o custodio actual (emisor), `snt.role=operator`.
- **Endoso**: `AND(org emisora, org receptora declarada)`, impuesto por la política de la clave que fijó el despacho. **Sin rama alternativa**: ninguna otra organización —tampoco `AnmatMSP`— puede sustituir a una de las dos partes (ADR-007, punto 6.b).
- **Request**: `UnitEventRequest` (`gtin`, `numeroSerie`, `motivo`).
- **Response**: `MedicationUnitView` con `estado=DEVUELTO` y `custodioActual` = el emisor, que permanece sin cambios por convención de ADR-004; T05 no registra que el retorno físico al remitente haya ocurrido (la resolución posterior de `DEVUELTO` se rige por ADR-001 y EXT-4).
- **Errores**: `UNIT_NOT_FOUND`, `NOT_IN_TRANSIT`, `RECEIVER_MISMATCH` (el invocador no es ni el destinatario declarado ni el emisor), `UNAUTHORIZED_ROLE`, `INVALID_REQUEST`.

### `Dispense`

```go
func (c *SNTContract) Dispense(ctx contractapi.TransactionContextInterface, req UnitRefRequest) (*MedicationUnitView, error)
```

- **Transición**: T06. Estado resultante: `DISPENSADO`.
- **Autorización**: invocador = custodio actual con `agentType=PHARMACY` o `HEALTHCARE_FACILITY`, `snt.role=operator`.
- **Endoso**: peer de la organización dispensadora, que es el custodio actual y la única rama de la política de reposo de la clave (ADR-007, punto 6.a).
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
| `Quarantine` | T07, T08, T09 | `EN_CUARENTENA` | Custodio actual, **destinatario declarado** (solo T09, unidad en `EN_TRANSITO`) o ANMAT | Peer del custodio actual; **+ peer de la organización regulatoria** cuando lo inicia ANMAT (marcador de participación) |
| `ReleaseQuarantine` | T10 | `EN_CUSTODIA` | Custodio actual o ANMAT | Peer del custodio actual; + peer regulatorio cuando lo inicia ANMAT |
| `ReportExpired` | T11, T12, T13 | `VENCIDO` | Custodio actual, **destinatario declarado** (solo T13 desde `EN_TRANSITO`) o ANMAT | Peer del custodio actual; + peer regulatorio cuando lo inicia ANMAT |
| `ReportStolen` | T14 | `ROBADO` | Custodio actual o ANMAT | Peer del custodio actual; + peer regulatorio cuando lo inicia ANMAT |
| `ReportLost` | T15 | `EXTRAVIADO` | Custodio actual o ANMAT | Peer del custodio actual; + peer regulatorio cuando lo inicia ANMAT |
| `ReportDamaged` | T16 | `DETERIORADO` | Custodio actual o ANMAT | Peer del custodio actual; + peer regulatorio cuando lo inicia ANMAT |
| `WithdrawFromMarket` | T17, T18, T19 | `RETIRADO_MERCADO` | ANMAT o laboratorio titular | Peer del custodio actual; + peer regulatorio cuando lo inicia ANMAT; **laboratorio no custodio: previa `AuthorizeLabIntervention` + peers del laboratorio y del regulador** (DES-6; ADR-007, punto 6.e) |
| `ProhibitProduct` | T20 | `PROHIBIDO` | Solo ANMAT | Peers de la organización regulatoria y del custodio actual (`REGULATORY_ONLY` si el invocador no es el regulador) |
| `ReturnProduct` | T21, T22, T23, T24 | `DEVUELTO` | Custodio actual (o ANMAT según origen) | Peer del custodio actual; + peer regulatorio cuando lo inicia ANMAT |
| `Restock` | T25, T26, T27 | `EN_CUSTODIA` | Agente de recupero, custodio, ANMAT o **laboratorio titular** (T27, desde `RETIRADO_MERCADO`) según origen | Peer del custodio actual; + peer regulatorio si lo inicia ANMAT; laboratorio no custodio: previa `AuthorizeLabIntervention` + peers del laboratorio y del regulador |
| `FinalDisposition` | T28, T29, T30, T31, T32, T33 | `DISPUESTO_FINAL` | Agente de recupero/disposición, ANMAT o laboratorio según origen | Peer del custodio actual; + peer regulatorio en disposiciones regulatorias; laboratorio no custodio: previa `AuthorizeLabIntervention` + peers del laboratorio y del regulador |

Notas:

- **`ReturnProduct` admite un `transient` opcional** con la clave `devolucion`, para declarar el receptor de la devolución (ADR-009). Como todo identificador de contraparte, **no** viaja como argumento público: revela una relación no consumada. Se persiste en la PDC del par, en el registro de devolución `ReturnOp`+[`gtin`,`numeroSerie`,`txIdDevolucion`] (ADR-006, punto 4) — clave propia, porque una devolución T21–T24 no nace de un despacho y no tiene registro de operación de transferencia al cual adosarse y **no** modifica `custodioActual`, que permanece en el custodio declarante.

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

- **Quién puede informar un evento durante el tránsito**: además del custodio registrado —que durante el tránsito sigue siendo el **emisor** (ADR-004)— y de ANMAT, el **destinatario declarado** puede invocar `Quarantine` (T09) y `ReportExpired` (T13). Es el caso que ADR-001 enuncia expresamente para T09, «se detecta anomalía durante el traslado **o recepción**»: sin esa habilitación, un receptor que recibe mercadería anómala solo podría aceptar la custodia y recién después ponerla en cuarentena, o rechazar la transferencia entera. El chaincode resuelve al destinatario declarado leyendo el registro de la operación **activa** en la PDC del par, igual que en `ReceiveTransfer`; si el invocador no es ni el custodio, ni el destinatario declarado, ni el regulador, devuelve `UNAUTHORIZED_CUSTODIAN`. No aplica a `ReportStolen`, `ReportLost` ni `ReportDamaged`: ADR-001 reserva T14–T16 al custodio actual o a ANMAT aunque la unidad esté en tránsito.

- **Eventos extraordinarios sobre una unidad en `EN_TRANSITO`**: mientras dura el tránsito, la clave de la unidad lleva la política `AND(org emisora, org receptora declarada)` (ADR-007, punto 6.b), de modo que **cualquier** evento en esa ventana —lo inicie el custodio o ANMAT— exige el endoso de **ambas** organizaciones de la operación pendiente. El evento cierra además el registro de operación en la PDC del par (`DelPrivateData`, ADR-006, punto 4), escritura sujeta a la política de la colección `OR(org emisora, org receptora)`, que queda satisfecha por construcción y opera como barrera independiente.

- **Endoso de los eventos iniciados por ANMAT**: la política de la clave de una unidad **no** admite a `AnmatMSP` como rama alternativa. Si la admitiera, cualquier operación sobre esa clave —incluida una dispensación o una recepción— podría endosarse únicamente con el peer del regulador, porque Fabric evalúa la política de la clave sin saber qué función la escribió. El coendoso regulatorio se obtiene por otro camino: la transacción escribe un **marcador de participación** en la colección implícita de la organización regulatoria, cuya política de endoso le pertenece, de modo que la plataforma exige su endoso. Endosantes resultantes: **organización regulatoria + custodio actual**, o la organización regulatoria y ambas partes si la unidad está en tránsito. La firma de creador del regulador sigue siendo necesaria —la lógica resuelve `cid.GetMSPID()` como `agentType=REGULATOR` con `snt.role=regulatory-admin`— pero ya no es lo único que acredita su participación. El cliente debe recolectar todos esos endosos; si alguno de esos peers no está disponible, la operación no puede confirmarse (limitación declarada en `docs/alcance-prototipo.md`).

- **Intervención de un laboratorio no custodio** (`WithdrawFromMarket`, `Restock`, `FinalDisposition`): cuando el invocador es una organización con `agentType=LABORATORY` que **no** es el custodio actual, el chaincode exige una autorización de intervención en `estado=ACTIVA` y no vencida (`AuthorizeLabIntervention`) para esa unidad, ese laboratorio y esa operación, la marca como `CONSUMIDA` y escribe **marcadores de participación** en la colección implícita del laboratorio y en la de la organización regulatoria. La plataforma exige, en consecuencia, el endoso del laboratorio designado y el de la organización regulatoria —el par que pide DES-6— más el del custodio actual que impone la clave de la unidad: tres organizaciones (ADR-007, punto 6.e). Sin autorización, o con una vencida, ya consumida o revocada, `LAB_INTERVENTION_REQUIRED`.

- El chaincode valida internamente que el estado de origen de la unidad admita la transición pedida; si no, devuelve `INVALID_STATE_TRANSITION`. La misma función cubre varios estados de origen (por ejemplo `ReportStolen` aplica a `EN_LABORATORIO`, `EN_TRANSITO`, `EN_CUSTODIA`, `EN_CUARENTENA` o `DEVUELTO`).
- Las operaciones que exigen ANMAT devuelven `REGULATORY_ONLY` si el invocador no satisface el rol o coendoso regulatorio.
- Errores comunes a todas: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `INVALID_STATE_TRANSITION`, `UNAUTHORIZED_CUSTODIAN`/`REGULATORY_ONLY`, `UNAUTHORIZED_ROLE`. En `WithdrawFromMarket`, `Restock` y `FinalDisposition` invocadas por un laboratorio no custodio, además `LAB_INTERVENTION_REQUIRED`; en `ReturnProduct` con `transient`, los seis códigos de la tabla de validación.

### `AuthorizeLabIntervention`

```go
func (c *SNTContract) AuthorizeLabIntervention(ctx contractapi.TransactionContextInterface, req AuthorizeLabInterventionRequest) (*LabInterventionView, error)
```

Autoriza a un laboratorio titular a ejecutar **una** operación extraordinaria sobre una unidad que está bajo custodia de un tercero. Existe porque el par de endosos que DES-6 exige para ese caso no puede imponerse con SBE sobre la clave de la unidad: la política de una clave se evalúa contra el estado previo y no puede condicionarse a la operación intentada (ADR-007, punto 6.e).

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
- **Efectos**: escribe la clave pública `LabIntervention`+[`gtin`,`numeroSerie`] con `estado=ACTIVA`, le fija una política de endoso por clave (SBE) **de la organización regulatoria** y escribe un marcador de participación en su colección implícita, que fuerza el endoso de su peer ya en esta primera escritura. Una autorización nueva sobre la misma unidad **reemplaza** a la anterior, cualquiera sea su estado; `GetUnitHistory` no la refleja, pero `GetHistoryForKey` sobre la clave de autorización conserva la secuencia completa.
- **Endoso**: peer de la organización regulatoria **solo**. La primera escritura de la clave se valida contra la política de chaincode, que incluye a la organización regulatoria (ADR-007, punto 6.j); las posteriores, contra la SBE regulatoria de la clave. En ambos casos el marcador impone el endoso de su peer.
- **Consumo**: la operación del laboratorio la lee, verifica que coincidan laboratorio y operación y que no haya expirado, y la marca como consumida. Los endosos que la plataforma exige a esa transacción provienen de los marcadores de participación (laboratorio designado y organización regulatoria) y de la clave de la unidad (custodio actual): **tres** organizaciones (ADR-007, punto 6.e).
- **Vigencia y estados**: la autorización persiste un `estado` con tres valores — `ACTIVA`, `CONSUMIDA` (la ejerció el laboratorio designado) y `REVOCADA` (la dejó sin efecto la autoridad) — y un `expiraEn` que la **lógica** del chaincode evalúa contra `GetTxTimestamp()`. El vencimiento es una condición **derivada**: no se persiste y no borra la clave ni su política. Una autorización vencida sigue existiendo, simplemente deja de ser ejercible. Por eso la SBE de esta clave es de la organización regulatoria y no conjunta con el laboratorio: si lo fuera, reemplazar una autorización vencida por otra dirigida a un laboratorio distinto exigiría el endoso del laboratorio anterior, y bastaría con que ese laboratorio se negara, perdiera su peer o saliera del canal para que la unidad quedara impedida de forma permanente de recibir cualquier autorización nueva.
- **Response** (`LabInterventionView`): los campos persistidos más el `mspId` regulatorio que la emitió y el timestamp de emisión.
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `ORG_NOT_REGISTERED`, `ORG_INACTIVE`, `INVALID_LAB_INTERVENTION`, `REGULATORY_ONLY`.
- **Revocación**: por `RevokeLabIntervention` (abajo). **No** puede hacerse reemitiendo la autorización con un `expiraEn` ya alcanzado: la validación de esta misma operación exige una fecha posterior al timestamp de la transacción y rechazaría esa reemisión con `INVALID_LAB_INTERVENTION`. La versión 2.4.0 de este contrato afirmaba lo contrario; era una contradicción con su propia validación.

### `RevokeLabIntervention`

```go
func (c *SNTContract) RevokeLabIntervention(ctx contractapi.TransactionContextInterface, req RevokeLabInterventionRequest) (*LabInterventionView, error)
```

Deja sin efecto una autorización de intervención antes de su vencimiento. Existe porque un campo de vigencia no es un mecanismo de revocación: `expiraEn` no borra la clave, y reemitir la autorización vencida es imposible por la validación de `AuthorizeLabIntervention` (ADR-007, punto 6.f).

- **Autorización**: organización con `agentType=REGULATOR` y `snt.role=regulatory-admin`. La emitió esa misma autoridad y la SBE de la clave la exige solo a ella.
- **Endoso**: peer de la organización regulatoria **solo** — SBE regulatoria de la clave, más su marcador de participación.
- **Request**:

```json
{
  "gtin": "07791234567898",
  "numeroSerie": "SN-0001-ABCD",
  "motivo": "Autorización emitida sobre la unidad equivocada."
}
```

- **Validaciones**: existe una autorización para esa unidad (`LAB_INTERVENTION_NOT_FOUND`); su `estado` es `ACTIVA` (`LAB_INTERVENTION_NOT_ACTIVE` si ya está `CONSUMIDA` o `REVOCADA`). Una autorización **vencida** pero `ACTIVA` sí puede revocarse: cierra el registro de forma explícita en lugar de dejarlo en un estado que solo el timestamp desambigua.
- **Efectos**: persiste `estado=REVOCADA`, `revocadaEn` = `GetTxTimestamp()` y `motivoRevocacion`; conserva la clave y su SBE; escribe el marcador de participación regulatorio. **No** se borra la clave: la traza de emisión y revocación es evidencia auditable, y borrarla devolvería la próxima autorización sobre esa unidad a la ventana de creación de una clave nueva.
- **`motivo`**: se aplica el mismo lineamiento que al resto del contrato — texto administrativo, sin datos personales del paciente ni de personal identificable.
- **Response** (`LabInterventionView`): la autorización con su estado ya revocado.
- **Errores**: `INVALID_REQUEST`, `UNIT_NOT_FOUND`, `LAB_INTERVENTION_NOT_FOUND`, `LAB_INTERVENTION_NOT_ACTIVE`, `REGULATORY_ONLY`.

## Operaciones del registro organización-establecimiento

Gobernadas por ADR-003 y DES-6. Solo `AnmatMSP` con `snt.role=regulatory-admin`.

### `RegisterOrganization`

```go
func (c *SNTContract) RegisterOrganization(ctx contractapi.TransactionContextInterface, req RegisterOrganizationRequest) (*OrganizationView, error)
```

- **Autorización / endoso**: organización con `agentType=REGULATOR` y `snt.role=regulatory-admin`. La operación escribe un marcador de participación en su colección implícita, de modo que el endoso de su peer queda exigido por la plataforma desde la primera escritura de la entrada (ADR-007, punto 6.g); la entrada recibe además SBE regulatoria para sus modificaciones posteriores. El peer regulatorio **basta**: la política de chaincode que valida esa primera escritura incluye a la organización regulatoria (ADR-007, punto 6.j), de modo que no hace falta el endoso de ninguna organización custodial.
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

- **Autorización / endoso**: organización con `agentType=REGULATOR` y `snt.role=regulatory-admin`. La operación escribe un marcador de participación en su colección implícita, de modo que el endoso de su peer queda exigido por la plataforma desde la primera escritura de la entrada (ADR-007, punto 6.g); la entrada recibe además SBE regulatoria para sus modificaciones posteriores. El peer regulatorio **basta**: la política de chaincode que valida esa primera escritura incluye a la organización regulatoria (ADR-007, punto 6.j), de modo que no hace falta el endoso de ninguna organización custodial.
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

// RevokeLabInterventionRequest
{
  "gtin": "string",
  "numeroSerie": "string",
  "motivo": "string"
}

// LabInterventionView
{
  "gtin": "string",
  "numeroSerie": "string",
  "laboratorio": "string (GLN:/CUFE:)",
  "operacion": "string",
  "motivo": "string",
  "expiraEn": "string (ISO 8601)",
  "estado": "ACTIVA | CONSUMIDA | REVOCADA",
  "emitidaPor": "string (mspId de la organización regulatoria)",
  "emitidaEn": "string (ISO 8601)",
  "consumidaEn": "string (ISO 8601, presente solo si estado = CONSUMIDA)",
  "revocadaEn": "string (ISO 8601, presente solo si estado = REVOCADA)",
  "motivoRevocacion": "string (presente solo si estado = REVOCADA)"
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
- **Historial de cambios compatibles**: `2.6.1` — dos precisiones de redacción, sin cambios de firmas, `code`s ni esquemas. La fila de ADR-006 de la tabla de relación con las decisiones existentes atribuía a la política de colección `OR(org A, org B)` una garantía que no da —«exija también el endoso de una de las dos partes»—; quien exige a ambas es la política de la clave pública `AND(emisor, receptor)`, y la de colección es una barrera adicional. Y la nota del `transient` `devolucion` decía que el receptor declarado se persiste «junto al resto del registro de la operación», cuando una devolución T21–T24 no nace de un despacho: ADR-006 le da clave propia (`ReturnOp`). `2.6.0` — alinea la columna de **actor habilitado** con la tabla de transiciones de ADR-001, que es la fuente de verdad de quién puede detonar cada transición y está en estado Aceptado. Tres filas la contradecían: `Quarantine` omitía al `DESTINATION_AGENT` de T09, `ReportExpired` al de T13 y `Restock` al `LABORATORY` de T27. La omisión no era inocua: durante el tránsito el custodio registrado es el emisor, de modo que el contrato anterior impedía al receptor poner en cuarentena una unidad anómala al recibirla — exactamente el caso que ADR-001 enuncia para T09. Se agrega la nota que explica cómo el chaincode resuelve al destinatario declarado y por qué T14–T16 no se amplían. Ninguna firma, esquema ni `code` cambia; **sí** cambia quién puede invocar tres operaciones, y por eso es MINOR y no PATCH. `2.5.0` — cierra dos consecuencias de la 2.4.0 que sus propias reglas volvían contradictorias. **Revocación**: la 2.4.0 sostenía que la autoridad podía dejar sin efecto una autorización de intervención reemitiéndola con un `expiraEn` ya alcanzado, pero la validación de `AuthorizeLabIntervention` exige que `expiraEn` sea posterior al timestamp de la transacción y habría rechazado esa reemisión con `INVALID_LAB_INTERVENTION`; se agrega la operación `RevokeLabIntervention`, el campo `estado` (`ACTIVA`/`CONSUMIDA`/`REVOCADA`) con `consumidaEn`, `revocadaEn` y `motivoRevocacion` en `LabInterventionView`, y los códigos `LAB_INTERVENTION_NOT_FOUND` y `LAB_INTERVENTION_NOT_ACTIVE`. **Política de endoso de chaincode**: se declara que la definición operativa lleva `OR(<custodiales>, <regulatoria>)` y no solo `OR(<custodiales>)`; con la política anterior, la primera escritura de una entrada del registro o de una clave de autorización —actos que este contrato declara exclusivos del regulador— exigía además el endoso accidental de una organización custodial cualquiera. Se documenta además el esquema de clave de los marcadores de participación, en dos variantes (`Unidad` y `Organizacion`), porque la única forma declarada en la 2.4.0 no era construible para las operaciones del registro. No cambian firmas existentes ni la semántica de ningún `code` previo; **sí** cambia de qué peers debe recolectar endosos el cliente en las operaciones del registro, y por eso es MINOR y no PATCH. `2.4.0` — incorpora el **marcador de participación** en colecciones implícitas como mecanismo de coendoso (ADR-007, punto 6), lo que corrige tres puntos de la versión anterior: los eventos regulatorios recuperan el coendoso real del peer de la organización regulatoria, que la 2.3.0 había dejado apoyado únicamente en la firma de creador —la misma distinción que el contrato ya aplicaba al laboratorio—; `RegisterUnit` pasa a exigir el peer del laboratorio invocante desde la primera escritura, cerrando la ventana de creación que la 2.3.0 declaraba como límite; y la SBE de la clave de `AuthorizeLabIntervention` vuelve a ser de la organización regulatoria, porque una política conjunta con el laboratorio impedía reemplazar una autorización vencida sin el endoso del laboratorio anterior y podía dejar una unidad permanentemente impedida de recibir otra. No cambian firmas, esquemas ni la semántica de ningún `code`; **sí** cambia de qué peers debe recolectar endosos el cliente, y por eso es MINOR y no PATCH. `2.3.0` — precisa las columnas y notas de **endoso** de todo el contrato tras la corrección de ADR-007 sobre la regla de que la política de endoso es de la clave y no de la función invocada. La política de la clave de una unidad deja de admitir a `AnmatMSP` como rama alternativa —la admitía para habilitar los eventos regulatorios y habilitaba con ello el endoso unilateral del regulador en operaciones ordinarias—, con lo que un evento regulatorio pasa a ejecutarse con ANMAT como creador y el peer del custodio actual como endosante, y todo evento durante el tránsito exige a las dos partes; la SBE de la clave de `AuthorizeLabIntervention` pasa de `AnmatMSP` a `AND(laboratorio designado, AnmatMSP)`, porque la firma de creador del laboratorio no es un endoso de peer; y `RegisterUnit` declara el límite de la ventana de creación de una clave. No cambian firmas, esquemas ni la semántica de ningún `code`, pero **sí cambia de qué peers debe recolectar endosos el cliente**, y por eso se registra como MINOR y no como PATCH. `2.2.0` — incorpora la superficie que exige la corrección de ADR-007 sobre los límites del endoso basado en estado, y cierra las reglas que ADR-009 dejaba abiertas: nueva operación `Init`, sin argumentos, que siembra la entrada `REGULATOR` resolviendo la identidad del regulador contra el manifiesto fundacional embebido en el paquete (ADR-010); nueva operación `AuthorizeLabIntervention`, sin la cual el par de endosos que DES-6 exige a un laboratorio no custodio no es materializable; seis validaciones tipificadas del receptor declarado en el `transient` `devolucion` de `ReturnProduct`; nota de endoso para los eventos extraordinarios sobre una unidad en `EN_TRANSITO`, que cierran el registro privado y por lo tanto exigen también el endoso de una de las dos partes; `UNAUTHORIZED_AGENT_TYPE` en la lista de errores de `VerifyTrace`; y los códigos nuevos `ALREADY_INITIALIZED`, `INVALID_LAB_INTERVENTION` y `LAB_INTERVENTION_REQUIRED`. Agregados compatibles: ninguna firma existente cambia y ningún `code` previo altera su semántica. `2.1.0` — incorpora al contrato la superficie pública que introdujeron ADR-009, ADR-010 y ADR-011, en lugar de diferirla a las issues de implementación: nueva operación de lectura `VerifyTrace` con su veredicto estructurado; `transient` opcional `devolucion` en `ReturnProduct`; valores admitidos de `agentType`/`idType` para organizaciones no custodiales en `RegisterOrganization`, con la invariante de unicidad del regulador; invariante de último regulador activo y nuevo `code` `LAST_ACTIVE_REGULATOR` en `SetOrganizationActive`. Agregados compatibles: ninguna firma existente cambia ni se altera la semántica de un `code` previo. `2.0.1` — corrige el dígito verificador GS1 del GTIN de los ejemplos (`07791234567890` → `07791234567898`; el valor anterior habría sido rechazado por la propia validación de `INVALID_REQUEST`), completa las listas de errores por operación para que toda condición de autorización declarada tenga su código (`DispatchTransfer`/`ReceiveTransfer`: `ORG_NOT_REGISTERED`/`ORG_INACTIVE`; `RejectTransfer`: `RECEIVER_MISMATCH`; `Dispense`: `UNAUTHORIZED_ROLE`), agrega el lineamiento sobre `motivo` y aclara quién es B. Sin cambios de firmas, `code`s del catálogo ni esquemas. `2.0.2` — la autorización de `ReceiveTransfer`/`RejectTransfer` y el error `RECEIVER_MISMATCH` validan contra el registro de la operación **activa** (nunca contra operaciones cerradas, conforme el ciclo de vida de ADR-004); la respuesta de `RejectTransfer` deja de afirmar un retorno físico consumado; se precisa que el flujo del financiador usa `ReadUnit`/`GetUnitHistory`; las notas de dependencia de merge pasan a describir ADR-004/ADR-005 como decisiones vigentes en `develop`. Sin cambios de firmas, `code`s ni esquemas.

