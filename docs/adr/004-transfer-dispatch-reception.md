# ADR-004: Modelado de la transferencia — despacho/recepción como dos transacciones

- **Estado**: Aceptado
- **Fecha**: 2026-08-13
- **Autores**: Serra, Zarlenga

---

## Contexto

El Sistema Nacional de Trazabilidad modela la transferencia de custodia como dos eventos con agentes detonantes distintos. La Disposición ANMAT 3683/2011, artículo 8, los enumera por separado: "distribución del producto a un eslabón posterior" (acción del emisor) y "recepción del producto en el establecimiento" (acción del receptor). La Resolución MS 435/2011 exige que cada agente registre los movimientos propios de su eslabón; no es un solo actor el que da fe de que entregó y recibió.

ADR-001 ya incorporó esta distinción en la máquina de estados: el estado `EN_TRANSITO` existe, las transiciones T02 y T03 lo originan (despacho), T04 lo resuelve (recepción), y T05 lo resuelve por rechazo (devolución). ADR-001 dejó explícitamente abierta la pregunta de si `EN_TRANSITO` se implementa como dos transacciones en cadena o como una operación atómica, derivándola a esta ADR.

El modelo de datos de DES-2 tiene una dependencia abierta hacia esta decisión: si se elige el modelo de dos fases, la semántica de `CustodioActual` durante el tránsito queda sin definir sin un campo adicional que distinga "quién tiene la custodia legal actual" de "quién fue declarado como destino pendiente de confirmar".

## Alternativas

**A. Dos transacciones de chaincode: despacho y recepción**

- El emisor invoca la operación de despacho (evento T02 o T03 de ADR-001; nombre concreto de función a definir por DES-5), que transiciona la unidad de `EN_LABORATORIO` o `EN_CUSTODIA` a `EN_TRANSITO`. En ese momento el chaincode registra el identificador canónico del destinatario declarado.
- El receptor invoca la operación de recepción (evento T04 de ADR-001; nombre concreto a definir por DES-5), que transiciona de `EN_TRANSITO` a `EN_CUSTODIA` y actualiza `CustodioActual` al identificador del receptor. Solo puede invocar esta transacción la organización que figura como destinatario declarado.
- Si el receptor rechaza, invoca la operación de rechazo (T05 de ADR-001; nombre concreto a definir por DES-5), que transiciona a `DEVUELTO`.
- El emisor y el receptor son quienes *invocan* cada transacción (cliente de la propuesta), lo que mantiene la auditabilidad del hecho "el emisor declaró enviar" separada del hecho "el receptor aceptó recibir". Quién debe *endosar* (firmar como peer requerido por la política) cada transacción es una decisión distinta, gobernada por DES-6 y no por esta ADR — ver "Endoso" más abajo.
- Introduce complejidad: la ventana de tiempo entre despacho y recepción expone un estado `EN_TRANSITO` que el chaincode debe manejar correctamente ante eventos extraordinarios (T09, T13, T14, T15, T16 de ADR-001).
- Se adopta.

**B. Transacción atómica única**

- Una sola transacción de chaincode representa el despacho y la recepción simultáneamente. El asset pasa directamente de `EN_LABORATORIO`/`EN_CUSTODIA` a `EN_CUSTODIA` del nuevo custodio, sin pasar por `EN_TRANSITO`.
- Más simple de implementar.
- Requiere modificar ADR-001: `EN_TRANSITO`, T02, T03, T04 y T05 pierden razón de ser y deben eliminarse.
- No representa fielmente el proceso del SNT: supone que emisor y receptor firman el mismo evento en el mismo instante, lo cual no ocurre en operaciones reales de distribución farmacéutica con entregas posteriores a la facturación.
- No permite modelar el rechazo en recepción como evento separado; el rechazo quedaría fuera del alcance del prototipo o requeriría un mecanismo fuera de la cadena.
- Se descarta.

## Decisión

Se adopta la **alternativa A**: la transferencia se modela como dos transacciones de chaincode separadas, una invocada por el emisor (despacho, genera `EN_TRANSITO`) y otra invocada por el receptor (recepción, genera `EN_CUSTODIA` o `DEVUELTO`).

`CustodioActual` permanece como el emisor durante el tránsito y no se modifica hasta que la recepción se confirma. El identificador canónico del receptor declarado (`DestinatarioPendiente`) **no se persiste en el estado público del canal**: se escribe en la colección privada (PDC) de la operación, con membresía emisor + receptor declarado + `AnmatMSP` (la misma colección que ya recibe factura/remito, ver "Referencias documentales de la operación"). La sección "Endoso" fija quién debe firmar cada transacción y la sección "Por qué DestinatarioPendiente no es estado público" explica por qué se descarta el estado público para este campo.

