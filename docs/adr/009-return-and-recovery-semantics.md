# ADR-009: Semántica de la devolución, custodia en DEVUELTO y actor de recupero/disposición

- **Estado**: Propuesto
- **Fecha**: 2026-08-17
- **Autores**: Serra, Zarlenga

---

## Contexto

ADR-001 define el estado `DEVUELTO` y cinco caminos de entrada: el rechazo en tránsito (T05, desde `EN_TRANSITO`) y las devoluciones posteriores a custodia (T21 desde `EN_CUSTODIA`, T22 desde `EN_CUARENTENA`, T23 desde `RETIRADO_MERCADO`/`PROHIBIDO`, T24 desde `VENCIDO`), todas bajo el evento `DEVOLVER_PRODUCTO`. ADR-004 resolvió la semántica de custodia solo para T05: `CustodioActual` permanece en el emisor del despacho rechazado, y el registro de la operación de transferencia se conserva cerrado en la PDC. Para T21–T24 ningún documento decidió si la custodia registrada cambia, cuándo, ni mediante qué evento, a pesar de que la unidad viaja físicamente de vuelta al proveedor. ADR-004 dejó además una pregunta expresa a la issue EXT-4: si el chaincode debe distinguir el rechazo en tránsito de la devolución post-custodia, dado que ambos comparten estado destino y transiciones de resolución.

En paralelo, el hallazgo C5 de la revisión de congruencia (`docs/consistency-review.md`) señala que el actor lógico `RECOVERY_OR_DISPOSAL_AGENT` de ADR-001 — habilitado en T25 (reingreso a stock) y T28, T29, T31, T32, T33 (disposiciones finales) — no tiene resolución en términos de organizaciones, `agentType` ni `snt.role`: ni ADR-003, ni DES-6, ni el contrato de API definen quién es. Sin esa resolución, esas transiciones no tienen regla de autorización implementable y las operaciones `ReturnProduct`, `Restock` y `FinalDisposition` del contrato (`docs/api-contract.md`) tienen actor habilitado ambiguo.

