# Chaincode `snt`

Smart contract Go del prototipo, implementado con `contractapi`. Su interfaz pública está congelada en [`docs/api-contract.md`](../docs/api-contract.md) (v2.6.1); el chaincode se llama `snt` y el canal `snt-channel` ([ADR-007](../docs/adr/007-network-topology.md), punto 4).

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
| `ReadUnit`, `GetUnitHistory`, `QueryUnitsByGTIN` | Declaradas | CC-5 (#18) |
| `Quarantine`, `ReleaseQuarantine` | Declaradas | EXT-1 (#27) |
| `ReportExpired` | Declarada | EXT-2 (#28) |
| `ReportStolen`, `ReportLost`, `ReportDamaged` | Declaradas | EXT-3 (#29) |
| `ReturnProduct` | Declarada | EXT-4 (#30) |
| `Restock` | Declarada | EXT-5 (#31) |
| `WithdrawFromMarket`, `ProhibitProduct` | Declaradas | EXT-6 (#32) |
| `FinalDisposition` | Declarada | EXT-8 (#63) |
| `VerifyTrace` | Declarada | CC-8 (#62) |

Las operaciones declaradas devuelven `INTERNAL_ERROR` con el detalle `{"operacion": …, "issue": …}`. El catálogo del contrato no tiene un código para «operación declarada sin implementar», y agregarlo sería un cambio MINOR del contrato que una issue de implementación no puede hacer.

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

## Desarrollo

```bash
cd chaincode
make build      # compila chaincode y paquete compartido
make test       # tests con -race de ambos módulos
make cover      # cobertura del chaincode
make lint       # golangci-lint (config en .golangci.yml de la raíz)
```

`.golangci.yml` **no** habilita `misspell`: solo trae diccionarios de inglés y la prosa del repositorio está en castellano, con lo que reportaba más de 150 falsos positivos (`transaccion`, `organizacion`, `EN_TRANSITO`…). El detalle está comentado en el propio archivo.
