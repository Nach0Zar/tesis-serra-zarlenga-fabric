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
| `RegisterUnit` | Declarada | CC-2 (#15) |
| `DispatchTransfer`, `ReceiveTransfer`, `RejectTransfer` | Declaradas | CC-3 (#16) |
| `Dispense` | Declarada | CC-4 (#17) |
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

`TestPublicKeyCreationWritesMarker` cubre la invariante de ADR-007 punto 6.j: toda operación que crea una clave pública nueva escribe también el marcador de la organización responsable. Mientras se cumpla, la política de chaincode `OR(custodiales, regulatoria)` no es una frontera de seguridad.

El test enumera las **tres** operaciones que hoy crean una clave pública —`RegisterUnit` (laboratorio), `RegisterOrganization` y `AuthorizeLabIntervention` (regulador)—, no solo las dos que CC-1 implementa. El caso de `RegisterUnit` comprueba que la operación siga siendo el stub de CC-2 (#15) y se marca como pendiente; el día que CC-2 le ponga lógica, deja de saltearse **por sí solo** y exige el marcador. La invariante queda así cubierta por un mecanismo y no por la convención de acordarse de agregar la fila.

`TestCompositeKeySchema` pinnea el esquema de claves compuestas que CC-1 fija para las issues que lo consumen: los tipos de objeto, el orden de los componentes y el `txId` al final en las dos variantes del marcador.

## Smoke test sobre la TEST-NETWORK

[`test/integration/chaincode-e2e.sh`](../test/integration/chaincode-e2e.sh) cubre los dos últimos criterios de aceptación de CC-1 (#14) —«despliega sobre la TEST-NETWORK» e «invocación dummy responde»— contra `fabric-samples/test-network`. Lo ejecuta el workflow `Chaincode Integration Test` en cada PR. Hace el ciclo de lifecycle completo a mano (`package`, `install`, `approveformyorg`, `commit`, `invoke`) en lugar de delegarlo en `network.sh deployCC`, porque esa vía no admite `--init-required` sin invocar además la función de init.

**Qué se puede probar ahí, y por qué no más que eso.** El chaincode se despliega con `--init-required` (ADR-007, punto 5.c), de modo que la primera —y, mientras `Init` no tenga éxito, la única— transacción que el peer admite es `Init`. Y `Init` no acepta el `mspId` regulatorio como argumento: lo resuelve contra el manifiesto fundacional embebido, que declara `AnmatMSP` (ADR-010, punto 4). La `test-network` de fabric-samples tiene `Org1MSP` y `Org2MSP`.

Sobre la red estándar y con el artefacto **real** —sin manifiestos adulterados para CI, que romperían justamente la propiedad de «un único artefacto construido una sola vez» de ADR-008 punto 5— la única respuesta posible del chaincode es el rechazo tipificado `REGULATORY_ONLY`. Ese rechazo **es** la evidencia buscada: para devolverlo, el chaincode tuvo que empaquetarse, instalarse, arrancar su contenedor, ejecutar Go, resolver el manifiesto embebido por `go:embed`, resolver `cid.GetMSPID()` y serializar un error del catálogo del contrato. Un chaincode que no desplegara, o que desplegara roto, devolvería un error de plataforma y no un objeto JSON con `code`.

El script comprueba, en orden:

1. `make package-reproducible`: el `packageID` no cambia entre dos empaquetados, y no quedó ningún `.tar.gz` dentro de `chaincode/`;
2. el `packageID` figura instalado en los peers de ambas organizaciones (`queryinstalled`);
3. la definición queda confirmada con `init_required: true` (`querycommitted`), bajo la política estricta `AND` de las organizaciones del canal que pide ADR-007 punto 5.c;
4. la plataforma **rechaza** cualquier función distinta de `Init` antes de inicializar — lo que distingue un despliegue con `--init-required` de uno sin él;
5. la invocación dummy de `Init` responde con el error tipificado `REGULATORY_ONLY`.

Un `Init` **exitoso** —y con él, el seed del registro y las operaciones de negocio— exige una red cuyas MSP sean las del manifiesto fundacional. Esa red la construye NET-4 (#23) sobre `snt-channel`; el escenario funcional completo sobre ella pertenece a CC-3 (#16).

## Desarrollo

```bash
cd chaincode
make build      # compila chaincode y paquete compartido
make test       # tests con -race de ambos módulos
make cover      # cobertura del chaincode
make lint       # golangci-lint (config en .golangci.yml de la raíz)
```

`.golangci.yml` **no** habilita `misspell`: solo trae diccionarios de inglés y la prosa del repositorio está en castellano, con lo que reportaba más de 150 falsos positivos (`transaccion`, `organizacion`, `EN_TRANSITO`…). El detalle está comentado en el propio archivo.
