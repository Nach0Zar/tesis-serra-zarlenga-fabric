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

Ventaja concreta de usar composite key en vez de concatenar el string a mano (`gtin + "-" + numeroSerie`): habilita `GetStateByPartialCompositeKey(medicationUnitObjectType, []string{gtin})` para recuperar todas las unidades registradas bajo un GTIN dado sin mantener un índice secundario aparte. Esto es relevante para casos de uso reales del SNT como "listar todas las unidades en circulación de este lote" o soporte a un evento de retiro del mercado por GTIN.

La validación de formato de GTIN y de número de serie (dígito verificador GS1, longitud máxima 20 caracteres alfanuméricos, prohibición de empezar con "779" si ocupa los 20 caracteres) es responsabilidad de una capa de validación de entrada, no de la construcción de la clave en sí — se menciona acá porque una clave construida sobre un GTIN o serie mal formado sería una clave válida para Fabric pero inválida para el dominio.

## 3. Struct de estado público

```go
// MedicationUnit representa el estado publico del canal para una unidad
// de medicamento. No incluye datos comerciales/documentales (ver ADR-002)
// ni informacion personal o clinica (ver seccion 5).
type MedicationUnit struct {
    DocType             string `json:"docType"`
    GTIN                string `json:"gtin"`
    NumeroSerie         string `json:"numeroSerie"`
    Lote                string `json:"lote"`
    FechaVencimiento    string `json:"fechaVencimiento"`
    CustodioActual      string `json:"custodioActual"`
    Estado              string `json:"estado"`
    UltimaActualizacion string `json:"ultimaActualizacion"`
}
```

### 3.1 `DocType`

No viene pedido explícitamente por el paper ni por los ADR — es un campo técnico que agrego y que conviene decidir de forma consciente, no asumir. El world state de un canal es un key-value store compartido por todos los tipos de activo que el chaincode maneje (por ejemplo, si más adelante se agrega el registro de organización-establecimiento de ADR-003 como otro tipo de documento en el mismo namespace). Un discriminador fijo (`"medicationUnit"`) evita ambigüedad en consultas ricas (CouchDB `GetQueryResult` con selector) y en índices. Si el diseño final decide que cada tipo de activo vive en su propio namespace de composite key sin necesidad de selector, este campo es descartable sin impacto en la clave.

### 3.2 `GTIN`, `NumeroSerie`

Duplican los componentes de la clave compuesta dentro del valor. Es redundante respecto a la clave, pero necesario en la práctica: Fabric no permite reconstruir de forma directa y económica los atributos de una composite key a partir de una consulta por rango sin volver a parsear el string de la clave (`SplitCompositeKey`), y cualquier consumidor que reciba el valor serializado (API REST, cliente de reporting) lo necesita como campo del payload, no como algo que tenga que inferir de la clave interna de Fabric.

### 3.3 `Lote`, `FechaVencimiento`

Ambos son parte del "estado mínimo de trazabilidad" fijado por ADR-002 y corresponden a los metadatos que el paper asocia a cada unidad desde el proceso de producción/importación (§3.2, §3.3).

Decisión a resolver, no cerrada por este documento: el estándar GS1 codifica el vencimiento como `AAMMDD` (año de 2 dígitos), que es el formato en el que llega desde el código de barras 2D. Se recomienda **normalizar a `YYYY-MM-DD` (ISO 8601) antes de escribir al chaincode**, resolviendo el siglo con la regla de ventana deslizante de GS1, en lugar de persistir `AAMMDD` crudo. Motivo: `AAMMDD` es ambiguo fuera de contexto (¿"30" es 1930 o 2030?) y no ordena lexicográficamente de forma confiable a largo plazo si se usa en consultas por rango de vencimiento. Esa normalización pertenece a la capa de adaptación (cliente que decodifica el código 2D), no al chaincode, que debería recibir y persistir ya el valor normalizado.

### 3.4 `CustodioActual`

Se persiste el identificador canónico `GLN:<13 dígitos>` o `CUFE:<13 dígitos>` resuelto por el chaincode desde el registro de organización-establecimiento, tal como lo fija ADR-003. Nunca `mspId`, nunca un atributo de certificado. El campo es de solo-lectura para el cliente en las operaciones donde el invocador actúa como custodio (el chaincode lo resuelve internamente vía `cid.GetMSPID()` + registro); solo es un parámetro de entrada legítimo cuando identifica al **destinatario** de una transferencia, y ahí también se valida contra el registro antes de aceptarse.

**Dependencia abierta con DES-9 (ADR-004)**: este documento asume implícitamente que `CustodioActual` tiene siempre un único valor bien definido. Si DES-9 decide modelar la transferencia en dos fases (despacho → `EN_TRANSITO` → recepción, en vez de una transacción atómica), la semántica de `CustodioActual` durante la ventana de tránsito queda sin resolver acá: ¿sigue siendo el emisor hasta que el receptor confirma, o hace falta un campo adicional (p. ej. `DestinatarioPendiente`) para distinguir "quién tiene la custodia legal" de "quién fue declarado como destino de un despacho aún no confirmado"? Este documento no prejuzga esa decisión; si DES-9 adopta el modelo de dos fases, este struct debe revisarse antes de darse por cerrado.

### 3.5 `Estado`