## Endoso

Esta ADR fija quién *invoca* cada transacción (sección "Alternativas"). Quién debe *endosar* cada una es responsabilidad de DES-6 (`docs/organizations-roles-endorsement.md`), no de esta ADR. Por consistencia con la necesidad técnica de que el peer del receptor pueda validar el destinatario declarado desde la PDC antes de confirmar recepción, la transacción de recepción (T04) requiere endoso conjunto de la organización emisora y la organización receptora — ambas son miembros de la PDC de la operación y ambas tienen interés directo en que el cambio de custodia quede correctamente registrado. La transacción de despacho (T02/T03) requiere, como mínimo, el endoso de la organización emisora, conforme a la política general de DES-6 para eventos iniciados por el custodio actual.

## Justificación

La Disposición ANMAT 3683/2011, artículo 8, lista distribución y recepción como movimientos logísticos distinguibles, cada uno con su propio agente detonante. Modelarlos como una única transacción elimina del ledger la evidencia de cuándo ocurrió el despacho y cuándo ocurrió la recepción — información que la normativa exige registrar separadamente y que el rol de auditoría de ANMAT necesita para reconstruir la trazabilidad completa de cada unidad.

La alternativa B requeriría modificar ADR-001, que ya fue aceptada por el equipo como base del diseño del chaincode y el baseline. Revertir `EN_TRANSITO` a esta altura del diseño tiene un costo mayor que resolver la semántica de `CustodioActual` durante el tránsito con un campo adicional.

La alternativa A también preserva el valor de la evaluación comparativa: la baseline debe implementar el mismo modelo de dos fases para que la comparación de latencia y throughput entre chaincode y baseline corresponda a los mismos procesos del SNT.

## Semántica de CustodioActual y DestinatarioPendiente durante el tránsito

| Campo | Ubicación | Significado durante EN_TRANSITO | Significado fuera de EN_TRANSITO |
|---|---|---|---|
| `CustodioActual` | Estado público del canal | Identificador canónico del emisor: custodio registrado en el ledger por convención de diseño de esta ADR mientras dura el tránsito. No es una calificación de responsabilidad legal — las fuentes normativas relevadas (Disposición 3683/2011, art. 8) distinguen despacho físico de llegada/aceptación, pero no asignan expresamente una responsabilidad jurídica intermedia; ver "Justificación". | Custodio actual de la unidad. |
| `DestinatarioPendiente` | PDC de la operación (emisor + receptor declarado + `AnmatMSP`) | Identificador canónico del receptor declarado por el emisor, dentro del registro de la operación **activa**. El chaincode valida, a partir de la PDC, que quien invoca la recepción coincida con este valor. | Sin operación activa: no existe transferencia pendiente que validar. Los registros de operaciones cerradas se conservan en la PDC como historial auditable — ver "Ciclo de vida del registro de operación". |

### Ciclo de vida del registro de operación

Para que "conservar la entrada como registro histórico" no contradiga "no hay transferencia activa fuera de `EN_TRANSITO`", esta ADR distingue dos condiciones del mismo registro — **activo** y **cerrado** — con las siguientes reglas:

1. Cada despacho (T02/T03) crea en la PDC un **registro de operación** con una clave propia por operación (por ejemplo, derivada del identificador de transacción del despacho; el esquema concreto de claves lo definen DES-5/NET-5). El registro contiene el destinatario declarado y los datos documentales de la operación.
2. Una unidad tiene **a lo sumo una operación de transferencia activa**: la creada por el último despacho, y solo mientras la unidad permanezca en `EN_TRANSITO`. La condición de "activa" se deriva del estado público de la unidad más la identificación de la última operación; no requiere un campo público adicional.
3. La recepción (T04) y el rechazo (T05) validan al invocador contra el destinatario declarado del registro de la operación **activa** — nunca contra registros de operaciones anteriores de la misma unidad.
4. Al salir de `EN_TRANSITO` por cualquier vía (T04, T05, T09, T13–T16), la operación deja de estar activa. Su registro **se conserva** en la PDC como registro histórico auditable de la operación cerrada — incluida la relación transferencia↔cuarentena que ADR-001 exige conservar en T09 — con la misma membresía (emisor + receptor declarado + `AnmatMSP`).
5. Un despacho posterior de la misma unidad crea un registro de operación **nuevo**, con clave nueva; nunca reutiliza ni sobreescribe un registro cerrado.

