# Modelo de datos: activo de trazabilidad de medicamento

Este documento define la clave y la estructura del **estado público del canal** para cada unidad de medicamento trazada por el prototipo: la clave compuesta que la identifica en el world state, el struct con sus metadatos normativos, y qué datos se excluyen deliberadamente de este activo y por qué.

No define: el catálogo cerrado de valores de `Estado` ni su matriz de transición (queda para un ADR/issue de diseño de chaincode dedicado), el struct de datos privados que va en Private Data Collections (se define junto con la configuración de colecciones de [ADR-002](adr/002-topologia-canales.md)), ni firmas de funciones de chaincode.

## 1. Alcance y relación con los ADR existentes

[ADR-002](adr/002-topologia-canales.md) ya fija qué campos son públicos dentro del canal y cuáles van a colecciones privadas:

> **Estado público del canal**: GTIN, número de serie, lote, fecha de vencimiento, custodio actual (identificador canónico GLN/CUFE) y estado del producto.

Este documento es la formalización de ese "estado público del canal" como clave y struct concretos. Los datos comerciales/documentales (precio, condiciones comerciales, factura, remito) **no forman parte de este struct** — van en PDC según ADR-002 y se documentan aparte.

[ADR-003](adr/003-establishment-identity-gln-cufe.md) fija cómo se resuelve y persiste el custodio: como identificador canónico `GLN:<13 dígitos>` o `CUFE:<13 dígitos>`, resuelto desde el registro de organización-establecimiento a partir del `mspId` del invocador — nunca el `mspId` en sí, y nunca un atributo de certificado.

## 2. Clave compuesta: GTIN + número de serie

### 2.1 Por qué GTIN + serie y no solo serie

El paper es explícito en que la unicidad exigida por el SNT es sobre la **combinación** GTIN + número de serie, no sobre el serie de forma aislada (§2.1.3.2, §3.3 del marco teórico): el laboratorio garantiza unicidad algorítmica del serie dentro del universo de unidades que produce bajo un mismo GTIN, pero dos laboratorios distintos (o el mismo laboratorio en dos presentaciones/GTIN distintos) pueden en principio generar el mismo string de serie sin que eso sea una colisión real. La clave del activo tiene que reflejar esa misma composición para que la unicidad que el chaincode aplica sea la unicidad que la normativa exige — no una más laxa (solo GTIN, agruparía todo el lote bajo una clave) ni una más estricta con supuestos no garantizados (solo serie, asumiría unicidad global que ningún actor garantiza).

### 2.2 Implementación con `CreateCompositeKey`

Fabric expone `stub.CreateCompositeKey(objectType, []string{attr1, attr2, ...})`, que arma una clave con el formato `\x00<objectType>\x00<attr1>\x00<attr2>\x00`. Se usa así:

```go
const medicationUnitObjectType = "MedicationUnit"

func medicationUnitKey(stub shim.ChaincodeStubInterface, gtin, numeroSerie string) (string, error) {
    return stub.CreateCompositeKey(medicationUnitObjectType, []string{gtin, numeroSerie})
}
```

Ventaja concreta de usar composite key en vez de concatenar el string a mano (`gtin + "-" + numeroSerie`): habilita `GetStateByPartialCompositeKey(medicationUnitObjectType, []string{gtin})` para recuperar todas las unidades registradas bajo un GTIN dado sin mantener un índice secundario aparte. Esto es relevante para soportar un evento de retiro del mercado por GTIN. **Nota**: esta consulta agrupa por GTIN, no por lote — un mismo GTIN puede tener múltiples lotes en circulación, así que "listar todas las unidades de este lote" no queda cubierto por la clave compuesta; requeriría una consulta rica (CouchDB `GetQueryResult` con selector) filtrando por el campo `Lote`, fuera del alcance de este documento.