Campo de tipo string sobre un catálogo cerrado, no un enum de Go embebido en este documento porque la matriz de transiciones válidas (qué estados pueden pasar a cuáles, y bajo qué evento/agente detonante) es responsabilidad de **ADR-001 (DES-1, issue #7)** — "el diagrama de estados ES el chaincode", en palabras del propio issue. El paper dedica una sección completa (§2.1.3.3) a los 12 procesos del flujo SNT sin reducirlos a una única tabla de transición trivial, y DES-1 ya trae un primer catálogo candidato (`EN_LABORATORIO`, `EN_TRANSITO`, `EN_CUSTODIA`, `EN_CUARENTENA`, `VENCIDO`, `ROBADO`, `EXTRAVIADO`, `DETERIORADO`, `RETIRADO_MERCADO`, `PROHIBIDO`, `DEVUELTO`, `DISPENSADO`, `DISPUESTO_FINAL`) que este documento no adopta ni valida — queda a criterio de quien resuelva DES-1, incluyendo si conviene alinear esos nombres con los pares que el paper agrupa (`ROBADO`/`EXTRAVIADO`, `DETERIORADO`/destruido) o mantenerlos separados. Este documento solo compromete el tipo del campo y su rol como "estado actual de la unidad".

### 3.6 `UltimaActualizacion`

Decisión importante y no obvia: este timestamp **debe** obtenerse de `ctx.GetStub().GetTxTimestamp()`, nunca de `time.Now()` del lado del chaincode. El modelo de endoso de Fabric exige que la ejecución del chaincode sea determinística: la misma propuesta, ejecutada por distintos peers endosantes de distintas organizaciones, tiene que producir exactamente el mismo write-set para que el endoso sea válido. `time.Now()` da un valor distinto en cada peer según cuándo ejecuta; `GetTxTimestamp()` devuelve el timestamp que el cliente fijó al armar la propuesta, idéntico para todos los peers que la procesan. Usar reloj local acá no es un detalle de estilo: rompe la propiedad de determinismo que justifica usar múltiples organizaciones endosando en primer lugar (ver fundamentos de smart contracts/consenso en el marco teórico del proyecto).

## 4. Qué NO se almacena en este activo, y por qué

| Dato excluido | Motivo |
|---|---|
| Nombre, DNI, domicilio del paciente | Ley 25.326 de Protección de Datos Personales. El propio paper restringe el registro de dispensación a "información mínima del paciente" para preservar su privacidad (§3.2). |
| Diagnóstico o condición clínica asociada a una dispensación | Calificaría como dato sensible de salud bajo la Ley 25.326, con protección reforzada más allá de un dato personal genérico. Ninguna obligación normativa del SNT relevada requiere que el chaincode lo conozca. |
| Identificación individual del afiliado/obra social más allá de lo necesario para cobertura | El paper es explícito: "evitando el almacenamiento de datos personales sensibles conforme a la legislación argentina de protección de datos personales" (§3.4). |
| Razón social, domicilio, CUIT del establecimiento custodio | Excluido explícitamente del registro de organización-establecimiento por ADR-003 ("no debe incluir razón social, domicilio, CUIT, datos personales, datos clínicos ni información comercial no necesaria"). Este activo hereda la misma restricción para el campo `CustodioActual`, que es solo un identificador, no un perfil del establecimiento. |
| `mspId` del custodio | ADR-003 lo excluye explícitamente como valor persistido de custodia — solo se persiste el identificador canónico GLN/CUFE, para no acoplar el historial de custodia a la configuración interna de red. |
| Precio, condiciones comerciales, cantidades negociadas, número de factura/remito | Corresponde a la capa de "información comercial y documental" que ADR-002 asigna a Private Data Collections, no al estado público del canal que modela este documento. |
| Historial completo de transacciones como campo embebido | Fabric ya provee el transaction log inmutable por clave (`GetHistoryForKey`). Duplicar el historial dentro del struct de estado actual sería redundante y generaría dos fuentes de verdad para lo mismo. |

## 5. Abierto / fuera de este documento

- Catálogo cerrado de valores de `Estado` y su matriz de transición válida por evento/agente detonante — **DES-1 / ADR-001 (issue #7)**.
- Si `CustodioActual` necesita un campo adicional para representar tránsito no confirmado — depende de **DES-9 / ADR-004 (issue #57)**.
- Struct de datos privados (PDC) para información comercial/documental, y el mecanismo de generación de membresía de colección a partir del registro de organización-establecimiento (pendiente de NET-5 según ADR-002).
- Validación de formato de entrada (dígito verificador GTIN, longitud/prefijo de número de serie, parseo AAMMDD→ISO 8601): pertenece a la capa de adaptación/cliente, no a este modelo. Si conviene validar el GTIN contra un catálogo/Vademécum real o asumirlo válido es una decisión de alcance de **DES-11 (issue #59)**, no de este documento.
- Firmas concretas de funciones de chaincode que leen/escriben este struct.

## Fuentes

| Fuente | Uso en este documento |
|---|---|
| [ADR-002: Topología de canales](adr/002-topologia-canales.md) | Fija qué campos son estado público del canal vs. PDC; este documento formaliza esa lista como struct. |
| [ADR-003: Identidad de establecimientos mediante GLN/CUFE](adr/003-establishment-identity-gln-cufe.md) | Fija el formato e identidad del campo `CustodioActual` y la exclusión de `mspId`/datos de establecimiento no necesarios. |
| Disposición ANMAT 3683/2011, §2.1.3.2 (marco teórico del proyecto) | Requisitos de serialización: unicidad GTIN+serie, formato de vencimiento GS1, longitud/prefijo de número de serie. |
| Ley 25.326 de Protección de Datos Personales | Justifica la exclusión de datos personales y clínicos de la sección 4. |
| Hyperledger Fabric chaincode shim (`CreateCompositeKey`, `GetStateByPartialCompositeKey`, `GetTxTimestamp`, `GetHistoryForKey`) | Fundamenta el diseño de clave compuesta y la regla de determinismo del timestamp. |