La decisión D4 de `docs/adr-roadmap.md` (issue #84, DES-15) agrupa estas cuatro preguntas — custodia tras cada camino a `DEVUELTO`, devolución como par bifásico o evento único, resolución de `RECOVERY_OR_DISPOSAL_AGENT`, y distinción entre rechazo en tránsito y devolución post-custodia — y advierte el riesgo de divergencia con el trabajo escrito: el paper describe el caso devolución como entrega y recepción entre dos actores, y el caso reingreso a stock con validaciones concretas sobre la aptitud de la unidad y sobre quién puede reincorporarla (con una ambigüedad de redacción en la fuente que esta ADR resuelve explícitamente; ver Justificación). Esta ADR resuelve las cuatro preguntas y desbloquea las issues EXT-4 (#30), EXT-5 (#31) y EXT-8 (#63).

## Alternativas

**A. Devolución como par despacho/recepción simétrico a ADR-004**

- Replica para `DEVOLVER_PRODUCTO` el modelo bifásico de la transferencia ordinaria: el custodio despacharía la devolución (estado intermedio en tránsito), y el receptor de la devolución la confirmaría, actualizando `CustodioActual`.
- Es la lectura más literal del trabajo escrito ("entrega y recepción de un medicamento como devolución entre dos actores de la cadena") y del proceso físico real.
- Exige modificar ADR-001: la máquina 1.0.0 no tiene estado intermedio "devolución en tránsito" ni evento de recepción de devolución; habría que duplicar `EN_TRANSITO`, T04 y T05 para el flujo inverso, o sobrecargar los existentes con una dirección adicional.
- Duplica el mecanismo de PDC, endoso conjunto y validación de destinatario declarado de ADR-004 para un flujo secundario que no forma parte de las operaciones core medidas por el protocolo de DES-7.
- Se descarta porque el costo de diseño e implementación no está justificado para v1: el valor comparativo del prototipo se concentra en la transferencia ordinaria, y la devolución bifásica puede incorporarse en una iteración futura como revisión explícita de esta ADR sin invalidar lo construido.

**B. Evento único con cambio inmediato de custodia al receptor de la devolución**

- `DEVOLVER_PRODUCTO` actualizaría `CustodioActual` al receptor de la devolución (laboratorio, proveedor) en la misma transacción.
- Evita el estado intermedio, pero registra un cambio de custodia que el receptor nunca confirmó: viola el principio establecido por ADR-004 de que ningún cambio de custodia se asienta sin un acto propio del receptor, y publica en el estado del canal una relación custodio→receptor no consumada, exactamente lo que ADR-002/ADR-004 clasifican como dato privado.
- Deja además sin respuesta quién resuelve `DEVUELTO` (T25/T33): si la custodia ya pasó al receptor, el custodio original pierde toda capacidad de acción sobre una unidad que puede seguir físicamente en su poder.
- Se descarta porque introduce una afirmación de custodia unilateral incompatible con el diseño de ADR-004 y con la clasificación de visibilidad de ADR-002.

**C. Evento único sin cambio de custodia registrado, con receptor declarado como dato privado**

- `DEVOLVER_PRODUCTO` registra el hecho normativo de la devolución en una sola transacción; `CustodioActual` no cambia; el receptor de la devolución puede declararse como dato de un registro propio en la PDC del par, informativo y auditable.
- Coherente con lo ya decidido para T05 por ADR-004 (el emisor conserva la custodia registrada en `DEVUELTO`), con la máquina de ADR-001 sin modificaciones, y con la regla de confidencialidad de relaciones no consumadas.
- El traslado físico de retorno queda sin representación como transferencia: es una simplificación consciente que debe documentarse como divergencia con el trabajo escrito y como limitación.
- Se adopta.

**D. `RECOVERY_OR_DISPOSAL_AGENT` como `agentType` o rol dedicado nuevo**

- Introducir en ADR-003/DES-6 un tipo de agente "recuperador/dispositor" (por ejemplo, gestores de residuos patológicos) con organizaciones propias en el registro.
- Ningún documento normativo relevado por el proyecto exige que el agente de recupero sea un eslabón trazado distinto de los ya modelados; agregarlo obliga a extender el catálogo de `agentType`, la matriz DES-3 y las políticas de DES-6 sin respaldo en el relevamiento.
- Se descarta porque agrega un tipo de actor sin fundamento normativo y contradice la validación de custodia que el trabajo escrito sí releva para el reingreso a stock (ver Justificación sobre la lectura adoptada de esa cláusula).

**E. `RECOVERY_OR_DISPOSAL_AGENT` resuelto como el custodio actual registrado con rol `operator`**

- El actor lógico se resuelve, en cada transición donde ADR-001 lo habilita, como la organización que figura como `CustodioActual` de la unidad, con `snt.role=operator`, complementada por los actores alternativos que la fila correspondiente de ADR-001 ya lista (ANMAT, laboratorio titular según transición).
- Aplica la validación de custodia que el trabajo escrito releva para el reingreso, en la lectura corregida que la Justificación declara, no requiere `agentType` ni rol nuevo, y es implementable con las reglas de autorización ya definidas por DES-6.
- Se adopta.

## Decisión

Se adoptan las **alternativas C y E**. Las reglas concretas que esta ADR fija son:

1. **La devolución es un evento único, no un par despacho/recepción.** `DEVOLVER_PRODUCTO` registra el hecho normativo de la devolución en una sola transacción de chaincode. El contrato vigente (DES-5) lo expone con dos funciones según el origen, y esta ADR conserva esa correspondencia: `RejectTransfer` cubre el rechazo en tránsito (T05) y `ReturnProduct` las devoluciones posteriores a custodia (T21–T24). `CustodioActual` **no cambia** en ninguna transición hacia `DEVUELTO`: permanece en quien tenía la custodia registrada — en T05, el emisor del despacho rechazado (como ya fijó ADR-004); en T21–T24, el custodio que declara devolver (aun cuando la transición la invoque ANMAT en los casos T22/T23 donde ADR-001 lo habilita). El traslado físico de retorno al proveedor **no se modela como cambio de custodia en v1**.

2. **Receptor de la devolución como dato privado.** La operación de devolución puede declarar el receptor de la devolución (laboratorio, proveedor, destino de recupero) como dato de un **registro de devolución propio** en la PDC del par — `ReturnOp`+[`gtin`,`numeroSerie`,`txIdDevolucion`] (ADR-006, punto 4) —, análogo al destinatario declarado de ADR-004 y con la misma lógica de confidencialidad. La clave es propia y no reutiliza `TransferOp`: una devolución T21–T24 no nace de un despacho, no tiene `txIdDespacho`, y no administra ciclo activo/cerrado porque no espera confirmación de nadie (punto 1). El rechazo en tránsito (T05) es el caso opuesto: nace de un despacho y cierra su `TransferOp` como cualquier otra salida de `EN_TRANSITO`: es una relación no consumada que no debe quedar en el estado público ni en los argumentos públicos de la transacción (transporte por campo `transient`, materialización de la colección según el esquema de ADR-006). Es un dato informativo y auditable — visible para el custodio declarante, el receptor declarado y `AnmatMSP` — **sin efecto sobre `CustodioActual`**. El `transient` es **opcional**: si no viene, la devolución se registra sin escritura privada y sin resolver colección alguna.

   **Reglas de validación del receptor declarado.** Cuando el `transient` trae un receptor, el chaincode lo valida **antes** de computar el nombre de la colección. Sin estas reglas, un receptor arbitrario haría que el chaincode calculara el nombre de una colección que no existe en la definición del chaincode, y la operación fallaría con un error interno de la plataforma en lugar de un error tipificado del contrato. Las validaciones se resuelven todas contra el world state, por lectura directa de clave, y se evalúan en este orden:

   | # | Validación | Código de error |
   |---|---|---|
   | 1 | El identificador tiene la forma canónica `GLN:`/`CUFE:` de ADR-003. | `INVALID_REQUEST` |
   | 2 | El receptor existe en el registro organización-establecimiento. | `ORG_NOT_REGISTERED` |
   | 3 | El receptor está activo (`active = true`). | `ORG_INACTIVE` |
   | 4 | El `agentType` del receptor es **custodial** (ADR-010: los no custodiales nunca son origen ni destino). | `INVALID_DESTINATION` |
   | 5 | El receptor no es la propia organización que declara la devolución. | `INVALID_DESTINATION` |
   | 6 | El par «`agentType` del receptor → `agentType` del custodio declarante» está autorizado por la matriz de DES-3 — es decir, el receptor podría legítimamente haber sido el proveedor de quien devuelve. | `TRANSFER_NOT_AUTHORIZED` |

   La validación 6 no es un formalismo: es **exactamente** la condición bajo la cual ADR-006 define la colección del par. ADR-006 crea una colección por cada par de organizaciones entre las que la matriz autoriza una transferencia en alguna dirección; si el par receptor → custodio está autorizado, la colección existe con certeza. Validarla convierte «la colección existe por definición» —afirmación que ADR-006 hacía sin respaldo— en una precondición comprobada por el chaincode.

   **Qué no se exige en v1, y por qué.** No se exige que el receptor declarado sea la organización que efectivamente le entregó **esa** unidad al custodio actual. Tres razones:
   - la devolución no cambia `CustodioActual` (punto 1), de modo que un receptor mal declarado no puede provocar una atribución de custodia incorrecta: degrada la calidad del dato documental, no la integridad de la traza;
   - la comprobación exigiría reconstruir el origen de la custodia con `GetHistoryForKey`, cuyos resultados **no** integran el read-set de la transacción y por lo tanto no se revalidan en el commit; una regla de autorización apoyada en historial no sería verificable en la fase de validación, que es justamente la propiedad que el resto del diseño busca;
   - un retorno legítimo puede dirigirse a un establecimiento del proveedor distinto de aquel desde el que se despachó (depósito de devoluciones, planta del titular), con lo que la coincidencia exacta sería incluso demasiado estricta.

   Queda declarada como limitación y con camino de revisión: persistir en el registro de la unidad el identificador de la transacción de despacho que originó la custodia actual permitiría una lectura privada por clave directa —verificable en el commit— y habilitaría la comprobación fuerte sin recurrir al historial.

3. **Resolución del actor `RECOVERY_OR_DISPOSAL_AGENT`** (cierra el hallazgo C5 de `docs/consistency-review.md`). En todas las transiciones donde ADR-001 lo habilita (T25, T28, T29, T31, T32, T33), `RECOVERY_OR_DISPOSAL_AGENT` se resuelve como **el custodio actual registrado de la unidad, con rol `operator`**, complementado por los actores alternativos que cada fila de ADR-001 ya lista (ANMAT o laboratorio titular, según la transición). Consecuencia directa: no se agrega `agentType` ni rol nuevo, y DES-6 no cambia — las columnas de autorización y endoso ya publicadas en `docs/api-contract.md` para `ReturnProduct`, `Restock` y `FinalDisposition` quedan confirmadas con esta resolución.

4. **Rechazo en tránsito y devolución post-custodia: unificación consciente en v1.** Ambos caminos comparten el estado destino (`DEVUELTO`) y las transiciones de resolución. El chaincode **no bifurca la semántica**: los distingue solo por la evidencia registrada — la transición de origen queda en el historial de la unidad, y el registro de operación de transferencia en PDC (ADR-004) existe solo en el caso T05, porque solo un despacho previo lo crea. Si una iteración futura necesita revertir custodia en devoluciones post-custodia (el "revierte la custodia" que la issue EXT-4 esbozaba), eso constituye una revisión de esta ADR, no una interpretación de ella.

5. **Reingreso a stock (T25/T26/T27).** `Restock` valida lo que el trabajo escrito enumera para el caso reingreso: la unidad no está vencida, no está destruida/deteriorada, no está robada/extraviada, y no pesan sobre ella retiro de mercado ni prohibición vigentes. El actor es el custodio actual registrado (T25/T26, conforme al punto 3) o ANMAT/laboratorio titular cuando existe cierre o corrección autorizada (T27, conforme a ADR-001). El resultado `EN_CUSTODIA` **mantiene `CustodioActual` sin cambios**: la unidad reingresa al stock de quien la tiene registrada.

Queda fuera de alcance de esta ADR: el modelado bifásico del retorno físico (derivado a una revisión futura de esta ADR si un requisito lo exige) y la implementación en chaincode y baseline (EXT-4/CC-5, BASE-2). Las firmas y payloads —incluidos el `transient` `devolucion` y las validaciones del punto 2— **sí** están fijados, en el contrato DES-5 (versiones 2.1.0 y 2.2.0); las políticas de endoso las fija DES-6 con las salvedades de ADR-007 y no requieren cambios adicionales por esta ADR.

## Justificación

La resolución del actor de recupero se apoya en la validación que el trabajo escrito releva para el reingreso a stock, con una corrección de lectura que debe declararse explícitamente. El texto original enumera las condiciones a verificar así:

> "Ejemplos de esto puede ser que el medicamento no se encuentre vencido, que el actor que intenta reingresar a stock el medicamento **no sea** el actual custodio de este o que el medicamento no se haya registrado como destruido o finalmente dispuesto."

Leído literalmente, el segundo ejemplo exigiría validar que quien reingresa **no** sea el custodio actual, lo que resulta incoherente con los otros dos elementos de la misma enumeración —ambos redactados como condiciones cuya verificación protege la operación (que no esté vencido, que no esté destruido)— y con la lógica del proceso: quien reincorpora una unidad al stock es precisamente quien la tiene bajo su custodia registrada. Esta ADR interpreta que se trata de un **error de redacción del trabajo escrito** y adopta la lectura corregida: la validación consiste en verificar que quien reingresa **sí sea** el custodio actual. Es una interpretación consciente, no una cita literal; queda registrada acá y en el manual de correcciones del trabajo escrito (`docs/paper-update-instructions.md`) para que la próxima iteración del documento resuelva la ambigüedad en la fuente.

Sobre esa lectura, resolver `RECOVERY_OR_DISPOSAL_AGENT` hacia el custodio actual registrado convierte la validación en regla determinística de autorización, sin introducir un tipo de agente nuevo que ninguna fuente normativa relevada respalda. Es además la opción que el hallazgo C5 y la decisión D4 del roadmap identificaron como la más simple y consistente con el resto del diseño.

La semántica de evento único extiende a T21–T24 el criterio que ADR-004 ya fijó para T05: en `DEVUELTO`, la custodia registrada permanece en quien la tenía. Esto mantiene una invariante única para todo el estado — el custodio registrado de una unidad en `DEVUELTO` es siempre quien puede resolverla (reingresar, disponer) o sobre quien recaen los eventos extraordinarios — y evita registrar cambios de custodia que el receptor nunca confirmó. El tratamiento del receptor declarado como dato privado es una aplicación directa de la regla de ADR-002/ADR-004: una relación entre partes que aún no se consumó no debe quedar expuesta en el estado público ni en el historial del canal.

La unificación de rechazo en tránsito y devolución post-custodia evita duplicar lógica para dos caminos que ADR-001 ya hace converger en el mismo estado con las mismas transiciones de salida. La distinción permanece disponible para auditoría — el historial conserva la transición de origen y la PDC conserva el registro de la operación rechazada en el caso T05 — sin costo adicional de diseño.

Descartar la alternativa A no niega su fidelidad al proceso físico: la descarta **para v1** porque la devolución no forma parte de las operaciones core que el protocolo de medición de DES-7 compara entre chaincode y baseline, y duplicar el mecanismo bifásico de ADR-004 (estado intermedio, PDC de operación, endoso conjunto, validación de destinatario) para un flujo secundario desplazaría esfuerzo de la hipótesis del trabajo sin mejorar la evidencia comparativa.

## Divergencia con el trabajo escrito

El trabajo escrito describe el caso devolución como "entrega y recepción de un medicamento como devolución **entre dos actores** de la cadena" — es decir, un flujo bifásico con dos agentes detonantes, simétrico al de distribución/recepción.

El prototipo v1 lo simplifica: la devolución es un **evento único** (`DEVOLVER_PRODUCTO`, expuesto como `RejectTransfer` para T05 y `ReturnProduct` para T21–T24) que registra el hecho normativo sin cambio de `CustodioActual`; el receptor de la devolución solo existe como dato privado declarado en la PDC de la operación, y el traslado físico de retorno no se representa como transferencia.

La razón es de alcance: replicar el mecanismo bifásico de ADR-004 (estado intermedio, registro de operación, endoso conjunto emisor/receptor, validación de destinatario declarado) para la devolución duplicaría el componente más complejo del diseño en un flujo secundario que no forma parte de las operaciones core medidas por el protocolo de DES-7, sin aportar a la hipótesis comparativa del trabajo.

Esta simplificación debe listarse en el capítulo de limitaciones de la tesis: el prototipo registra que una unidad fue devuelta y por quién, pero no registra la recepción de la devolución por el segundo actor ni el cambio de custodia asociado. La próxima iteración del documento de tesis debe describir el caso devolución conforme a esta ADR (evento único en v1, flujo bifásico como extensión futura) o mantener la descripción original acompañada de la limitación explícita.

## Consecuencias

- **Para EXT-4 (#30), EXT-5 (#31) y EXT-8 (#63)**: quedan desbloqueadas. Las siete transiciones sin regla de autorización implementable (T21–T25, T28, T33, más T29/T31/T32) tienen ahora actor resuelto; los criterios de aceptación de EXT-4 que suponían dos eventos y reversión de custodia deben ajustarse a la semántica de evento único de esta ADR.
- **Para CC-5/CC-6 y el contrato de API**: `RejectTransfer`, `ReturnProduct`, `Restock` y `FinalDisposition` son implementables con las columnas de actor y endoso ya publicadas. El contrato ya incorporó el transporte `transient` opcional del receptor declarado de la devolución (versión 2.1.0) y suma en la versión 2.2.0 las **seis validaciones del punto 2** con sus códigos de error: el diseño de esa superficie pertenece a DES-5, no a las issues de implementación. La story de implementación (EXT-4, #30) debe cubrir con tests los seis rechazos y el caso sin `transient`.
- **Para DES-6**: sin cambios. La resolución del punto 3 se apoya en roles y políticas ya definidos (`operator`, coendoso regulatorio existente).
- **Para ADR-001**: sin cambios en la máquina; esta ADR fija la interpretación de `RECOVERY_OR_DISPOSAL_AGENT` sin tocar la tabla de transiciones.
- **Para la baseline (BASE-2/ADR-012)**: debe replicar la misma semántica — devolución como evento único sin cambio de custodia y misma resolución de actores — para preservar la paridad funcional exigida por el protocolo de medición.
- **Para `docs/alcance-prototipo.md`**: se agrega la fila que registra la simplificación de la devolución como decisión consciente de alcance.
- **Se gana**: siete transiciones con regla de autorización determinística; cierre del hallazgo C5 sin agregar tipos de agente ni roles; una única invariante de custodia para `DEVUELTO`; confidencialidad del receptor de devolución consistente con ADR-002/ADR-004.
- **Se pierde / costo**: el ledger no registra la recepción de la devolución ni el cambio de custodia del retorno físico — divergencia documentada con el trabajo escrito y limitación a listar en la tesis.
- **Queda pendiente**: modelar el retorno físico como flujo bifásico con reversión de custodia, si un requisito futuro lo exige; esa extensión constituye una revisión de esta ADR y debe definir el estado intermedio, el registro de operación y el endoso correspondientes.

## Contexto utilizado

- Issue GitHub #84: DES-15 · ADR-009: Semántica de devolución y custodia en DEVUELTO, consultada el 2026-08-17.
- Issue GitHub #30: EXT-4 · Devolución, consultada el 2026-08-17.
- Issue GitHub #31: EXT-5 · Reingreso a stock, consultada el 2026-08-17.
- Issue GitHub #63: EXT-8 · Disposición final / destrucción autorizada, consultada el 2026-08-17.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): estados `DEVUELTO`/`RETIRADO_MERCADO`, transiciones T05, T21–T33 y actor lógico `RECOVERY_OR_DISPOSAL_AGENT`.
- [ADR-004: Modelado de la transferencia — despacho/recepción como dos transacciones](004-transfer-dispatch-reception.md): semántica de `CustodioActual` en T05, ciclo de vida del registro de operación en PDC, sección "Rechazo en recepción y relación con EXT-4" y regla de confidencialidad de relaciones no consumadas.
- [Contrato de API (DES-5)](../api-contract.md): operaciones `ReturnProduct`, `Restock` y `FinalDisposition`, columnas de actor habilitado y endoso.
- [Revisión de congruencia](../consistency-review.md): hallazgo C5 sobre la falta de resolución de `RECOVERY_OR_DISPOSAL_AGENT`.
- [Roadmap de ADRs](../adr-roadmap.md): decisión D4, preguntas agrupadas y riesgo de divergencia con el trabajo escrito.
- Paper del proyecto: caso devolución ("entrega y recepción de un medicamento como devolución entre dos actores de la cadena") y caso reingreso a stock (validaciones de aptitud y exigencia de que quien reingresa sea el actual custodio).
