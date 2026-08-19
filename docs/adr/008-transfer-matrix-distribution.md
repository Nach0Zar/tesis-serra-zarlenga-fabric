# ADR-008: Distribución y versionado de la matriz de transferencias en chaincode y baseline

- **Estado**: Propuesto
- **Fecha**: 2026-08-17
- **Autores**: Serra, Zarlenga

---

## Contexto

`domain/README.md` establece que `domain/authorized-transfers.json` es la fuente única de verdad para validar pares origen → destino en transferencias ordinarias de custodia, y exige que tanto el chaincode como la baseline centralizada la consuman **sin duplicar las reglas mediante `if` o `switch` mantenidos manualmente en cada implementación**. Ante el mismo par de agentes, ambas implementaciones deben devolver la misma decisión, el mismo identificador de regla cuando corresponda y una razón equivalente de rechazo. Ningún documento había decidido, sin embargo, el mecanismo concreto por el cual ambos prototipos obtienen la matriz.

La decisión no es solo de empaquetado; condiciona tres propiedades del diseño:

1. **Determinismo de endoso.** `DispatchTransfer` valida el par origen → destino contra la matriz y falla con `TRANSFER_NOT_AUTHORIZED` cuando el par no está permitido (`api-contract.md`). En Fabric, esa validación la ejecutan todos los peers endosantes (origen y destino de la transferencia, según DES-6), y sus resultados deben coincidir byte a byte para que la transacción sea válida. Si dos endosantes pudieran leer versiones distintas de la matriz, el endoso divergiría.
2. **Auditoría histórica.** ADR-005 dejó escrita la pregunta exacta que esta ADR debe responder: al verificar una traza, ¿una transferencia se juzga por si "fue autorizada según la matriz de DES-3 **vigente al momento de esa transferencia**"? Esa pregunta solo es verificable si queda registrado qué regla, y de qué versión de la matriz, autorizó cada movimiento. La decisión D6/ADR-011 (criterios de verificación del financiador, issue #86) depende de esta respuesta.
3. **Paridad de la comparación.** El protocolo de medición exige paridad funcional entre chaincode y baseline; si cada implementación interpretara la matriz por su cuenta, cualquier diferencia de resultados en la evaluación sería atacable.

Esta ADR resuelve la decisión D3 de `docs/adr-roadmap.md` (issue #83) y desbloquea CC-3 (#16), BASE-2 (#38) y la semántica de verificación de DES-17/ADR-011 (#86).

El algoritmo de decisión ya está fijado por `domain/README.md` y no se rediscute acá: coincidencia exacta en `authorizedTransfers` → autorizar devolviendo el `id` de la regla; si no, coincidencia con una prohibición explícita de `prohibitedTransfers` → denegar devolviendo su `id`; en cualquier otro caso → denegar por `defaultDecision` con razón `DEFAULT_DENY`. La matriz vigente es `schemaVersion` `1.0.0`, `rulesetId` `PFI_SNT_AUTHORIZED_TRANSFERS`, con versionado semántico del propio archivo (`PATCH`/`MINOR`/`MAJOR` según impacto).

## Alternativas

**A. Matriz persistida en el ledger, administrada por `AnmatMSP` con `regulatory-admin`**

- La matriz vive como estado del ledger; actualizarla es una transacción regulatoria, sin recompilar ni actualizar el chaincode.
- Es el camino natural si apareciera un requisito de actualización "en caliente": la historia de versiones de la matriz quedaría en el propio ledger, y la re-evaluación histórica de una traza podría leer la versión vigente a la altura de cada transacción.
- Exige diseñar piezas que el prototipo no necesita: una operación de administración de matriz, un esquema de versionado on-ledger, validación estructural del JSON dentro del chaincode y control de acceso adicional sobre esa operación. Durante toda la evaluación del prototipo v1 existirá una única versión de matriz (ver Decisión, punto 4), por lo que toda esa maquinaria no ejercitaría más que su caso trivial.
- Además, la baseline centralizada no tiene ledger: necesitaría un mecanismo paralelo de distribución, reabriendo el riesgo de divergencia que `domain/README.md` prohíbe.
- Se descarta para v1; queda señalada como evolución natural en una revisión futura de esta ADR si el requisito de actualización sin upgrade apareciera.

**B. `go:embed` del JSON + paquete Go compartido entre chaincode y baseline**

- El JSON se embebe en el binario en tiempo de compilación; todos los peers endosantes ejecutan el mismo binario con la misma matriz, con determinismo garantizado por construcción.
- Un único paquete Go del repositorio embebe el archivo y expone la función de decisión; chaincode y baseline lo importan, de modo que la paridad no depende de disciplina de los implementadores sino de compartir literalmente el mismo código.
- Costo: actualizar la matriz exige recompilar y actualizar el chaincode (nueva secuencia del ciclo de vida) y recompilar la baseline. Aceptable para v1, donde la matriz no cambia durante la evaluación.
- Se adopta.

**C. Archivo leído del filesystem en tiempo de ejecución**

- Evitaría recompilar ante cambios de matriz.
- Rompe el empaquetado del chaincode: el paquete que se instala y aprueba en el ciclo de vida de Fabric dejaría de contener la matriz que efectivamente se evalúa, y nada garantizaría que todos los peers endosantes lean el mismo archivo — exactamente la fuente de no determinismo que el punto 1 del Contexto obliga a eliminar.
- Se descarta porque sacrifica la garantía de que todos los endosantes evalúan la misma matriz, sin aportar un beneficio que el prototipo necesite.

## Decisión

Se adopta la **alternativa B**, con las siguientes reglas:

1. **Distribución por `go:embed`.** El chaincode embebe `domain/authorized-transfers.json` en su binario en tiempo de compilación mediante la directiva `//go:embed`, lo parsea una única vez en la inicialización del contrato y evalúa cada despacho con el algoritmo exacto de `domain/README.md`: coincidencia exacta en `authorizedTransfers` → autorizar con el `ruleId` de la regla; coincidencia con una prohibición explícita → denegar con el `id` de la prohibición; en cualquier otro caso → denegar con razón `DEFAULT_DENY`. El determinismo entre peers **no** lo garantiza el lifecycle: el `packageID` es un parámetro local de `approveformyorg` y Fabric no exige que coincida entre organizaciones (ver ADR-010, punto 4), de modo que dos organizaciones pueden aprobar la misma definición con binarios distintos. Lo que `go:embed` sí garantiza es que **un binario dado evalúa siempre la misma matriz**, sin depender del filesystem ni de estado externo. La igualdad *entre* binarios se obtiene por el punto 5.
2. **Paridad por construcción con la baseline.** El chaincode y la baseline (que será Go, conforme ADR-012/D7) consumen la matriz a través de un **paquete Go compartido del repositorio** — por ejemplo, `domain/` como módulo o paquete importable que embebe el JSON y expone la función de decisión. Ninguna de las dos implementaciones reescribe reglas: misma función, misma decisión, mismo `ruleId`, misma razón de rechazo — que es literalmente lo que exige `domain/README.md`. La denegación se materializa en el contrato como `TRANSFER_NOT_AUTHORIZED` (`api-contract.md`), con el mismo `code` en ambas implementaciones.
3. **Registro de la regla aplicada.** Cada despacho persiste en el registro de operación de la PDC (ADR-006, decisión D1/issue #81) el `ruleId` y la `schemaVersion` de la matriz que autorizó el par. Esto deja evidencia auditable — por ANMAT y por las partes de la operación — de qué regla habilitó cada movimiento, y es el dato que hace verificable la pregunta de ADR-005.
4. **Versionado.** Actualizar la matriz significa: nueva versión del JSON en el repositorio (versionado semántico del propio archivo, según `domain/README.md`) + recompilación y upgrade del chaincode (nueva secuencia del ciclo de vida de Fabric) + rebuild de la baseline. Para el prototipo v1 se declara que existirá **una sola versión de matriz durante toda la evaluación** (lo que hace además trivialmente comparable la `schemaVersion` de la comprobación cruzada del punto 5), por lo que la validación histórica ("¿la transferencia cumplía la matriz vigente en su momento?") es trivialmente equivalente a validar contra la matriz actual. La verificación del financiador (ADR-011) puede, por lo tanto, re-evaluar pares del historial contra la matriz embebida vigente, con la **limitación declarada**: si en el futuro la matriz cambiara entre eventos, la re-evaluación histórica exigiría o bien conservar embebidas las versiones anteriores, o bien mover la matriz al ledger (alternativa A). Eso queda como revisión futura de esta ADR, no como problema de v1.

5. **Un artefacto, verificado, y una comprobación cruzada en la recepción.** La igualdad de matriz entre organizaciones se sostiene sobre dos mecanismos, y conviene no confundir su fuerza:

   - **Verificación operativa** (misma que ADR-010, punto 4): el paquete se construye una sola vez y se distribuye idéntico; el repositorio versiona el `packageID` y el checksum esperados; NET-4 (#23) demuestra con `queryinstalled`/`queryapproved` que cada organización instaló exactamente ese artefacto. Es un control de despliegue, no una garantía de plataforma.
   - **Comprobación cruzada en la recepción**: el problema que la verificación operativa no cubre es que `DispatchTransfer` lo endosa **solo el emisor** (política de reposo, ADR-007 punto 6.a), de modo que una matriz divergente en *ese* peer autorizaría un par que ninguna otra organización contrastó. Se cierra aprovechando que la recepción (T04) sí exige `AND(emisor, receptor)`: el peer del **receptor** re-evalúa el par origen → destino contra **su propia** matriz embebida y lo compara con el `ruleId` y la `schemaVersion` persistidos en el registro de operación (punto 3). Si no coinciden, su endoso difiere del del emisor, los read-write sets no cuadran y la transferencia **no puede confirmarse**. Con eso, ningún cambio de custodia se consuma sin que **dos binarios independientes** hayan coincidido en que el par estaba autorizado.

     El costo es nulo en superficie de contrato —no agrega operaciones ni códigos de error nuevos: la divergencia se manifiesta como imposibilidad de reunir endosos coincidentes, igual que cualquier otro no determinismo— y la ventana que queda abierta es acotada y declarable: entre el despacho y la recepción, una unidad puede estar en `EN_TRANSITO` bajo un par que solo el emisor validó. CC-3 (#16) implementa la re-evaluación y NET-6 (#25) la demuestra con un peer que corre una matriz alterada a propósito.

Queda fuera del alcance de esta ADR el layout Go concreto del paquete compartido (módulo único del repositorio vs. módulos separados con directiva `replace`); se deriva a CC-1.

## Justificación

- **Determinismo primero, sin sobreestimar lo que da la plataforma.** La propiedad no negociable es que todos los endosantes lleguen a la misma decisión. Embeber la matriz elimina el no determinismo *dentro* de un binario: las reglas viajan en el artefacto que se instala, sin depender del filesystem del peer (alternativa C) ni de un estado a sincronizar (alternativa A). Lo que **no** da es igualdad entre binarios, porque el lifecycle de Fabric no obliga a que las organizaciones aprueben el mismo `packageID`; una versión anterior de este ADR lo daba por sentado. Esa igualdad se obtiene con el control de despliegue y la comprobación cruzada del punto 5, y la diferencia entre «garantizado por la plataforma» y «verificado en el despliegue» se declara en lugar de disimularse.
- **La paridad exigida por `domain/README.md` se obtiene por construcción, no por convención.** Un paquete compartido que embebe el JSON y expone la única función de decisión elimina de raíz la posibilidad de que chaincode y baseline diverjan en un par: no hay dos implementaciones que mantener consistentes, hay una sola que ambos binarios importan. Es la materialización más directa del mandato de no duplicar pares en condicionales.
- **El costo de la alternativa adoptada es el que el prototipo puede pagar.** El precio de `go:embed` es que cambiar la matriz requiere upgrade de chaincode. Como el prototipo v1 declara una única versión de matriz durante toda la evaluación, ese precio no se paga nunca dentro del alcance; diseñar administración on-ledger (alternativa A) habría sido pagar por adelantado una flexibilidad sin caso de uso en v1.
- **La pregunta de ADR-005 queda respondida sin sobre-ingeniería.** Persistir `ruleId` + `schemaVersion` por despacho (punto 3) registra qué norma habilitó cada movimiento; la declaración de matriz única (punto 4) hace que "matriz vigente al momento de la transferencia" y "matriz actual" coincidan durante la evaluación. La limitación de esa equivalencia queda declarada explícitamente, en el mismo espíritu de límites documentados de ADR-003 y ADR-005, en lugar de resolverse con mecanismos que v1 no ejercitaría.

## Consecuencias

- **Para CC-3 (#16) y BASE-2 (#38)**: ambas implementaciones consumen el paquete compartido de `domain/`; ninguna define pares por su cuenta. Los tests de paridad pueden ejercitar la misma función contra los mismos casos. CC-3 implementa además la **re-evaluación del par en la recepción** contra el `ruleId`/`schemaVersion` persistidos (punto 5).
- **Para NET-4 (#23) y NET-6 (#25)**: NET-4 verifica con `queryinstalled`/`queryapproved` que todas las organizaciones instalaron el `packageID` versionado, y guarda la salida como evidencia; NET-6 demuestra que un peer con una matriz alterada a propósito no puede completar una recepción.
- **Para ADR-006 (D1, #81)**: el registro de operación de la PDC incorpora los campos `ruleId` y versión de matriz del despacho que la creó.
- **Para DES-17/ADR-011 (#86)**: hereda la simplificación de matriz única — la re-evaluación histórica de pares se hace contra la matriz embebida vigente — junto con su límite declarado (punto 4 de la Decisión).
- **Para CLI-3 (#36)**: el generador del dataset puede usar el mismo paquete para producir únicamente transferencias válidas (o inválidas a propósito), sin duplicar la matriz.
- **Se gana**: determinismo de endoso por construcción, paridad chaincode/baseline garantizada por código compartido y no por disciplina, y trazabilidad auditable de qué regla y qué versión de matriz autorizó cada despacho.
- **Se pierde / costo**: actualizar la matriz exige recompilar y hacer upgrade del chaincode y rebuild de la baseline; la re-evaluación histórica solo es válida mientras la matriz no cambie entre eventos (limitación declarada de v1); y la igualdad de matriz entre organizaciones descansa en un control de **despliegue** verificable (punto 5) y no en una garantía del lifecycle, con una ventana declarada entre el despacho —endosado solo por el emisor— y la recepción, que es donde se contrasta.
- **Queda pendiente**: decidir el layout Go concreto del paquete compartido (módulo único del repo vs. módulos separados con `replace`) en CC-1; y, si un requisito futuro exigiera actualizar la matriz sin upgrade de chaincode o auditar trazas que crucen versiones de matriz, revisar esta ADR evaluando la alternativa A.

## Divergencia con el trabajo escrito

No hay divergencia. El trabajo escrito exige que la validación normativa se traslade al chaincode y que el rechazo de operaciones no autorizadas sea determinístico; esta decisión lo materializa de la forma más directa: las reglas normativas viajan dentro del propio binario del chaincode que todas las organizaciones aprueban, y la evaluación es una función pura sobre datos embebidos e idénticos en todos los peers.

## Contexto utilizado

- Issue GitHub #83: DES-14 · ADR-008: Distribución y versionado de la matriz DES-3 en chaincode y baseline, consultada el 2026-08-17.
- [`domain/README.md`](../../domain/README.md): mandato de fuente única sin duplicación de reglas, algoritmo de decisión y política de versionado de la matriz.
- [`domain/authorized-transfers.json`](../../domain/authorized-transfers.json): matriz vigente (`schemaVersion` 1.0.0, `rulesetId` `PFI_SNT_AUTHORIZED_TRANSFERS`), ids de reglas y prohibiciones.
- [ADR-005: Rol del organismo financiador en la dispensación](005-rol-organismo-financiador.md): origen de la pregunta sobre la matriz "vigente al momento de esa transferencia" (Decisión, punto 2).
- [ADR-006: Mecanismo de colecciones privadas](006-private-data-collections.md): destino del registro de `ruleId` + `schemaVersion` en el registro de operación de la PDC. ADR-006 lo incorporó al contenido del registro de operación (punto 5 de su Decisión).
- [`docs/api-contract.md`](../api-contract.md): `DispatchTransfer` y el error `TRANSFER_NOT_AUTHORIZED`.
- [`docs/adr-roadmap.md`](../adr-roadmap.md): decisión D3, recomendación validada por esta ADR.
- [Documentación del paquete `embed` de Go](https://pkg.go.dev/embed): semántica de la directiva `//go:embed` (archivos embebidos en el binario en tiempo de compilación).
- [Documentación de Hyperledger Fabric — ciclo de vida del chaincode](https://hyperledger-fabric.readthedocs.io/en/latest/chaincode_lifecycle.html): proceso de instalación, aprobación y upgrade por secuencia que implica actualizar la matriz embebida.