Reglas de modificación:
- **T02 / T03 (despacho)**: el emisor crea el registro de operación en la PDC con el destinatario declarado; `CustodioActual` (público) no cambia.
- **T04 (recepción)**: el chaincode valida contra el registro de la operación activa que el invocador coincida con el destinatario declarado, y actualiza `CustodioActual` (público) al receptor. La operación pasa a cerrada; su registro se conserva, igual que factura/remito.
- **T05 (rechazo/devolución en tránsito)**: el chaincode transiciona a `DEVUELTO`; `CustodioActual` (público) se mantiene en el emisor. La operación pasa a cerrada; su registro se conserva.
- **T09, T13–T16 (eventos extraordinarios en tránsito)**: `CustodioActual` (público) no cambia respecto del emisor. La operación pasa a cerrada (no puede confirmarse una recepción desde `EN_CUARENTENA` ni desde estados bloqueantes); su registro se conserva sin modificar para auditoría.

El destinatario declarado es provisto por el emisor en la operación de despacho **exclusivamente mediante el campo `transient` de la propuesta**, y es de solo-lectura para el chaincode (desde la PDC) en el resto de las operaciones. La invariante es: **el identificador del destinatario declarado nunca viaja como argumento público de una transacción ni se incluye en una respuesta pública**, porque los argumentos ordinarios de una propuesta quedan registrados en la transacción visible del canal aun cuando el valor se persista después en una PDC; solo el campo `transient` queda excluido de la transacción del canal. El payload concreto del `transient` lo define DES-5. El chaincode valida que el destinatario declarado exista en el registro de organización-establecimiento (ADR-003), esté activo y tenga un `agentType` compatible con el par de transferencia según la matriz de DES-3, antes de aceptar el despacho.

## Por qué DestinatarioPendiente no es estado público

ADR-002 enumeró un conjunto cerrado de campos de estado público del canal (identificador, lote, vencimiento, custodio actual, estado del producto) y clasificó como privado "cualquier dato que permita inferir relaciones o volúmenes comerciales entre partes específicas". El identificador del receptor declarado durante el tránsito cae directamente en esa clasificación: revela una relación emisor → destinatario antes de que la operación se concrete.

Una versión anterior de esta ADR proponía mantener este dato en estado público, con el argumento de que "no agrega información que no sea ya pública" porque, si la recepción se confirma, el receptor pasa a ser `CustodioActual` público de todos modos. Ese argumento no cubre los casos en que la recepción **no** se confirma: rechazo (T05), robo/extravío/deterioro/vencimiento en tránsito (T13–T16) o cuarentena (T09). En esos casos el destinatario declarado nunca llega a ser `CustodioActual`, y aun así, si el campo fuera público, el historial del canal (`GetHistoryForKey`) conservaría indefinidamente el destino intentado de una operación que no se concretó — exactamente el tipo de relación no consumada que ADR-002 protege, y en tensión con el artículo 9(f) de la Disposición ANMAT 3683/2011, que restringe el acceso a información de transacciones ajenas.

No hay necesidad técnica que justifique la excepción: el peer endosante del receptor puede leer datos privados de una PDC de la que su organización es miembro para ejecutar la validación de T04, igual que ya lo hace para leer/escribir factura y remito. Por eso se decide:

- `DestinatarioPendiente` se escribe y se lee desde la PDC de la operación (emisor + receptor declarado + `AnmatMSP`), nunca desde el estado público del canal.
- El estado público del canal solo revela, durante el tránsito, que la unidad está en `EN_TRANSITO` y que `CustodioActual` sigue siendo el emisor — consistente con lo que ADR-002 ya clasificó como público — sin identificar al destinatario intentado.
- `AnmatMSP` conserva visibilidad completa del destinatario declarado por ser miembro de la PDC de toda operación, preservando la auditabilidad regulatoria que motivó adoptar la alternativa A.

Esta decisión no requiere modificar la clasificación de ADR-002: el destinatario declarado queda comprendido, desde el inicio, en la categoría de "dato que permite inferir relaciones... entre partes específicas" que ADR-002 ya asigna a PDC — no es una excepción a esa regla, sino una aplicación directa de ella que una versión anterior de este documento había pasado por alto.

## Referencias documentales de la operación

La Disposición ANMAT 3683/2011 exige que la documentación comercial respaldatoria de toda transferencia incluya número de factura, remito y datos de identificación del origen y destino. Estos datos constituyen información documental en el sentido de ADR-002.

