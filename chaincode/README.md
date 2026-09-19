# Chaincode `snt`

Smart contract Go del prototipo, implementado con `contractapi`. Su interfaz pública está congelada en [`docs/api-contract.md`](../docs/api-contract.md) —la versión vigente es la que declara su encabezado—; el chaincode se llama `snt` y el canal `snt-channel` ([ADR-007](../docs/adr/007-network-topology.md), punto 4).

La lógica respeta:

- la máquina de estados del medicamento de [ADR-001](../docs/adr/001-maquina-estados-medicamento.md);
- la identidad de establecimientos por MSP → registro → `agentType` de [ADR-003](../docs/adr/003-establishment-identity-gln-cufe.md), extendida a las organizaciones no custodiales por [ADR-010](../docs/adr/010-non-custodial-identity.md);
- la transferencia como dos transacciones (despacho/recepción) de [ADR-004](../docs/adr/004-transfer-dispatch-reception.md);
- las colecciones privadas por par de organizaciones de [ADR-006](../docs/adr/006-private-data-collections.md);
- la materialización del endoso de [ADR-007](../docs/adr/007-network-topology.md), punto 6;
- la matriz regulatoria [`domain/authorized-transfers.json`](../domain/authorized-transfers.json), consumida por el paquete compartido de [ADR-008](../docs/adr/008-transfer-matrix-distribution.md).

## Layout del workspace Go

