# ADR-012: Diseño de la línea base centralizada y checklist de paridad funcional

- **Estado**: Propuesto
- **Fecha**: 2026-08-17
- **Autores**: Serra, Zarlenga

---

## Contexto

La comparación cuantitativa contra un sistema centralizado es la mitad de la hipótesis del trabajo: el prototipo Fabric debe mostrar "mejoras medibles respecto al sistema centralizado actual", y el protocolo de medición (`docs/measurement-protocol.md`) exige ejecutar toda conclusión de desempeño y disponibilidad contra una "línea base centralizada funcionalmente equivalente" (§1), bajo condiciones idénticas de host, dataset, carga y tratamiento estadístico (§5). El protocolo ya caracteriza esa baseline como una API sobre base relacional: la latencia write de la baseline termina "cuando la API responde después de confirmar la escritura en la base relacional" (§3.1) y los escenarios de disponibilidad DB-1/API-1 asumen PostgreSQL y una API REST como componentes a derribar (§8.2). También exige que la baseline exponga y mida la transferencia como los mismos dos pasos despacho/recepción de ADR-004 (§3.4): "de lo contrario la comparación no mide los mismos procesos".

Lo que ningún documento había decidido formalmente es el diseño de esa baseline: stack, esquema, emulación de identidad y — sobre todo — qué significa en concreto la "paridad funcional" que el protocolo da por sentada. Sin ese checklist explícito, cualquier diferencia de resultados en la evaluación es atacable con "compararon contra una baseline mal hecha" (`docs/adr-roadmap.md`, decisión D7). Piezas del rompecabezas ya estaban fijadas por decisiones previas: el contrato del chaincode (`docs/api-contract.md`) define el catálogo de códigos de error estables sobre los que "el cliente y la baseline deben ramificar"; ADR-001 obliga a que "el chaincode y la baseline" rechacen toda transición no declarada; `domain/README.md` prohíbe duplicar la matriz de transferencias en condicionales mantenidos a mano; y ADR-008 ya decidió que chaincode y baseline consumen la matriz a través de un paquete Go compartido del repositorio, anticipando que la baseline sería Go "conforme ADR-012/D7". El `baseline/README.md` recoge esos tres mandatos como requisitos del directorio aún vacío.