ADR-002 los clasifica explícitamente como privados: "número de factura o remito" forma parte de la "información comercial y documental" que se almacena en Private Data Collections, no en el estado público del canal.

Esta ADR confirma esa clasificación para ambas operaciones, y agrega el destinatario declarado (`DestinatarioPendiente`, ver "Por qué DestinatarioPendiente no es estado público") al mismo contexto privado:
- **Despacho**: el emisor escribe en la PDC de la operación el identificador canónico del destinatario declarado, el número de remito/factura, cantidades despachadas y demás datos documentales exigidos. La membresía de la colección incluye al emisor, al receptor declarado y a `AnmatMSP`.
- **Recepción**: el receptor puede agregar al mismo contexto privado la confirmación de los datos documentales, o crear una entrada diferenciada si la normativa exige registro separado de la recepción.

**Aclaración sobre "PDC de la operación"**: esta expresión, y la membresía "emisor + receptor declarado + `AnmatMSP`", enuncian una **regla lógica de visibilidad por operación**, no la creación dinámica de una colección Fabric por cada transferencia. Las definiciones de colecciones y sus políticas forman parte de la definición del chaincode y no se crean por transacción; Fabric ofrece para materializar esta regla mecanismos como colecciones explícitas predefinidas o colecciones implícitas por organización (escribiendo el dato en la colección implícita de cada parte involucrada más la de ANMAT). La elección del mecanismo, los nombres y la persistencia del hash en el ledger compartido quedan para NET-5, tal como establece ADR-002.

**Actualización posterior**: [ADR-006](006-private-data-collections.md) hizo esa elección — **colecciones explícitas por par de organizaciones**, nombre determinístico `transfer_<mspIdA>_<mspIdB>` con los `mspId` ordenados lexicográficamente para que la transferencia y la devolución en sentido inverso resuelvan a la misma colección. Descartó expresamente las colecciones implícitas para este uso, y por una razón que nace de esta ADR: escribir el registro de operación en las implícitas del receptor y de ANMAT arrastraría sus políticas de endoso y convertiría el **despacho unilateral del emisor** en una operación multiparte. ADR-006 fija además las claves (`TransferOpActive`+[gtin,serie] y `TransferOp`+[gtin,serie,txIdDespacho]) que el punto 1 del ciclo de vida de abajo deja abiertas.

## Rechazo en recepción y relación con EXT-4

El rechazo en recepción es la transición T05 de ADR-001: `EN_TRANSITO → DEVUELTO`, invocada por `DESTINATION_AGENT` o `CURRENT_CUSTODIAN`. El chaincode valida que quien invoca sea la organización declarada como destinatario o el emisor, y que la causa de rechazo esté documentada.

`DEVUELTO` es un estado no terminal de ADR-001 con las siguientes transiciones disponibles: `REINGRESAR_STOCK` (T25), `RETIRAR_MERCADO` (T19), `PROHIBIR_PRODUCTO` (T20), `DISPONER_FINAL` (T33), `INFORMAR_ROBO` (T14), `INFORMAR_EXTRAVIO` (T15), `INFORMAR_DETERIORO` (T16). La issue EXT-4 (devolución entre actores ya en custodia confirmada) cubre el mismo estado destino pero a partir de T21/T22/T23/T24. Esta ADR no define si ambos orígenes de `DEVUELTO` deben tratarse como semánticamente equivalentes en el chaincode: distinguir un rechazo en tránsito (nunca hubo cambio de custodia confirmado) de una devolución tras custodia confirmada es una decisión que corresponde a EXT-4.

**Actualización posterior**: [ADR-009](009-return-and-recovery-semantics.md) la tomó, y en sentido contrario al que esta sección anticipaba. La devolución **no** se modela como entrega/recepción con reversión de custodia: es un **evento único** que no modifica `CustodioActual`, ni en T05 ni en T21–T24. El chaincode **no bifurca** la semántica entre ambos orígenes; los distingue solo por la evidencia registrada — la transición de origen queda en el historial, y el registro de operación en PDC existe únicamente en T05, porque solo un despacho previo lo crea. Modelar el retorno físico como flujo bifásico queda como revisión futura de ADR-009. Esta ADR solo deja constancia de que ambos caminos comparten el mismo estado destino y, por lo tanto, las mismas transiciones de resolución listadas arriba.

## Consecuencias