[ADR-008](../docs/adr/008-transfer-matrix-distribution.md) y [ADR-012](../docs/adr/012-baseline-design.md) dejaron esta decisión derivada explícitamente a CC-1 (#14): *módulo único del repositorio* vs. *módulos separados con directiva `replace`*.

**Se adoptan módulos separados con `replace`.**

```text
domain/            módulo github.com/…/domain     — paquete compartido de reglas
  authorized-transfers.json                        matriz DES-3 (go:embed)
  transfers.go                                     función de decisión de la matriz (ADR-008)
  states.go                                        máquina de estados de ADR-001
  manifest/                                        manifiesto fundacional (go:embed, ADR-010)
chaincode/         módulo github.com/…/chaincode  — replace …/domain => ../domain
baseline/          módulo propio (BASE-1/BASE-2), mismo replace
```

Por qué, y no un módulo único del repositorio:

- **El empaquetado exige un `go.mod` en `chaincode/`.** `peer lifecycle chaincode package --path chaincode --lang golang` empaqueta un módulo Go; con módulo único habría que empaquetar la raíz del repositorio, y el artefacto arrastraría la baseline y los scripts de red.
- **El `packageID` debe depender solo del chaincode.** ADR-008 (punto 5) y ADR-010 (punto 4) construyen la raíz de confianza del bootstrap sobre un artefacto construido una sola vez, con `packageID` y checksum versionados y verificados con `queryinstalled`/`queryapproved`. Con módulo único, cambiar la baseline cambiaría el `packageID` del chaincode y obligaría a rehacer el ciclo de lifecycle sin que el chaincode hubiera cambiado.
- **La paridad de reglas se conserva igual.** Chaincode y baseline importan literalmente el mismo paquete `domain`: no hay dos implementaciones de la máquina de estados ni de la matriz que mantener consistentes (ADR-012, sección 1).

**Costo asumido — el paso de vendoring.** El `replace` apunta a `../domain`, fuera del directorio que el peer empaqueta. Para que el paquete sea autocontenido hay que materializar el módulo compartido dentro del chaincode antes de empaquetar:

```bash
cd chaincode
make vendor                # go mod vendor + verificación de que los dos JSON quedaron adentro
make stage                 # arma el árbol limpio que se empaqueta y lo compila
make package               # empaqueta, imprime checksum y packageID
make package-reproducible  # empaqueta dos veces y falla si el packageID cambia
```

`go mod vendor` copia también los archivos alcanzados por `//go:embed` (la matriz de DES-3 y el manifiesto fundacional), y `make vendor` lo verifica explícitamente en lugar de darlo por sentado. `chaincode/vendor/` **no se versiona**: es un artefacto de build.

### Por qué el empaquetado no ocurre dentro de `chaincode/`

ADR-008 (punto 5) y ADR-010 (punto 4) apoyan la raíz de confianza del bootstrap en un artefacto **construido una sola vez**, con `packageID` y checksum versionados y verificados con `queryinstalled`/`queryapproved`. Esa propiedad exige que el `packageID` sea función del código y de nada más.

`peer lifecycle chaincode package --path X` empaqueta **todo** lo que haya en `X`. Si el artefacto y el árbol empaquetado vivieran dentro de `chaincode/`, una segunda corrida de `make package` incluiría el `.tar.gz` de la primera —y cualquier `coverage.out` o archivo suelto del árbol de trabajo— y cambiaría el checksum sin que el código hubiera cambiado.

Por eso `make package` no empaqueta `chaincode/`:

```text
build/chaincode/                  fuera del árbol empaquetado (en .gitignore)
  snt_1.0/                        árbol staged: go.mod, go.sum, main.go, internal/, vendor/
  snt_1.0.tar.gz                  artefacto
```

`make stage` reconstruye el árbol staged desde cero en cada corrida, copiando una **lista explícita** de fuentes, y lo compila con `-mod=vendor` (con `-o` hacia afuera del árbol, para que el binario tampoco entre al paquete). Ese build es la red de seguridad de la lista: si una fuente nueva quedara afuera, falla ahí y no en el builder del peer. `make package-reproducible` cierra el punto empaquetando dos veces y comparando el `packageID`; el escenario de integración lo ejecuta en cada PR.

**No se usa `go.work`.** El `replace` alcanza para compilar, testear y vendorizar, y el workspace introduce una capa (`go.work.sum`, modo workspace en subdirectorios) que interactúa mal con `go mod vendor`. `go.work` sigue en `.gitignore` para quien lo quiera de forma local.

### Versiones de las librerías de Fabric

`chaincode/go.mod` fija `fabric-contract-api-go/v2 v2.2.0` y `fabric-chaincode-go/v2 v2.3.0` **a propósito**, no en su última versión: de `v2.2.1` en adelante la contract API declara `go 1.24`/`go 1.25`, lo que obligaría al builder de chaincode del peer (Fabric 2.5.x) a disponer de esa toolchain. Subirlas exige verificar antes el builder de la red (NET-3/#22, NET-4/#23).

## Manifiesto fundacional embebido

`Init` resuelve la identidad de la organización regulatoria contra el manifiesto embebido en el paquete, sin recibirla como argumento (ADR-010, punto 4). El archivo canónico es [`network/organizations-manifest.json`](../network/organizations-manifest.json), que NET-2 fijó como fuente de verdad de despliegue para sus tres consumidores (material criptográfico, colecciones y bootstrap del registro).

`//go:embed` no puede referenciar archivos fuera del directorio del paquete, así que `domain/manifest/` conserva una **copia**. No es una segunda fuente de verdad: `domain/manifest/sync_test.go` compara byte a byte contra el archivo canónico y falla si divergen, de modo que la divergencia no puede llegar a merge. Para refrescarla:

```bash
cd chaincode && make sync-manifest
```

Cambiar el manifiesto cambia el binario y, por lo tanto, el `packageID`: hay que rehacer la verificación de despliegue de ADR-008 punto 5.

## Estado de implementación

CC-1 (#14) entrega el scaffold. Las 25 operaciones del contrato están **declaradas** con su firma definitiva, y las que todavía no tienen lógica devuelven un error tipificado que nombra a su issue dueña.

Tres tests distintos custodian el congelamiento del contrato, y hacen falta los tres:

| Test | Qué garantiza | Qué dejaría pasar por sí solo |
|---|---|---|
| `TestContractSurfaceMatchesFrozenContract` | La lista de **nombres** es exactamente la del contrato: ni una de menos ni una de más. | Un cambio de tipo de request o de response conservando el nombre. |
| `TestChaincodeBuildsWithContractAPI` | `contractapi` **acepta** todas las firmas: si una no fuera admisible, el chaincode no arrancaría en el peer. | Una firma admisible pero distinta de la documentada (`UnitRefRequest` por `UnitEventRequest`, por ejemplo). |
| `TestContractSignaturesMatchFrozenContract` | Cada firma real coincide **tipo a tipo** con la que declara [`docs/api-contract.md`](../docs/api-contract.md), parseada del propio documento. | — |

`TestDocumentedOperationsMatchDeclaredSurface` cierra el otro sentido: que la lista contra la que se contrasta la superficie sea la que el contrato documenta, y no una copia que quedó atrás.

| Operación | Estado | Dueña |
|---|---|---|
| `Init` | Implementada | CC-1 (#14) |
| `RegisterOrganization`, `SetOrganizationActive` | Implementadas | CC-1 (#14) |
| `AuthorizeLabIntervention`, `RevokeLabIntervention` | Implementadas | CC-1 (#14) |
| `RegisterUnit` | Implementada | CC-2 (#15) |
| `DispatchTransfer`, `ReceiveTransfer`, `RejectTransfer` | Implementadas | CC-3 (#16) |
| `Dispense` | Implementada | CC-4 (#17) |
| `ReadUnit`, `GetUnitHistory`, `QueryUnitsByGTIN` | Implementadas | CC-5 (#18) |
| `Quarantine`, `ReleaseQuarantine` | Implementadas | EXT-1 (#27) |
| `ReportExpired` | Implementada | EXT-2 (#28) |
| `ReportStolen`, `ReportLost`, `ReportDamaged` | Implementadas | EXT-3 (#29) |
| `ReturnProduct` | Implementada | EXT-4 (#30) |
| `Restock` | Implementada | EXT-5 (#31) |
| `WithdrawFromMarket`, `ProhibitProduct` | Implementadas | EXT-6 (#32) |
| `FinalDisposition` | Implementada | EXT-8 (#63) |
| `VerifyTrace` | Implementada | CC-8 (#62) |

**No queda ninguna operación declarada sin implementar.** EXT-8 (#63) cerró la última, y con eso se retiró la maquinaria que CC-1 (#14) había dejado para los stubs: `internal/snt/declared.go`, el helper `notImplemented` y el salteo condicional de `TestPublicKeyCreationWritesMarker`. El test que comprobaba que un stub nombrara a su issue dueña se reemplazó por su inverso —`TestNoOperationRemainsAStub`—, que recorre la superficie congelada e impide reintroducir uno en silencio.

**`DISPUESTO_FINAL` es el único estado terminal que alcanza una operación del contrato.** La garantía de ADR-001 es que ninguna transición declara ese estado de origen —es decir, que la unidad **no puede salir** de él—, no que toda escritura devuelva el mismo código: las trece que llegan a la transición dan `INVALID_STATE_TRANSITION`, `ReceiveTransfer`/`RejectTransfer` dan `NOT_IN_TRANSIT` porque validan el tránsito antes, `RegisterUnit` da `UNIT_ALREADY_EXISTS` y `AuthorizeLabIntervention` **no falla** —límite declarado en el contrato: la autorización que emite es inconsumible porque ejercerla exigiría una transición inexistente—. `TestDisposedUnitCannotLeaveItsState` recorre las diecisiete escrituras públicas y comprueba en todas la única invariante que ADR-001 garantiza: el estado no se mueve.

**Auditoría de la disposición final, en dos alcances, ambos cubiertos**: por unidad, las lecturas públicas del canal no son restringibles ([ADR-005](../docs/adr/005-rol-organismo-financiador.md)), así que ANMAT lee estado e historial de cualquier unidad; y globalmente, `QueryUnitsByState` —que integró CC-9 (#112)— enumera el conjunto en `DISPUESTO_FINAL`, porque `FinalDisposition` escribe por `putUnit` como el resto y mantiene el índice `UnitByState`. `TestDisposedUnitsAreEnumerableByRegulator` lo comprueba disponiendo dos unidades y dejando una tercera en otro estado.

**Norma aplicable, y hasta dónde llega el prototipo**: el destino de una unidad dispuesta finalmente es un residuo peligroso — **Ley 24.051, Anexo I, categoría Y3** («Desechos de medicamentos y productos farmacéuticos»), [texto oficial](https://www.argentina.gob.ar/normativa/nacional/450/actualizacion). Ese régimen **no** queda satisfecho por la operación: sus **arts. 12 a 14** exigen un **manifiesto** con generador, transportista y planta de tratamiento. Los tres sujetos no están en la misma situación: el **generador** —que el art. 14 define como quien produce el residuo, o sea el custodio actual al disponer la unidad— **sí** es una organización del SNT y queda registrado; el transportista y la planta no pertenecen a la cadena que ADR-002 modela, y el manifiesto no se emite. Está declarado como límite en [`docs/alcance-prototipo.md`](../docs/alcance-prototipo.md). Lo que la operación sí aporta son los dos datos que vuelven atribuible el hecho: el `motivo` obligatorio y el `custodioActual`, que no se mueve. El chaincode **no** valida el contenido del `motivo`: exigir que cite una norma sería una condición de rechazo que ningún ADR decide, y un texto libre no es verificable.

**La asimetría entre T31 y T27** dice algo que ninguna de las dos filas dice sola: desde `RETIRADO_MERCADO` el custodio **puede** disponer finalmente la unidad pero **no** puede reingresarla a stock. Destruir lo retirado es una salida siempre admisible; devolverlo a la circulación es una decisión que solo el titular o la autoridad pueden tomar.

**Cómo se resuelve el actor lógico de un evento extraordinario** (EXT-5): los caracteres de ADR-001 **no son excluyentes** —el laboratorio titular que además custodia la unidad reúne dos—, de modo que cuál aplica no lo decide el código sino la columna «actor habilitado» de la fila que corresponde al estado de origen observado: se toma el primer carácter que esa fila habilita. De ahí sale, sin regla propia, que el destinatario declarado pueda poner en cuarentena (T09) e informar vencimiento (T13) durante el tránsito pero no informar robo, extravío ni deterioro (T14–T16). `RECOVERY_OR_DISPOSAL_AGENT` se resuelve como el custodio actual registrado ([ADR-009](../docs/adr/009-return-and-recovery-semantics.md) punto 3), que es lo que hace alcanzable T25. Los dos rechazos no son intercambiables: `UNAUTHORIZED_CUSTODIAN` cuando el invocador no reúne **ningún** carácter, `INVALID_STATE_TRANSITION` cuando reúne alguno que esa fila no habilita — ahí el problema no es quién pide, es desde dónde.

Antes de EXT-5 esa habilitación vivía en dos listas de eventos escritas a mano. La granularidad estaba mal: `REINGRESAR_STOCK` habría resuelto a `LABORATORY` también en T25 y T26, que ADR-001 reserva al custodio — el mismo defecto que EXT-6 tuvo que corregir en sentido inverso para T17. La tabla ya expresa la regla por transición, que es donde ADR-001 la decide.

**Alcance de `WithdrawFromMarket` en tránsito** (DES-19, #116): el retiro desde `EN_TRANSITO` está reservado a la organización regulatoria. El laboratorio titular conserva T17, T18 y el retiro desde `EN_CUARENTENA` o `DEVUELTO`, pero no el origen `EN_TRANSITO`: salir de ese estado obliga a cerrar el registro de la operación en la colección privada del par ([ADR-007](../docs/adr/007-network-topology.md) punto 6.c) y [ADR-006](../docs/adr/006-private-data-collections.md) punto 1 limita su membresía a `{emisor, receptor, regulador}`, de modo que un laboratorio ajeno al par no puede cerrarla. [ADR-001](../docs/adr/001-maquina-estados-medicamento.md) habilitaba ambos actores sobre ese origen; su revisión 2 parte la fila `T19` en dos y resuelve la contradicción a favor de la membresía de las colecciones, que es la propiedad de confidencialidad que el trabajo demuestra. El rechazo del laboratorio en tránsito es `INVALID_STATE_TRANSITION`: proviene de la tabla de ADR-001, no de la autorización de intervención.

## Mecanismos de endoso implementados

`internal/snt/endorsement.go` concentra los dos mecanismos de plataforma de ADR-007 punto 6:

- **State-based endorsement por clave** (`setKeyEndorsement`), para los requisitos derivables del estado confirmado. Con un solo `mspId` la política exige a esa organización; con varios, `statebased` construye la conjunción — la semántica que necesita `AND(emisor, receptor)` durante el tránsito. **Ninguna política de clave de unidad admite a la organización regulatoria como rama alternativa**: la política es de la clave y no de la función, y una rama disyuntiva agregada para un caso excepcional habilitaría con la misma fuerza todos los casos ordinarios.
- **Marcador de participación** en la colección implícita de una organización, en sus dos variantes (`Unidad` y `Organizacion`). Es la única forma nativa de exigir el endoso de una organización que no es titular de la clave escrita, o de exigirlo en la **primera** escritura de una clave, donde SBE todavía no puede aplicarse. El `txId` va último en la clave: la hace única por transacción, sin contención MVCC.

### Salida de `EN_TRANSITO` por evento extraordinario

`CloseTransitForExtraordinaryEvent` es el mecanismo que CC-3 (#16) deja listo para las issues EXT, que implementan las operaciones (T09, T13–T16). Compone las **tres** piezas que ADR-007 exige, para que ninguna quede afuera por olvido:

1. el **marcador de participación** en la colección implícita de la organización regulatoria, **sólo cuando es ella quien inicia el evento** (punto 6.d). Es el segundo uso del marcador —exigir el endoso de una organización que no es titular de la clave escrita—, distinto del que cierra la ventana de creación de una clave pública nueva (punto 6.g). Sin él, la participación de ANMAT descansaría en su firma de creador, que acredita identidad pero no prueba que ningún peer suyo haya ejecutado la lógica;
2. el **cierre del registro de operación**: histórico + `DelPrivateData` de la clave activa;
3. la **restauración de la política de reposo** hacia el emisor, que sigue siendo el custodio registrado porque el tránsito no se consumó (punto 6.c).

Escribir el marcador siempre —y no sólo cuando invoca el regulador— convertiría a `AnmatMSP` en coendosante obligatoria de eventos que no inició, que es exactamente lo que DES-6 prohíbe. `TestExtraordinaryExitByCustodianWritesNoRegulatoryMarker` lo deja fijado.

### Receptor equivocado vs. dato privado no diseminado

Ambos casos se ven igual desde el contenido privado: la clave `TransferOpActive` no está. El chaincode los separa con el **hash público** que Fabric persiste por cada escritura privada (ADR-006, punto 6), legible con `GetPrivateDataHash` desde cualquier peer sin exigir membresía en la colección ni que el dato se haya diseminado:

- **hay hash vivo** → la operación existe y su contenido todavía no llegó a este peer: `INTERNAL_ERROR` con `reintentable: true`, la falla transitoria que ADR-006 punto 1 obliga a contemplar;
- **no hay hash** → en esa colección nunca hubo operación, de modo que el invocador no es el destinatario declarado: `RECEIVER_MISMATCH`, definitivo y no reintentable.

Cerrar una operación elimina también esa entrada del estado público, así que un registro histórico no deja hash vivo y no puede confundirse con una operación en curso.

**El orden de las dos consultas no es intercambiable, y por eso vive dentro de `readActiveTransferOperation` y no en cada llamador.** Fabric no se comporta como un mapa: el *query helper* del peer compara la versión del hash público con la del dato privado y, cuando difieren —hash confirmado, contenido todavía no reconciliado—, la lectura **falla** con `private data matching public hash version is not available`; no devuelve vacío. Consultar el hash *después* de una lectura privada que se asume vacía nunca llegaría a ejecutarse: el error sepultaría la condición transitoria bajo un `INTERNAL_ERROR` genérico, indistinguible de cualquier otra falla de plataforma. Por eso la función consulta primero el hash, sólo entonces lee el contenido, y convierte el fallo de esa lectura en la condición tipificada, conservando el mensaje original de Fabric en `details.causaSubyacente`.

`TestMockStubReproducesFabricPrivateDataSemantics` fija esa semántica en el doble de prueba, porque de ella depende que los tests de la condición transitoria prueben algo: si el mock devolviera `(nil, nil)` con el hash presente, el chaincode podría apoyarse en un camino que en la red real no se recorre y los tests pasarían igual.

`TestPublicKeyCreationWritesMarker` cubre la invariante de ADR-007 punto 6.j: toda operación que crea una clave pública nueva escribe también el marcador de la organización responsable. Hoy son exactamente tres — `RegisterUnit` (laboratorio invocante), `RegisterOrganization` y `AuthorizeLabIntervention` (organización regulatoria) — y el test las cubre a las tres. Mientras se cumpla, la política de chaincode `OR(custodiales, regulatoria)` no es una frontera de seguridad; una operación futura que cree una clave pública sin marcador reabriría una ventana de creación sin dueño.

El campo `pendingOwner` de ese test es la maquinaria que CC-1 (#14) dejó para las operaciones todavía no implementadas: mientras una siga devolviendo el error de stub de su issue dueña, su caso se saltea; el día que esa issue le ponga lógica, el caso deja de saltearse **por sí solo** y exige el marcador. Con `RegisterUnit` ya implementada ningún caso lo usa hoy, pero se conserva para las operaciones que CC-3 (#16) y siguientes agreguen a la lista, de modo que la invariante quede cubierta por un mecanismo y no por la convención de acordarse de agregar la fila.

`TestCompositeKeySchema` pinnea el esquema de claves compuestas que CC-1 fija para las issues que lo consumen: los tipos de objeto, el orden de los componentes y el `txId` al final en las dos variantes del marcador.

## Verificación de autenticidad del adquirente (`VerifyUnit`)

`VerifyUnit` ([`internal/snt/verify.go`](internal/snt/verify.go)) materializa la obligación que la Disposición ANMAT 3683/2011 impone al miembro de la cadena que adquiere. Su semántica la fija [ADR-013](../docs/adr/013-acquirer-authenticity-verification.md): cuatro comprobaciones determinísticas evaluadas en orden —existencia, unicidad, cadena de custodia legítima y aptitud del estado actual— con un veredicto estructurado.

**No es `VerifyTrace` con otro nombre**, y la distinción es la razón de que exista. La checklist de [ADR-011](../docs/adr/011-financier-trace-verification.md) exige que la unidad esté `DISPENSADO` y devuelve `NO_DISPENSADA` en cualquier otro caso: es correcta para el financiador, cuya condición de pago nace de una dispensa ya ocurrida, e inservible para el adquirente, que consulta **antes** de aceptar la custodia —cuando la unidad está justamente en `EN_TRANSITO` o `EN_CUSTODIA`—. Aplicarle la checklist del financiador respondería `NO_DISPENSADA` en el 100 % de sus consultas legítimas.

Lo que **sí** comparten son las dos comprobaciones de cadena de custodia (camino de estados contra ADR-001 y pares autorizados contra la matriz de ADR-008), y por eso están implementadas **una sola vez**, en `verifyCustodyChain`. `VerifyTrace` (CC-8, #62) consume ese helper en lugar de reescribirlo: dos implementaciones de la misma regla divergirían en silencio, y el día que lo hicieran el adquirente y el financiador darían veredictos distintos sobre la misma unidad. `TestVerifyCustodyChainIsSharedWithVerifyTrace` ejercita el helper directamente para dejar constancia de que es una pieza con contrato propio.

**Las comprobaciones de cadena verifican ADR-001 y ADR-004 juntas, no por separado.** La legitimidad de una transferencia no es el producto de dos condiciones independientes: ADR-004 acopla estado y custodia — el despacho lleva la unidad a `EN_TRANSITO` **sin** mover `CustodioActual`, y solo la recepción (T04) lo mueve. Verificar «los estados forman un camino declarado» por un lado y «los pares de agentes están autorizados» por otro deja pasar historiales que violan el acoplamiento aunque ambas proyecciones sean válidas: `EN_LABORATORIO/laboratorio → EN_TRANSITO/droguería` es una transición declarada con un par autorizado, y aun así describe una custodia que se movió durante el tránsito. Que T04 sea la única transición que mueve la custodia no es interpretación: ADR-009 (punto 1) lo confirma para las cuatro vías hacia `DEVUELTO` y descarta expresamente la alternativa que la cambiaba.

**La aptitud para operar mira la fecha, no solo el estado.** El paso del tiempo no ejecuta transacciones: `VENCIDO` se alcanza por T11/T12/T13, que alguien tiene que invocar, y hasta entonces una unidad cuya `fechaVencimiento` ya pasó sigue registrada como `EN_CUSTODIA`. Declararla apta sería el peor resultado posible de esta operación — decirle a quien está por adquirir que un producto vencido es apto, con la fecha que lo desmiente en el mismo estado público que la verificación ya está leyendo. Tiene veredicto propio (`VENCIDO_POR_FECHA`) y no se reporta como `ESTADO_BLOQUEANTE`, porque son acciones distintas: ante un estado bloqueante el ledger ya registró la causa; acá el adquirente está descubriendo una condición no informada y corresponde además detonar `ReportExpired` (T13). La comparación sale siempre de `GetTxTimestamp()`, nunca del reloj local, y `fechaVencimiento` es el **último día operable**.

Dos propiedades que el código sostiene y los tests verifican en lugar de afirmar:

- **No lee datos privados.** El veredicto se computa solo sobre el estado mínimo de trazabilidad que ADR-002 declara de visibilidad amplia. `TestVerifyUnitReadsNoPrivateData` inyecta una falla en toda lectura privada del stub y exige que la operación siga funcionando: si algún día tocara una colección, el test lo dice.
- **La autorización no finge ser una barrera.** Se exige invocador registrado y habilitado, pero **no** `agentType` ni `snt.role`, a diferencia de `VerifyTrace`. Restringirlo sería aparente: la misma información es alcanzable con `ReadUnit` y `GetUnitHistory`, que no autorizan en absoluto porque ADR-005 declara que la lectura del estado público no es restringible por chaincode. Una barrera que no detiene nada es peor que ninguna, porque induce a confiar en ella.

Y un límite que conviene repetir donde se lea el código, porque la palabra «autenticidad» sugiere más de lo que el ledger puede acreditar: **un envase falsificado que reproduzca un GTIN + serie legítimo obtiene `autentica: true`**. La verificación acredita la traza registrada, no el envase. La lista completa está en «Límites de la verificación» de ADR-013.

## Verificación de trazabilidad del financiador (`VerifyTrace`)

`VerifyTrace` ([`internal/snt/verify.go`](internal/snt/verify.go)) es la operación con la que el organismo financiador satisface su condición de pago. [ADR-005](../docs/adr/005-rol-organismo-financiador.md) la modeló como consulta claim-driven y dejó su semántica deliberadamente abierta; [ADR-011](../docs/adr/011-financier-trace-verification.md) la cierra con cinco comprobaciones determinísticas evaluadas en orden —existencia, estado `DISPENSADO`, dispensador habilitado, camino de estados y pares autorizados— y un veredicto estructurado. La misma operación le sirve a ANMAT para auditoría.

**A diferencia de `VerifyUnit`, esta operación sí restringe el acceso.** El financiador no es un eslabón de la cadena y su consulta no es la lectura del estado público que ADR-005 declara no restringible: es un veredicto normativo sobre una unidad que puede no tener ninguna relación con el invocador. Se admite `agentType=FINANCIER` con `snt.role=financier-auditor`, o `agentType=REGULATOR` con `auditor` o `regulatory-admin` ([ADR-010](../docs/adr/010-non-custodial-identity.md)), siempre resuelto contra el registro organización-establecimiento y nunca contra literales de MSP. Un invocador registrado y activo cuyo `agentType` no habilita la operación recibe `UNAUTHORIZED_AGENT_TYPE`, que es un rechazo de autorización y **no** un veredicto de traza: por eso no figura entre los valores de `motivo`.

**Las comprobaciones 4 y 5 se evalúan en dos pasadas, en ese orden.** ADR-011 las declara como comprobaciones sucesivas y fija que `motivo` nombre «la primera comprobación que falla, en el orden declarado». Una sola pasada intercalada devolvería la primera violación por **posición** en el historial, que no es lo mismo: un historial con un par no autorizado temprano y una secuencia inválida posterior debe reportar `SECUENCIA_INVALIDA`, porque la comprobación 4 precede a la 5 y falla. `TestVerifyTraceEvaluationOrder` discrimina entre ambas implementaciones. El acoplamiento estado-custodia de ADR-004 pertenece a la primera pasada, de modo que ADR-001 y ADR-004 siguen verificándose juntas.

**Límites declarados** (ADR-011, «Límites de la verificación»), que el trabajo escrito debe citar como limitaciones conscientes: la verificación no valida la habilitación **histórica** de los actores —el registro de ADR-003 persiste `active` actual y no versiona habilitaciones—, no distingue versiones históricas de la matriz —ADR-008 declara matriz única para toda la evaluación de v1—, no ve transacciones rechazadas —`GetHistoryForKey` solo devuelve modificaciones confirmadas— y no puede comprobar que el serial corresponda a un afiliado del financiador invocante, porque ese vínculo es off-ledger. El quinto es el que hereda de los «Límites de garantía» de [ADR-003](../docs/adr/003-establishment-identity-gln-cufe.md): **acredita la traza registrada, no la autenticidad física del producto** — ni la posesión física efectiva, ni la autenticidad material del envase, ni la ausencia de clonación del código serializado. Un veredicto `legitima: true` dice que el ledger registra una cadena de custodia impecable para ese GTIN + serie, no que el envase que alguien tiene en la mano sea ése.

**Confidencialidad y datos personales.** El veredicto se computa exclusivamente sobre el estado mínimo de trazabilidad y el registro de organizaciones: la operación no lee ninguna colección privada, de modo que no puede exponerle al financiador información comercial de operaciones de las que no es parte. Es una propiedad estructural, no una promesa. Tampoco recibe ni devuelve dato alguno del afiliado (Ley 25.326).

## Smoke test local sobre la TEST-NETWORK

[`test/integration/chaincode-e2e.sh`](../test/integration/chaincode-e2e.sh) conserva la cobertura histórica de CC-1 (#14) contra `fabric-samples/test-network` como prueba local opcional. Hace el ciclo de lifecycle completo a mano porque esa red no contiene `AnmatMSP` y solo puede comprobar el rechazo `REGULATORY_ONLY`.

El workflow de integración principal usa ahora la red propia: `network/network.sh deployCC` ejecuta `Init` exitosamente sobre `snt-channel`, confirma las secuencias 1 y 2 y siembra el registro. [`test/integration/pdc-evidence.sh`](../test/integration/pdc-evidence.sh) agrega evidencia de plataforma sobre colecciones privadas mediante un probe separado del contrato productivo.

**Qué se puede probar ahí, y por qué no más que eso.** El chaincode se despliega con `--init-required` (ADR-007, punto 5.c), de modo que la primera —y, mientras `Init` no tenga éxito, la única— transacción que el peer admite es `Init`. Y `Init` no acepta el `mspId` regulatorio como argumento: lo resuelve contra el manifiesto fundacional embebido, que declara `AnmatMSP` (ADR-010, punto 4). La `test-network` de fabric-samples tiene `Org1MSP` y `Org2MSP`.

Sobre la red estándar y con el artefacto **real** —sin manifiestos adulterados para CI, que romperían justamente la propiedad de «un único artefacto construido una sola vez» de ADR-008 punto 5— la única respuesta posible del chaincode es el rechazo tipificado `REGULATORY_ONLY`. Ese rechazo **es** la evidencia buscada: para devolverlo, el chaincode tuvo que empaquetarse, instalarse, arrancar su contenedor, ejecutar Go, resolver el manifiesto embebido por `go:embed`, resolver `cid.GetMSPID()` y serializar un error del catálogo del contrato. Un chaincode que no desplegara, o que desplegara roto, devolvería un error de plataforma y no un objeto JSON con `code`.

El script comprueba, en orden:

1. `make package-reproducible`: el `packageID` no cambia entre dos empaquetados, y no quedó ningún `.tar.gz` dentro de `chaincode/`;
2. el `packageID` figura instalado en los peers de ambas organizaciones (`queryinstalled`);
3. la definición queda confirmada con `init_required: true` (`querycommitted`), bajo la política estricta `AND` de las organizaciones del canal que pide ADR-007 punto 5.c;
4. la plataforma **rechaza** cualquier función distinta de `Init` antes de inicializar — lo que distingue un despliegue con `--init-required` de uno sin él;
5. la invocación dummy de `Init` responde con el error tipificado `REGULATORY_ONLY`.

El `Init` **exitoso** —y con él, el seed del registro— se prueba en la red de NET-4. Las operaciones funcionales de transferencia sobre ella permanecen en CC-3 (#16).

## Tests

CC-6 (#19) es dueña de la batería:

- **mocks del `ChaincodeStub`** y de la identidad del cliente (`internal/snt/mocks_test.go`), con world state, datos privados —incluida la capa de hashes públicos y la semántica real de `GetPrivateData`—, políticas de endoso por clave, transaction log e **inyección de fallas de plataforma** por método, para poder ejercitar las ramas `INTERNAL_ERROR`;
- **un camino feliz por operación implementada**, inventariado en `TestHappyPathInventory`;
- **un escenario por cada código del catálogo de errores** del contrato, en `TestErrorCatalogIsCovered`. Los códigos que las operaciones implementadas todavía no pueden producir se declaran con la issue que los habilitará, de modo que la tabla nunca queda muda sobre uno de ellos — hoy solo `LAB_INTERVENTION_REQUIRED`, que aparece con las issues EXT;
- cobertura por encima del 80 % y suite limpia con `-race`.

Los mocks embeben la interfaz de Fabric en lugar de implementarla entera: un método que un test use sin estar implementado entra en pánico de forma evidente, en vez de devolver un cero silencioso.

### Lo que la implementación devuelve vs. lo que el contrato declara

`TestProducedErrorsAreDeclaredByTheContract` cierra un hueco que las otras comprobaciones dejaban abierto entre sí: `TestErrorCatalogIsCovered` exige que cada código del catálogo tenga **algún** escenario, sin mirar de qué operación sale, y `TestContractSignaturesMatchFrozenContract` compara firmas, no errores. Entre las dos, una operación podía devolver de forma estable un código que su sección del contrato no nombraba.

Es exactamente lo que le pasaba a `Dispense` y a `RejectTransfer` con `ORG_NOT_REGISTERED` y `ORG_INACTIVE` hasta la v2.6.2: ambas resuelven la identidad del invocador contra el registro de ADR-003 —lo exigen #17 y DES-6— y por lo tanto pueden rechazar porque la organización no tiene entrada o no está habilitada, pero sus listas de errores no lo declaraban. El test recorre las cinco operaciones custodiales, produce las dos condiciones transversales de DES-6 y falla si el contrato no las declara para esa operación. `INTERNAL_ERROR` queda deliberadamente fuera: el catálogo lo define como el error no clasificable de cualquier operación y el contrato no lo repite en cada lista.

`TestContractVersionMatchesFrozenContract` impide, por lo mismo, que `ContractVersion` y el encabezado del documento se separen: esa constante viaja al peer como `Info.Version` y aparece en los mensajes de los tests de firma, de modo que si citara una versión inexistente todo el andamiaje de congelamiento estaría hablando de un contrato que no existe.

## Desarrollo

```bash
cd chaincode
make build      # compila chaincode y paquete compartido
make test       # tests con -race de ambos módulos
make cover      # cobertura del chaincode
make lint       # golangci-lint (config en .golangci.yml de la raíz)
```

`.golangci.yml` **no** habilita `misspell`: solo trae diccionarios de inglés y la prosa del repositorio está en castellano, con lo que reportaba más de 150 falsos positivos (`transaccion`, `organizacion`, `EN_TRANSITO`…). El detalle está comentado en el propio archivo.