Esta ADR resuelve la decisión D7 de `docs/adr-roadmap.md` (issue #87, DES-18) y desbloquea las issues BASE-1 (#37, esquema relacional) y BASE-2 (#38, API REST), que están explícitamente bloqueadas por ella.

## Alternativas

**A. Stack distinto del chaincode (Node.js o Python) + PostgreSQL**

- Es el stack más habitual para una API REST de referencia y no exige que el equipo comparta toolchain entre chaincode y baseline.
- Obliga a reimplementar la máquina de estados de ADR-001 y la función de decisión de la matriz (ADR-008) en otro lenguaje: la paridad pasa a depender de disciplina y de tests de equivalencia, exactamente el riesgo que `domain/README.md` prohíbe ("sin duplicar las reglas mediante `if` o `switch` mantenidos manualmente en cada implementación") y que ADR-008 ya eliminó por construcción para la matriz.
- Cualquier divergencia sutil entre implementaciones (un par autorizado de más, una transición aceptada de menos) contamina la comparación y es indetectable sin una batería de tests espejo que habría que mantener por duplicado.
- Se descarta porque pierde el paquete compartido y con él la paridad por construcción; reintroduce el vector de ataque que esta ADR existe para cerrar.

**B. API REST en Go + PostgreSQL, con paquete Go compartido**

- El mismo lenguaje del chaincode permite que la máquina de estados (ADR-001) y la función de decisión de la matriz (ADR-008) vivan en un paquete Go del repositorio que ambos binarios importan: no hay dos implementaciones de las reglas que mantener consistentes, hay una sola.
- PostgreSQL en Docker satisface directamente los escenarios DB-1/API-1 del protocolo (§8.2) y la semántica de confirmación de escritura de §3.1.
- Costo: el integrante responsable de la baseline trabaja en Go aunque no sea su stack preferido, y el layout de módulos del paquete compartido debe diseñarse (queda derivado, ver Decisión).
- Se adopta.

**C. Baseline sobre el propio Fabric con una sola organización**

- Reutilizaría el chaincode tal cual (paridad trivial) y solo cambiaría la topología: una organización, un peer, un orderer.
- No representa el término de comparación que el trabajo escrito define: una "interfaz de servicios sobre una base de datos relacional". Mediría "Fabric degradado a un nodo" contra "Fabric completo", que es una comparación de topologías de la misma arquitectura, no la comparación arquitectónica centralizado-relacional vs. distribuido-blockchain que sostiene la hipótesis.
- Tampoco permitiría los escenarios DB-1/API-1 del protocolo tal como están definidos (caída de PostgreSQL, caída de la API REST).
- Se descarta porque no representa el modelo centralizado con base relacional que el trabajo escrito fija como término de comparación.

## Decisión

Se adopta la **alternativa B**, con las siguientes reglas.

### 1. Stack

La baseline es una **API REST implementada en Go** sobre **PostgreSQL**, ambos en contenedores Docker en el **mismo host** que la red Fabric. Fabric y baseline nunca se ejecutan concurrentemente durante las mediciones, conforme §5 del protocolo ("ejecución no concurrente de Fabric y baseline").

La elección de Go es deliberada y es el corazón de esta decisión: la máquina de estados de ADR-001 y la función de decisión de la matriz de transferencias (ADR-008) viven en un **paquete Go compartido del repositorio** que el chaincode y la baseline importan por igual. La paridad de reglas queda así **garantizada por construcción, no por disciplina**: ante la misma operación sobre el mismo estado, ambas implementaciones ejecutan literalmente el mismo código de decisión y producen el mismo veredicto, el mismo `ruleId` y la misma razón de rechazo.

### 2. Esquema relacional mínimo

Cuatro tablas, cuyo DDL concreto, índices y migraciones define BASE-1 (#37):

| Tabla | Contenido | Análogo Fabric |
|---|---|---|
| `organizations` | Espejo del registro organización-establecimiento de ADR-003 **extendido por ADR-010**: identificador canónico (`GLN:`, `CUFE:` **o `REG:`**), `idType` (`GLN`/`CUFE`/`REG`), `agentType` — los seis custodiales de DES-3 más `REGULATOR` y `FINANCIER` — y `active`. Sin `REG:` no hay dónde espejar al regulador ni a los financiadores, y la baseline no podría resolver el invocador de una operación regulatoria ni de una verificación de traza. | Registro organización-establecimiento del ledger (ADR-003 + ADR-010). |
| `medication_units` | Espejo del struct público de `modelo-datos.md`: `gtin`, `numeroSerie`, `lote`, `fechaVencimiento`, `custodioActual`, `estado`, `ultimaActualizacion`. Clave primaria compuesta (GTIN + número de serie). | Estado público del canal (world state). |
| `unit_events` | Historial **append-only**: cada operación de escritura inserta un evento con timestamp, operación, invocador y el **snapshot público completo resultante** de la unidad (los siete campos de `MedicationUnit`), no solo el estado. Nunca se actualiza ni se borra una fila. Guardar el snapshot completo —y en particular `custodioActual`— es un requisito de implementabilidad, no una comodidad: `GetHistoryForKey` devuelve en Fabric el **valor entero** de la clave en cada punto, y la comprobación 5 de ADR-011 recorre «cada cambio de `CustodioActual` observado en el historial». Con solo el estado resultante, ni la consulta de historial ni la verificación de traza del financiador son replicables, y el checklist de paridad de la sección 4 sería inexigible. | Análogo funcional del transaction log de Fabric y de `GetHistoryForKey`. |
| `transfer_operations` | Registro de operación de transferencia: destinatario declarado, remito/factura, `ruleId` y `schemaVersion` de la matriz (ADR-008), estado activo/cerrado. | Registro de operación en la PDC del par (ADR-006, punto 4: `TransferOpActive`/`TransferOp`), con la **diferencia declarada** de que acá no existe confidencialidad real entre "organizaciones": todas las filas conviven en la misma base administrada por un único operador. |
| `return_operations` | Registro de devolución de las transiciones T21–T24: receptor declarado, motivo/documentación y timestamp. Histórico inmutable, sin ciclo activo/cerrado — una devolución no espera confirmación de nadie (ADR-009, punto 1). | Registro `ReturnOp`+[`gtin`,`numeroSerie`,`txIdDevolucion`] en la PDC del par (ADR-006, punto 4). Tabla separada por la misma razón por la que ADR-006 le da clave propia: una devolución no nace de un despacho y no tiene operación de transferencia a la cual adosarse. |

La API de consulta de historial lee `unit_events` — el equivalente conceptual de `GetHistoryForKey` — de modo que la operación "consulta de historial" del protocolo (§2) recupere en ambos SUT una traza de la misma forma y semántica.

### 3. Identidad emulada

Cada organización del dataset recibe una **credencial estática** (API key) que el cliente envía en el header `X-Org-Key`. La API resuelve esa key contra `organizations`, obteniendo **la organización y el rol asignado**: existe una key por par organización+rol del dataset. No hay un header de rol separado; el rol no lo declara el cliente, lo determina la key. Esto reemplaza y precisa el "header de rol" que BASE-2 (#38) esbozaba.

**Simplificación documentada**: no hay PKI, certificados ni firmas. Esta emulación es suficiente para la **paridad funcional de autorización** — la baseline puede distinguir invocadores y aplicar las mismas reglas de custodio, `agentType`, `active` y rol que el chaincode — pero **no** emula las garantías criptográficas de la membresía Fabric (imposibilidad de suplantación sin comprometer una CA, no repudio por firma). Esa asimetría no es un defecto de la baseline: las garantías criptográficas son precisamente parte de lo que la comparación evalúa, y se tratan como propiedad cualitativa conforme §1 del protocolo.

### 4. Checklist de paridad funcional

Este checklist es **normativo**: BASE-1/BASE-2/BASE-3 deben satisfacerlo y la evaluación no puede ejecutarse contra una baseline que incumpla algún ítem.

- [ ] **Misma máquina de estados** (ADR-001): vía el paquete Go compartido; toda transición no declarada se rechaza con la misma semántica.
- [ ] **Misma matriz de transferencias con los mismos `ruleId`** (ADR-008): vía el paquete Go compartido; misma decisión, misma regla, misma razón de rechazo.
- [ ] **Transferencia en dos pasos** despacho/recepción con la misma semántica de operación activa (ADR-004; §3.4 del protocolo): a lo sumo una operación activa por unidad, recepción/rechazo validados contra el destinatario declarado de la operación activa, nunca contra operaciones cerradas.
- [ ] **Mismos códigos de error del contrato** (`docs/api-contract.md`): los `code` del catálogo son idénticos; el cliente ramifica sobre `code` en ambos SUT. El transporte HTTP agrega el status correspondiente: 400 para `INVALID_REQUEST`, 404 para `UNIT_NOT_FOUND`, 403 para las fallas de autorización, 409 para `UNIT_ALREADY_EXISTS`/`INVALID_STATE_TRANSITION`. El mapeo definitivo y exhaustivo por `code` lo fija BASE-2.
- [ ] **Misma exclusión de datos personales**: la dispensación no persiste datos del paciente en ninguna tabla (Ley 25.326; ADR-005; CC-4).
- [ ] **Misma verificación del financiador**: la baseline expone la verificación de traza con el mismo veredicto estructurado que define ADR-011. Requiere que `unit_events` conserve el snapshot público completo (sección 2): sin `custodioActual` por evento, la comprobación 5 no es computable.
- [ ] **Misma autorización previa de intervención de laboratorio** (ADR-007, puntos 6.e y 6.f; contrato v2.5.0): la baseline replica `AuthorizeLabIntervention` y `RevokeLabIntervention` con el mismo `estado` (`ACTIVA`/`CONSUMIDA`/`REVOCADA`) y la misma regla de ejercicio — un laboratorio no custodio que intenta un retiro, recupero o disposición final sin una autorización `ACTIVA` y vigente recibe `LAB_INTERVENTION_REQUIRED` en **ambos** SUT. Sin esto, las rondas de rechazo esperado del protocolo (§6.5) medirían universos distintos. Lo que la baseline **no** replica es el endoso multiorganizacional que esa autorización habilita (ver sección 5): acá la valida un único proceso.
- [ ] **Mismos endpoints conceptuales** que las operaciones del contrato: registro de unidades, despacho, recepción, rechazo, dispensación, eventos extraordinarios, devolución (con su registro privado), autorización y revocación de intervención de laboratorio, administración del registro organización-establecimiento, consulta puntual, **consulta por GTIN** (`QueryUnitsByGTIN`) y consulta de historial, y verificación de traza (los nombres REST concretos los define BASE-2). La inicialización (`Init`) no tiene análogo: en la baseline el seed del registro es una migración, no una transacción bajo política de endoso.

### 5. Qué NO replica, deliberadamente

Lo que sigue queda fuera de la baseline porque es exactamente lo que la comparación evalúa; replicarlo destruiría el término de comparación:

- **Endoso multiorganizacional**: en la baseline valida un único proceso; no hay coendoso emisor+receptor ni coendoso regulatorio, ni **marcadores de participación** (ADR-007, punto 6) — no hay colecciones implícitas cuya política pertenezca a una organización, ni un plano de plataforma que pueda rechazar una operación por falta de endosos. Lo que en Fabric rechaza la plataforma con `ENDORSEMENT_POLICY_FAILURE`, en la baseline no tiene equivalente: es exactamente la asimetría que la evidencia de NET-6 explota y que el capítulo de resultados debe presentar como propiedad comparada, no como funcionalidad faltante.
- **Colecciones privadas con confidencialidad real**: `transfer_operations` reproduce la *función* del registro de operación de ADR-006, pero cualquier consulta con privilegios sobre la base ve todas las operaciones de todas las "organizaciones".
- **Inmutabilidad criptográfica del log**: el carácter append-only de `unit_events` es una **convención de aplicación** — la API nunca ejecuta `UPDATE`/`DELETE` sobre esa tabla — que un administrador de la base puede violar con una sentencia SQL, sin dejar evidencia criptográfica. Esa asimetría es exactamente el argumento cualitativo de integridad del trabajo: en Fabric la alteración retroactiva exige comprometer la cadena de hashes replicada en múltiples organizaciones; en la baseline exige un `UPDATE`.
- **Tolerancia a fallas distribuida**: la baseline es un punto único de falla **por diseño**; los escenarios DB-1/API-1 del protocolo (§8.2) existen para medir precisamente esa propiedad.

### 6. Declaración de alcance comparativo

La baseline representa el **modelo arquitectónico centralizado** — una API de servicios sobre base relacional con las mismas reglas de negocio — como **análogo funcional**. No representa, ni pretende representar, al SNT real operado por ANMAT: no se conoce ni se replica su implementación, su infraestructura ni su carga real. Todas las conclusiones cuantitativas y cualitativas del trabajo se formulan contra ese análogo, no contra el sistema productivo. Esta declaración es obligatoria en esta ADR y debe incorporarse como ítem del capítulo de limitaciones de la tesis.

Queda fuera del alcance de esta ADR: el DDL, los índices y las migraciones concretas (BASE-1, #37); los paths, verbos y el mapeo HTTP definitivo por `code` (BASE-2, #38); la paridad de eventos extraordinarios y del veredicto del financiador (BASE-3, #39); el empaquetado Compose y el seed del dataset (BASE-4, #40); y el layout Go concreto del paquete compartido, ya derivado a CC-1 por ADR-008.

## Justificación

- **La paridad por construcción es la única defensa sólida de la comparación.** La hipótesis se juega en que las diferencias medidas sean atribuibles a la arquitectura, no a la implementación. Un paquete compartido que ambos binarios importan elimina de raíz la clase de objeción "las reglas no eran las mismas": no hay que demostrar equivalencia entre dos implementaciones, porque hay una sola. Es la extensión natural — y ya anticipada — de lo que ADR-008 decidió para la matriz, ahora aplicada también a la máquina de estados de ADR-001.
- **El stack decidido es el que el protocolo ya asume.** PostgreSQL + API REST no es una preferencia nueva: §3.1 define la latencia write de la baseline contra la confirmación de la base relacional y §8.2 nombra a PostgreSQL y a la API REST como los componentes a derribar en DB-1/API-1. Esta ADR formaliza como decisión lo que el protocolo daba por caracterizado; que Go sea el lenguaje es lo único genuinamente nuevo, y se sigue del punto anterior.
- **El esquema espeja las estructuras ya decididas, no las reinventa.** `organizations` (ADR-003), `medication_units` (`modelo-datos.md`), `unit_events` (transaction log) y `transfer_operations` (ADR-006) mapean uno a uno los contenedores de datos del prototipo Fabric. Eso hace verificable la equivalencia de procesos que exige el checklist previo a medir del protocolo (§12: "Fabric y baseline implementan los mismos procesos core") y deja la asimetría restante — confidencialidad e inmutabilidad — donde debe estar: en las propiedades comparadas, no en los procesos.
- **La identidad emulada separa lo que se compara de lo que se iguala.** La autorización funcional (quién puede hacer qué) debe ser idéntica para que los rechazos esperados de §6.5 midan lo mismo en ambos SUT; las garantías criptográficas de identidad deben ser distintas porque son parte del objeto de estudio. Una API key por par organización+rol iguala lo primero sin fingir lo segundo, y documentarlo evita que la tesis sobreatribuya a la baseline garantías que no tiene.
- **Las alternativas descartadas fallan por la misma razón, en direcciones opuestas.** La alternativa A iguala el stack de moda pero rompe la paridad de reglas; la C garantiza la paridad de reglas pero destruye el término de comparación arquitectónico. Solo B mantiene ambas propiedades a la vez.

## Consecuencias

- **BASE-1 (#37)**: desbloqueada; materializa el esquema de la sección 2 como DDL con migraciones versionadas e índices equivalentes a las consultas del chaincode — incluido un índice por `gtin` sobre `medication_units` para `QueryUnitsByGTIN`, análogo de la consulta por clave compuesta parcial.
- **BASE-2 (#38)**: desbloqueada; define endpoints REST, el mapeo HTTP definitivo por `code` y consume el paquete compartido para validaciones. El "header de rol" que esbozaba queda reemplazado por la resolución key→(organización, rol) de la sección 3.
- **BASE-3 (#39)**: desbloqueada; implementa la paridad de eventos extraordinarios y el veredicto de verificación de ADR-011 sobre esta base.
- **BASE-4 (#40)**: desbloqueada; empaqueta API + PostgreSQL en Compose y carga el seed del dataset compartido (§4 del protocolo).
- **EVAL-3 (#43) y EVAL-5 (#45)**: desbloqueadas; las mediciones y la prueba de disponibilidad de la baseline ya tienen un SUT definido contra el cual escribirse.
- **CLI-3 (#36)**: el generador de dataset comparte el mismo paquete Go para producir cadenas válidas e inválidas coherentes con ambas implementaciones, como ya anticipó ADR-008.
- **Para la tesis**: el capítulo de limitaciones debe incorporar la declaración de alcance comparativo de la sección 6 (la baseline es un análogo del modelo centralizado, no el SNT real).

- **Se gana**: una baseline cuyo diseño es defendible — paridad de reglas por construcción, esquema espejo de las estructuras decididas, asimetrías declaradas y acotadas a las propiedades comparadas — y seis issues de implementación desbloqueadas con criterios de aceptación claros.
- **Se pierde / costo**: la baseline se implementa en Go aunque no sea el stack natural de una API de referencia; la comparación queda formalmente acotada a un análogo del modelo centralizado, lo que impide afirmar mediciones contra el sistema productivo de ANMAT (renuncia deliberada, no accidental).
- **Queda pendiente**: el layout Go concreto del paquete compartido (módulo único del repositorio vs. módulos con `replace`), derivado a CC-1 y consumido por BASE-1; y el mapeo HTTP definitivo y exhaustivo por `code`, que fija BASE-2 sobre la orientación de la sección 4.

## Divergencia con el trabajo escrito

No hay divergencia. El trabajo escrito define la baseline como "interfaz de servicios sobre una base de datos relacional que implementa los mismos procesos core... bajo idénticas condiciones de carga", y esta decisión lo cumple punto por punto: API de servicios (REST en Go), base relacional (PostgreSQL), mismos procesos core (checklist de paridad de la sección 4, con las reglas compartidas por construcción) e idénticas condiciones de carga (§5 del protocolo de medición, que esta ADR adopta como restricción operativa). Se deja constancia, además, de la precisión de la sección 6: la baseline representa el modelo arquitectónico centralizado como análogo funcional, no al SNT real de ANMAT, y esa acotación debe reflejarse en el capítulo de limitaciones de la próxima iteración del documento.

## Contexto utilizado

- Issue GitHub #87: DES-18 · ADR-012: Diseño de la baseline centralizada y paridad funcional, consultada el 2026-08-17.
- Issue GitHub #37: BASE-1 · Esquema relacional, consultada el 2026-08-17.
- Issue GitHub #38: BASE-2 · API REST con los procesos core, consultada el 2026-08-17.
- [`docs/measurement-protocol.md`](../measurement-protocol.md): objetivo y alcance de la comparación (§1–2), definición de latencia de la baseline (§3.1), medición bifásica de la transferencia (§3.4), dataset compartido (§4), condiciones idénticas (§5) y escenarios DB-1/API-1 (§8.2).
- [`docs/api-contract.md`](../api-contract.md) (v2.5.0): catálogo de códigos de error estables y operaciones del contrato que la baseline debe espejar.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): la máquina que la baseline aplica vía el paquete compartido; regla de consumo "el chaincode y la baseline deben rechazar cualquier transición no listada".
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): registro organización-establecimiento que `organizations` espeja.
- [ADR-004: Modelado de la transferencia — despacho/recepción como dos transacciones](004-transfer-dispatch-reception.md): semántica de operación activa que la baseline replica en dos pasos.
- [ADR-006: Mecanismo de colecciones privadas](006-private-data-collections.md): registro de operación en PDC del que `transfer_operations` es análogo sin confidencialidad real.
- [ADR-008: Distribución y versionado de la matriz de transferencias](008-transfer-matrix-distribution.md): paquete Go compartido, `ruleId` y versión de matriz persistidos por despacho.
- [ADR-010: Identidad de las organizaciones no custodiales](010-non-custodial-identity.md): `agentType` no custodiales que `organizations` debe admitir.
- [ADR-011: Criterios de verificación de traza del financiador](011-financier-trace-verification.md): veredicto estructurado que la baseline expone con la misma semántica.
- [ADR-007: Topología física de la red del prototipo](007-network-topology.md): mecanismos de endoso que la baseline **no** replica (sección 5) y superficie de autorización de intervención de laboratorio que **sí** debe replicar (sección 4).
- [`baseline/README.md`](../../baseline/README.md): requisitos preexistentes del directorio (dos transacciones, matriz única, códigos de error estables).
- [`docs/adr-roadmap.md`](../adr-roadmap.md): decisión D7, incluida la advertencia de consistencia con el trabajo escrito.
- [`docs/modelo-datos.md`](../modelo-datos.md): struct público `MedicationUnit` y clave compuesta GTIN+serie que `medication_units` espeja.