- **Para ADR-001**: no se requieren cambios. La alternativa A confirma la existencia de `EN_TRANSITO` y las transiciones T02–T05, que ADR-001 ya define. Esta ADR resuelve la única pregunta que ADR-001 dejó abierta (dos transacciones vs. atómica); el cambio de estado de ADR-001 queda a criterio del proceso de aprobación del equipo.
- **Para DES-2 / modelo-datos.md**: el struct público `MedicationUnit` **no** agrega un campo `DestinatarioPendiente`. El identificador del destinatario declarado se persiste en la PDC de la operación (ver "Por qué DestinatarioPendiente no es estado público"), no en el estado público del canal. La sección 5 de modelo-datos.md puede cerrar la dependencia abierta hacia DES-9 con esta aclaración.
- **Para DES-5 (issue #11)**: el contrato público del chaincode debe exponer dos operaciones distintas para la transferencia: una de despacho (invocable por el emisor, que recibe el destinatario declarado por el campo `transient`, nunca como argumento público) y otra de recepción (invocable solo por el destinatario declarado, validado contra la PDC de la operación). Las firmas exactas, parámetros, errores y payloads quedan para DES-5.
- **Para DES-6 / docs/organizations-roles-endorsement.md**: la transacción de recepción (T04) requiere endoso conjunto de la organización emisora y la receptora, conforme a "Endoso" más arriba; DES-6 debe reflejar esto en su tabla de políticas por clase de operación si aún no cubre el caso de confirmación de custodia entre dos organizaciones no-ANMAT.
- **Para CC-3 (issue #16) y BASE-2 (issue #38)**: la implementación del chaincode y del baseline debe respetar el modelo de dos transacciones para que la comparación cuantitativa de DES-7 mida los mismos procesos en ambos prototipos. Los criterios de aceptación de CC-3 (#16) y BASE-2 (#38) fueron actualizados el 2026-08-16 para exigir explícitamente despacho y recepción como dos transacciones separadas, y DES-5 lo incorpora al contrato público con dos operaciones de escritura diferenciadas más la de rechazo (PR #80). Con esto queda satisfecho el criterio correspondiente de la issue #57.
- **Para NET-5 / ADR-002**: las colecciones privadas deben cubrir despacho y recepción, con el mismo esquema de membresía (emisor + receptor declarado + `AnmatMSP`), e incluir ahora también el identificador del destinatario declarado, no solo los datos documentales.
- **Se gana**: fidelidad normativa, auditabilidad del hecho separado de cada parte, soporte de rechazo en recepción sin mecanismos externos, sin exponer en estado público relaciones emisor-destinatario no consumadas.
- **Se pierde / costo**: mayor complejidad de implementación; la ventana de tiempo en `EN_TRANSITO` debe manejarse correctamente ante eventos extraordinarios (T09, T13–T16 de ADR-001); la validación de recepción depende de acceso a PDC en vez de estado público, lo que ata su disponibilidad al diseño concreto de NET-5.
- **Queda pendiente**: el mecanismo de idempotencia para el caso en que el receptor invoque la operación de recepción más de una vez, y el comportamiento esperado si el despacho queda en `EN_TRANSITO` de forma indefinida sin que el receptor confirme ni rechace (queda fuera del alcance del prototipo salvo que una issue específica lo incorpore).

## Contexto utilizado

- Issue GitHub #57: DES-9 · ADR-004, consultada el 2026-08-13.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): fuente de los estados `EN_TRANSITO`, `DEVUELTO` y las transiciones T02–T05, T09, T13–T16.
- [ADR-002: Topología de canales](002-topologia-canales.md): clasificación de factura/remito como datos privados de PDC.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): base para validar existencia y habilitación del destinatario declarado.
- [DES-2 / docs/modelo-datos.md](../modelo-datos.md): dependencia abierta resuelta por esta ADR — el destinatario declarado se persiste en PDC, no en el struct público.
- [DES-6 / docs/organizations-roles-endorsement.md](../organizations-roles-endorsement.md): base de la política de endoso citada en "Endoso"; esta ADR fija quién invoca cada transacción, DES-6 fija quién la endosa.
- Disposición ANMAT 3683/2011, artículo 8: enumera distribución al siguiente eslabón y recepción en establecimiento como movimientos logísticos separados con agentes detonantes distintos. URL: https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/actualizacion
- Resolución MS 435/2011, artículo 1: exige que cada agente implemente un sistema de trazabilidad que permita el control y seguimiento de las unidades. URL: https://www.argentina.gob.ar/normativa/nacional/resoluci%C3%B3n-435-2011-180934/texto