El formato persistido esperado de `GTIN` y `NumeroSerie` es el que fija GS1: dígito verificador GS1 en el GTIN, y número de serie alfanumérico de hasta 20 caracteres que no puede empezar con "779" si ocupa los 20 caracteres completos — este struct asume valores ya conformes a ese formato. Lo que este documento **no** fija es dónde se aplica esa validación (¿el chaincode la valida al recibir la transacción, o se asume como precondición garantizada por el cliente?); esa es una decisión de implementación que corresponde a **DES-5 (issue #11, contrato de interfaz del chaincode)** y al chaincode en sí, no a este modelo de datos. Se menciona acá porque una clave construida sobre un GTIN o serie mal formado sería una clave válida para Fabric pero inválida para el dominio.

## 3. Struct de estado público

```go
// MedicationUnit representa el estado publico del canal para una unidad
// de medicamento. No incluye datos comerciales/documentales (ver ADR-002)
// ni informacion personal o clinica (ver seccion 4).
type MedicationUnit struct {
    GTIN                string `json:"gtin"`
    NumeroSerie         string `json:"numeroSerie"`
    Lote                string `json:"lote"`
    FechaVencimiento    string `json:"fechaVencimiento"`
    CustodioActual      string `json:"custodioActual"`
    Estado              string `json:"estado"`
    UltimaActualizacion string `json:"ultimaActualizacion"`
}
```

Este struct se ciñe a lo pedido por la issue #8 (DES-2): clave GTIN+serie, y metadatos normativos lote/vencimiento/custodio/estado/timestamp. No incluye un campo discriminador de tipo de documento (`DocType`) — ver sección 5 para por qué queda fuera de este documento. Tampoco incluye el identificador del destinatario declarado durante una transferencia en curso: [ADR-004](adr/004-transfer-dispatch-reception.md) decidió que ese dato se persiste en la PDC de la operación, no en el estado público del canal — ver sección 3.6.

### 3.1 `GTIN`, `NumeroSerie`

Duplican los componentes de la clave compuesta dentro del valor. Es redundante respecto a la clave, pero necesario en la práctica: Fabric no permite reconstruir de forma directa y económica los atributos de una composite key a partir de una consulta por rango sin volver a parsear el string de la clave (`SplitCompositeKey`), y cualquier consumidor que reciba el valor serializado (API REST, cliente de reporting) lo necesita como campo del payload, no como algo que tenga que inferir de la clave interna de Fabric.

### 3.2 `Lote`, `FechaVencimiento`

Ambos son parte del "estado mínimo de trazabilidad" fijado por ADR-002 y corresponden a los metadatos que el paper asocia a cada unidad desde el proceso de producción/importación (§3.2, §3.3).

Decisión que este documento sí fija: el valor persistido de `FechaVencimiento` es `YYYY-MM-DD` (ISO 8601), no el `AAMMDD` (año de 2 dígitos) en el que llega el dato desde el código de barras 2D GS1, resolviendo el siglo con la regla de ventana deslizante de GS1. Motivo: `AAMMDD` es ambiguo fuera de contexto (¿"30" es 1930 o 2030?) y no ordena lexicográficamente de forma confiable a largo plazo si se usa en consultas por rango de vencimiento. Lo que este documento **no** fija es dónde ocurre esa normalización (cliente al decodificar el código 2D vs. chaincode al recibir la transacción) ni si el chaincode debe validar que el valor recibido ya está en formato ISO 8601 — son decisiones de implementación de **DES-5 (issue #11)** y del chaincode, no de este modelo de datos.

### 3.3 `CustodioActual`

Se persiste el identificador canónico `GLN:<13 dígitos>` o `CUFE:<13 dígitos>` resuelto por el chaincode desde el registro de organización-establecimiento, tal como lo fija ADR-003. Nunca `mspId`, nunca un atributo de certificado. El campo es de solo-lectura para el cliente en las operaciones donde el invocador actúa como custodio (el chaincode lo resuelve internamente vía `cid.GetMSPID()` + registro); el único dato de identidad que el cliente provee es el **destinatario** de una transferencia, que viaja por el campo `transient` de la propuesta de despacho — nunca como argumento público (ADR-004) — y también se valida contra el registro antes de aceptarse.

**Decisión resuelta por ADR-004**: `CustodioActual` permanece como el emisor durante el tránsito. El receptor declarado se registra como `DestinatarioPendiente` en la PDC de la operación, no en este struct (sección 3.6). Cuando el receptor confirma la recepción (T04 de ADR-001), el chaincode valida contra la PDC y actualiza `CustodioActual` (público) al receptor.

### 3.4 `Estado`

Campo de tipo string sobre un catálogo cerrado, no un enum de Go embebido en este documento porque la matriz de transiciones válidas (qué estados pueden pasar a cuáles, y bajo qué evento/agente detonante) es responsabilidad de **ADR-001 (DES-1, issue #7)** — "el diagrama de estados ES el chaincode", en palabras del propio issue. El paper dedica una sección completa (§2.1.3.3) a los 12 procesos del flujo SNT sin reducirlos a una única tabla de transición trivial, y ADR-001 (Aceptado) fijó el catálogo definitivo, que este documento adopta por referencia; era un catálogo candidato (`EN_LABORATORIO`, `EN_TRANSITO`, `EN_CUSTODIA`, `EN_CUARENTENA`, `VENCIDO`, `ROBADO`, `EXTRAVIADO`, `DETERIORADO`, `RETIRADO_MERCADO`, `PROHIBIDO`, `DEVUELTO`, `DISPENSADO`, `DISPUESTO_FINAL`) cuando se escribió esta sección. ADR-001 resolvió mantener `ROBADO`/`EXTRAVIADO` y `DETERIORADO` como estados separados, en lugar de agruparlos como hace el paper. Este documento solo compromete el tipo del campo y su rol como "estado actual de la unidad".

### 3.5 `UltimaActualizacion`

Decisión importante y no obvia: este timestamp **debe** obtenerse de `ctx.GetStub().GetTxTimestamp()`, nunca de `time.Now()` del lado del chaincode. El modelo de endoso de Fabric exige que la ejecución del chaincode sea determinística: la misma propuesta, ejecutada por distintos peers endosantes de distintas organizaciones, tiene que producir exactamente el mismo write-set para que el endoso sea válido. `time.Now()` da un valor distinto en cada peer según cuándo ejecuta; `GetTxTimestamp()` devuelve el timestamp que el cliente fijó al armar la propuesta, idéntico para todos los peers que la procesan. Usar reloj local acá no es un detalle de estilo: rompe la propiedad de determinismo que justifica usar múltiples organizaciones endosando en primer lugar (ver fundamentos de smart contracts/consenso en el marco teórico del proyecto).

### 3.6 `DestinatarioPendiente` (no es un campo de este struct — vive en PDC)

Durante una transferencia en curso (`Estado == EN_TRANSITO`), el chaincode necesita saber quién es el receptor declarado para validar, en la recepción, que quien invoca coincide con ese destino. [ADR-004](adr/004-transfer-dispatch-reception.md) decidió que este identificador **no se persiste en el struct público** de esta sección: se escribe en la PDC de la operación (membresía emisor + receptor declarado + `AnmatMSP`), la misma colección que ya recibe factura y remito. La razón es que el destinatario declarado revela una relación emisor→receptor no consumada mientras la transferencia está pendiente, y eso cae en la categoría de dato privado que ADR-002 ya reserva para PDC — no es información que el estado público del canal deba exponer. El detalle del argumento y el ciclo de vida completo del dato están en la sección "Por qué DestinatarioPendiente no es estado público" de ADR-004.

Contiene el identificador canónico `GLN:<13 dígitos>` o `CUFE:<13 dígitos>` del receptor declarado por el emisor al invocar el despacho (T02 o T03 de ADR-001), con el mismo formato que `CustodioActual` (ADR-003): nunca `mspId`, nunca un atributo de certificado. El emisor lo provee **exclusivamente por el campo `transient` de la propuesta** — nunca como argumento público de la transacción, porque los argumentos ordinarios quedan registrados en la transacción visible del canal (ADR-004, "Ciclo de vida del registro de operación").

El chaincode debe validar en el despacho que el destinatario declarado exista en el registro de organización-establecimiento (ADR-003), esté activo, y tenga un `agentType` compatible con el par origen-destino según la matriz de DES-3. En la recepción (T04), el chaincode valida contra la PDC que el `mspId` del invocador corresponda al establecimiento cuyo identificador canónico figura como destinatario declarado; esta transacción requiere endoso conjunto de la organización emisora y la receptora (ADR-004, sección "Endoso").

## 4. Qué NO se almacena en este activo, y por qué

| Dato excluido | Motivo |
|---|---|
| Nombre, DNI, domicilio del paciente | Dato personal bajo Ley 25.326, art. 2. El propio paper restringe el registro de dispensación a "información mínima del paciente" para preservar su privacidad (§3.2). |
| Diagnóstico o condición clínica asociada a una dispensación | Dato sensible de salud bajo Ley 25.326, art. 2, con régimen de protección reforzada por arts. 7-8. Ninguna obligación normativa del SNT relevada requiere que el chaincode lo conozca. |
| Identificación individual del afiliado/obra social más allá de lo necesario para cobertura | El paper es explícito: "evitando el almacenamiento de datos personales sensibles conforme a la legislación argentina de protección de datos personales" (§3.4). |
| Razón social, domicilio, CUIT del establecimiento custodio | Excluido explícitamente del registro de organización-establecimiento por ADR-003 ("no debe incluir razón social, domicilio, CUIT, datos personales, datos clínicos ni información comercial no necesaria"). Este activo hereda la misma restricción para el campo `CustodioActual`, que es solo un identificador, no un perfil del establecimiento. |
| `mspId` del custodio | ADR-003 lo excluye explícitamente como valor persistido de custodia — solo se persiste el identificador canónico GLN/CUFE, para no acoplar el historial de custodia a la configuración interna de red. |
| Precio, condiciones comerciales, cantidades negociadas, número de factura/remito | Corresponde a la capa de "información comercial y documental" que ADR-002 asigna a Private Data Collections, no al estado público del canal que modela este documento. |
| Historial completo de transacciones como campo embebido | Fabric ya provee el transaction log inmutable por clave (`GetHistoryForKey`). Duplicar el historial dentro del struct de estado actual sería redundante y generaría dos fuentes de verdad para lo mismo. |
| Identificador del destinatario declarado durante una transferencia en curso (`DestinatarioPendiente`) | Revela una relación emisor→receptor no consumada mientras dure el tránsito; ADR-004 lo clasifica como dato privado según ADR-002 y lo persiste en la PDC de la operación en lugar del estado público. Ver sección 3.6. |

## 5. Abierto / fuera de este documento

- Catálogo cerrado de valores de `Estado` y su matriz de transición válida por evento/agente detonante — **DES-1 / ADR-001 (issue #7)**.
- ~~Si `CustodioActual` necesita un campo adicional para representar tránsito no confirmado~~ — resuelto por **ADR-004**: el destinatario declarado se registra en la PDC de la operación, no como campo adicional del struct público (sección 3.6).
- ~~Struct de datos privados (PDC) para información comercial/documental, y el mecanismo de generación de membresía de colección~~ — resuelto por **ADR-006**: colecciones explícitas por par de organizaciones, nombre `transfer_<mspIdA>_<mspIdB>` con los `mspId` ordenados lexicográficamente, generadas por una herramienta de build a partir de un **manifiesto de organizaciones versionado en el repositorio** y de la matriz de DES-3. La entrada **no** es el registro organización-establecimiento del ledger: ese registro se siembra después de desplegar el chaincode, y el `collections_config.json` forma parte de la definición que se aprueba antes, con lo que tomarlo del ledger crearía una dependencia circular. Lo que queda para NET-5 (#24) es implementar la herramienta, no elegir el mecanismo.
- Punto de aplicación de la validación de formato de entrada (dígito verificador GTIN, longitud/prefijo de número de serie, parseo AAMMDD→ISO 8601): este documento fija el formato persistido esperado (secciones 2.2 y 3.2) pero no si la validación corre en el chaincode, en una capa de adaptación/cliente previa, o en ambos — es una decisión de **DES-5 (issue #11, contrato de interfaz del chaincode)** y de la implementación del chaincode. Si conviene validar el GTIN contra un catálogo/Vademécum real o asumirlo válido es además una decisión de alcance de **DES-11 (issue #59)**.
- Campo discriminador de tipo de documento (`DocType`) para desambiguar el namespace del world state si en el futuro conviven otros tipos de activo (p. ej. el registro de organización-establecimiento de ADR-003) en el mismo canal y hace falta filtrarlos en consultas ricas (CouchDB `GetQueryResult`). No lo pide la issue #8 y por eso no forma parte del struct de la sección 3 — si se necesita, es una decisión de una issue futura de consultas/índices, no de este documento.
- Firmas concretas de funciones de chaincode que leen/escriben este struct.

## Fuentes

| Fuente | Uso en este documento |
|---|---|
| [ADR-002: Topología de canales](adr/002-topologia-canales.md) | Fija qué campos son estado público del canal vs. PDC; este documento formaliza esa lista como struct. |
| [ADR-003: Identidad de establecimientos mediante GLN/CUFE](adr/003-establishment-identity-gln-cufe.md) | Fija el formato e identidad del campo `CustodioActual` y la exclusión de `mspId`/datos de establecimiento no necesarios. |
| [ADR-004: Transferencia como despacho/recepción](adr/004-transfer-dispatch-reception.md) | Fija que el destinatario declarado durante el tránsito se persiste en PDC, no en el struct público (sección 3.6). |
| [Disposición ANMAT 3683/2011](https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/actualizacion), art. 3 (contenido del código de identificación: GTIN + número de serie) y art. 6 (delega en ANMAT/entidades técnicas los parámetros específicos del sistema) | Fundamenta la elección de GTIN+serie como composición de la clave (sección 2). Los detalles de formato de las secciones 2.2/3.2 (longitud/prefijo del número de serie, formato AAMMDD del vencimiento) son especificaciones técnicas GS1 delegadas por el art. 6, no texto literal de la disposición — ver fila siguiente. |
| Marco teórico del proyecto (§2.1.3.2) | Fuente de los detalles de formato GS1 citados en 2.2/3.2 (longitud máxima y prefijo "779" del número de serie, formato AAMMDD del vencimiento), tal como la tesis resume las especificaciones técnicas delegadas por el art. 6 de la Disposición 3683/2011. |
| [Ley 25.326 de Protección de Datos Personales](https://servicios.infoleg.gob.ar/infolegInternet/anexos/60000-64999/64790/texact.htm), art. 2 (define "datos sensibles", incluida la información referente a la salud) y arts. 7-8 (régimen especial y prohibición de tratamiento de datos sensibles salvo excepciones) | Fundamenta la exclusión de datos personales y de salud de la sección 4. |
| Hyperledger Fabric chaincode shim (`CreateCompositeKey`, `GetStateByPartialCompositeKey`, `GetTxTimestamp`, `GetHistoryForKey`) | Fundamenta el diseño de clave compuesta y la regla de determinismo del timestamp. |