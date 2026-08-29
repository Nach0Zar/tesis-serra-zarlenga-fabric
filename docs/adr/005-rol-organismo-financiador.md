# ADR-005: Rol del organismo financiador en la dispensación

- **Estado**: Aceptado
- **Fecha**: 2026-08-13
- **Autores**: Serra, Zarlenga

---

## Contexto

DES-6 introdujo `FinanciadorMSP` como una organización no custodial dentro de la red del prototipo, con un rol reservado (`financier-auditor`) que "no habilita escrituras ni custodia", y difirió explícitamente la definición de su comportamiento concreto a DES-10 y CC-8. Esta ADR resuelve esa reserva: define qué hace el organismo financiador (PAMI, obras sociales) en el prototipo.

El marco normativo y el relevamiento del proyecto delimitan el rol del financiador:

- El paper representa gráficamente la dispensación al paciente por parte de un centro médico o farmacia "y su validación por parte de un organismo financiador". La figura agrupa ambos pasos sin especificar si la validación es previa o posterior a la dispensación física; una lectura puramente visual admite ambas secuencias.
- El texto del paper sí es explícito sobre la secuencia: "El PAMI efectúa una validación informática del trazado de los productos listados e impone la correcta trazabilidad en el SNT como condición excluyente para la liberación de pagos a los laboratorios." La condición de trazabilidad habilita el pago, no la entrega física — el pago a laboratorios es un evento necesariamente posterior a que el producto ya circuló y fue dispensado.
- El relevamiento de campo del proyecto (entrevista a un actor del sector que gestiona la relación entre farmacias y financiadores) corrobora el mismo orden desde la práctica: describe la validación de cobertura en el momento de dispensación como un circuito administrativo separado ("distintas aristas del mismo problema... nuestra gestión es más administrativa para garantizar o no el pago de los financiadores"), y un proceso de auditoría posterior que puede incluso reabrir una solicitud ya aprobada antes de resolver el pago ("un equipo de auditores se toma el caso puntual para analizar si se avanza o no con el reclamo y con el pago posterior").
- Esta ADR privilegia la secuencia que fijan el texto normativo y la entrevista (validación de trazabilidad como condición de pago, posterior a la dispensación) por sobre una lectura de la figura que no distingue temporalidad explícitamente. Esta es una decisión de interpretación consciente, no una lectura unívoca de la única fuente gráfica.
- La normativa del Instituto Nacional de Servicios Sociales para Jubilados y Pensionados (PAMI) condiciona el pago al cumplimiento del SNT: convalida las dispensaciones **ya informadas** al sistema y supedita a esa convalidación el pago de las liquidaciones de la industria (Resolución PAMI 1735/2016, Disposición PAMI 1/17). Esto es todo lo que estas normas establecen en su texto, y confirma la secuencia adoptada: la convalidación opera sobre dispensas ya ocurridas e informadas.
- El acceso de auditoría de PAMI a la base de datos central del SNT es una afirmación del relevamiento académico del proyecto (paper, sección "Organismos financiadores"), no de las resoluciones citadas en el punto anterior, que no lo establecen expresamente. Esta ADR lo toma como caracterización del proceso relevado, no como obligación normativa.
- El financiador integra el conjunto de agentes externos con acceso al SNT que obtienen usuario para verificar y auditar las dispensas realizadas a sus beneficiarios, sin ser custodios ni participar de la circulación física del medicamento.
- La Ley 25.326 de Protección de Datos Personales define los datos personales y sensibles del afiliado (art. 2); esta ADR decide, como cuestión de diseño y no como mandato textual de la ley, no persistirlos en el ledger, en línea con la restricción que ya fija `modelo-datos.md` (§4) y con el alcance de dispensación de CC-4.

El problema de diseño es que estos requisitos están en tensión: el financiador debe poder validar la trazabilidad para liberar un pago, pero no debe ser custodio, no debe acceder a la información comercial de operaciones ajenas (ADR-002) y no debe incorporar datos personales del afiliado al ledger.

## Alternativas

**A. Validación previa: el financiador endosa o autoriza la dispensación**

- El financiador participa como coendosante o autorizador de la transacción de dispensación, de modo que la dispensa no se confirma sin su intervención.
- No representa el proceso real de la validación de trazabilidad específicamente: el texto del paper y el relevamiento de campo (ver "Contexto") coinciden en que esa validación es una condición para **liberar el pago**, posterior a la dispensación física al paciente, no una precondición para entregarla. (La figura del paper no contradice esto — solo no distingue la secuencia explícitamente; ver "Contexto".) Esta ADR no toma posición sobre si existe además una autorización de cobertura en tiempo real al momento de dispensar (el relevamiento sugiere que sí, como circuito administrativo separado del SNT) — esa autorización de cobertura no es una validación de trazabilidad y queda fuera del alcance de esta ADR.
- Coloca a un actor no custodial en el camino de endoso de una operación core, agregando latencia y una dependencia de disponibilidad: cada dispensa a un afiliado del financiador requeriría a ese financiador en línea para endosar. Contradice DES-6, que establece que el financiador no habilita escrituras.
- Empuja la identidad del afiliado al momento de validación de la dispensación, en tensión con la Ley 25.326.
- Se descarta.

**B. Verificación posterior: el financiador consulta la traza antes de liberar el pago**

- El financiador es una organización no custodial de solo lectura. Su "validación" consiste en consultar el estado público de trazabilidad y el historial de la unidad dispensada, para confirmar que fue producida, transferida por la cadena autorizada y dispensada por un agente habilitado, antes de liberar el pago correspondiente.
- El pago en sí mismo es un flujo externo al ledger y queda fuera del alcance del prototipo.
- Consistente con DES-6 (`financier-auditor`, sin escrituras), con ADR-002 (visibilidad de lectura del estado público, sin acceso a datos comerciales ajenos) y con la evidencia normativa sobre el rol de PAMI.
- Se adopta.

**C. Exclusión del alcance**

- No se modela el financiador en el prototipo; se documenta que su flujo de pago no altera la cadena de trazabilidad.
- Es defendible, pero desaprovecha que el paper representa explícitamente la validación del financiador como parte del proceso de dispensación y que su verificación de trazabilidad es un consumidor legítimo del registro que el prototipo puede representar sin costo arquitectónico significativo mediante lectura.
- Se descarta como decisión por defecto, aunque su footprint on-ledger es deliberadamente mínimo (ver Decisión).

## Decisión

Se adopta la **alternativa B**: el organismo financiador se modela como una **organización no custodial de solo lectura** que verifica la trazabilidad de una unidad dispensada con posterioridad a la dispensa, como condición para liberar el pago.

1. El financiador **no endosa ni autoriza** la transacción de dispensación. La dispensación se gobierna por ADR-001 (T06) y DES-6 (custodio con `agentType=PHARMACY` o `HEALTHCARE_FACILITY`, rol `operator`), sin intervención del financiador.
2. La "validación por parte del organismo financiador" se materializa como una **consulta de lectura** sobre el estado público de trazabilidad y el historial (`GetHistoryForKey`) de la unidad. `GetHistoryForKey` devuelve la secuencia de modificaciones confirmadas de la clave en el world state — no incluye transacciones inválidas o rechazadas, que nunca llegan a modificarlo. Esta ADR fija qué debe confirmar la consulta (existencia de la unidad, estado `DISPENSADO`, agente dispensador habilitado), pero **no** define qué constituye una "traza irregular" más allá de eso — por ejemplo, si una transferencia fue autorizada según la matriz de DES-3 vigente al momento de esa transferencia, si algún actor en la secuencia estuvo habilitado en el momento del evento y no solo actualmente, o si falta algún evento esperado en la secuencia. Definir esa semántica verificable en detalle es un prerequisito para CC-8, no una decisión que esta ADR resuelve; la firma concreta de la función de consulta tampoco se define acá y queda para DES-5/CC-8.

   **Actualización posterior — ambas quedaron cerradas.** [ADR-011](011-financier-trace-verification.md) fija la semántica como una **checklist determinística de cinco comprobaciones** en orden (existencia; estado `DISPENSADO`; dispensador con `agentType` habilitado; camino de estados válido contra la tabla de ADR-001; pares de transferencia autorizados contra la matriz embebida de ADR-008) con un **veredicto estructurado** y veredictos nombrados. Las tres preguntas abiertas de este punto quedan respondidas así: la matriz «vigente en su momento» se resuelve con la declaración de matriz única de ADR-008 y su límite heredado; la habilitación **histórica** de actores se descarta en v1 y se declara como límite (el registro persiste el `active` actual); y la integridad de la secuencia se verifica recomputando el camino. La firma está congelada en el contrato DES-5: `VerifyTrace(ctx, gtin, numeroSerie) (*TraceVerdict, error)`. CC-8 implementa, no diseña.
3. La verificación es **dirigida por reclamo (claim-driven)**: el financiador consulta la traza de los seriales específicos que se le presentan en la facturación off-ledger, no enumera ni recorre la totalidad de las dispensaciones del canal.
4. **El pago es off-ledger** y no se representa como transacción del prototipo.
5. **No se persiste ningún dato personal del afiliado en el ledger**, ni siquiera obra social o número de afiliado. El vínculo afiliado ↔ unidad dispensada vive en el sistema de facturación externo; el financiador parte de ese vínculo off-ledger y usa el ledger solo para confirmar la legitimidad de la traza del serial.

## Alcance de lectura y relación con ADR-002

El financiador es una organización miembro del canal con acceso de lectura al **estado público de trazabilidad** definido por ADR-002 (identificador, lote, vencimiento, custodio actual, estado). Ese estado público no contiene información comercial, por lo que la lectura del financiador nunca expone precios, condiciones ni documentos de operaciones ajenas.

El financiador **no es miembro de las Private Data Collections** de operaciones en las que no participa. No accede a la información comercial ni documental (factura, remito, cantidades) de las transferencias entre otros establecimientos, conforme a ADR-002.

**Límite no garantizado técnicamente — supuesto de confianza del prototipo.** La verificación claim-driven (punto 3 de la Decisión) es una convención de uso, no una restricción de acceso exigible por el chaincode: cualquier peer del canal, incluido el del financiador, tiene acceso de lectura al estado público completo de **todas** las unidades, no solo a las de sus propios afiliados — eso es una propiedad del modelo de canales de Fabric que ADR-002 ya estableció para todo miembro del canal, y esta ADR no la modifica. El chaincode no tiene forma de saber, para una consulta de lectura dada, si el serial consultado corresponde a un afiliado real del financiador que la ejecuta, porque el vínculo afiliado↔unidad vive deliberadamente off-ledger (ver "Datos del afiliado y Ley 25.326"). En consecuencia, "el financiador solo puede ver unidades de sus afiliados" no es una propiedad garantizada por el diseño: es un supuesto de buen comportamiento del prototipo, equivalente a asumir que el financiador solo consulta los seriales que efectivamente le llegaron por facturación.

Cerrar esta brecha con una garantía técnica real requeriría una frontera que el estado público del canal, tal como lo define ADR-002, no provee — por ejemplo, un servicio intermediario que medie las consultas del financiador validando el vínculo afiliado↔unidad antes de reenviarlas al chaincode, o mover el estado consultable a una estructura que sí permita filtrar por relación. Cualquiera de esas alternativas es un cambio de topología de canales/colecciones y queda fuera del alcance de esta ADR: se deja como decisión pendiente para una revisión futura de ADR-002/NET-5, no como algo que el prototipo v1 resuelve.

Impacto para NET-5: la configuración de red debe otorgar al financiador lectura del estado público del canal sin incorporarlo como miembro de las colecciones privadas comerciales. No se define en esta ADR ninguna colección privada específica del financiador; si un requerimiento futuro la justificara, sería una decisión aparte fuera de este alcance.

## Datos del afiliado y Ley 25.326

El ledger no almacena datos personales del afiliado. La verificación claim-driven permite que el financiador confirme la legitimidad de una dispensa **sin** que el número de afiliado ni la obra social se registren en cadena:

- El sistema de facturación externo mantiene el vínculo "serial X fue dispensado al afiliado Y de la cobertura Z".
- El financiador toma el serial del reclamo y consulta el estado público y el historial de esa unidad en el ledger.
- Si la traza es legítima (producción, cadena autorizada, dispensación por agente habilitado), la condición de trazabilidad para liberar el pago queda satisfecha.

Esto mantiene la coherencia con `modelo-datos.md` (§4), que ya excluye del activo la identificación individual del afiliado más allá de lo necesario para la cobertura, y con el alcance de dispensación de CC-4. La decisión de mantener el número de afiliado fuera incluso de una colección privada es deliberada: el prototipo no necesita el vínculo on-ledger para representar la verificación de trazabilidad, y evitarlo elimina el dato sensible del alcance del ledger por completo.

## Consecuencias

- **Para DES-6**: confirma el rol `financier-auditor` como de solo lectura. El financiador no invoca transferencias, dispensación ni eventos extraordinarios, tal como DES-6 anticipaba.
- **Para CC-8**: la implementación del comportamiento del financiador se reduce a operaciones de consulta sobre el estado público y el historial. No introduce funciones que muten estado. La firma quedó congelada en el contrato DES-5 (`VerifyTrace`) y la semántica en ADR-011.
- **Para ADR-002 / NET-5**: el financiador obtiene lectura del estado público del canal, sin membresía en colecciones privadas comerciales ajenas.
- **Para CC-4 / modelo-datos.md**: no se agrega ningún campo al struct `MedicationUnit` ni se incorpora dato de afiliado al ledger. La dispensación no cambia su modelo por esta ADR.
- **Para la baseline**: la línea base centralizada debe replicar la verificación de trazabilidad de solo lectura del financiador para preservar la paridad funcional de la comparación (DES-7).
- **Se gana**: fidelidad al proceso real (validación posterior para liberar pago), footprint on-ledger mínimo del financiador, cero dato personal del afiliado en cadena, sin dependencia de disponibilidad del financiador para operar la dispensación.
- **Se pierde / costo**: el prototipo no representa el flujo de pago ni la conciliación económica; la verificación depende de un vínculo afiliado↔unidad que permanece fuera del ledger y, por lo tanto, fuera de las garantías de integridad de la cadena. Además, "solo unidades de sus afiliados" no es una restricción exigible por el chaincode — es un supuesto de confianza del prototipo (ver "Alcance de lectura y relación con ADR-002"); un financiador podría, técnicamente, leer el estado público de cualquier unidad del canal.
- **Queda pendiente**: si una iteración futura quisiera representar on-ledger la relación entre una dispensa y una cobertura (por ejemplo, para auditar volúmenes por financiador sin exponer al afiliado), debería evaluarse una colección privada específica bajo una ADR propia; queda fuera del alcance del prototipo v1. También queda pendiente cerrar con una garantía técnica real el supuesto de confianza señalado arriba, si una futura revisión de ADR-002/NET-5 lo justifica; y —**ya resuelto**— definir la semántica precisa de "traza irregular" que CC-8 necesita: la fijó ADR-011 (ver la actualización del punto 2 de la Decisión).

## Contexto utilizado

- Issue GitHub #58: DES-10 · Rol del organismo financiador en la dispensación, consultada el 2026-08-13.
- [DES-6: Organizaciones, MSP, roles, ABAC y políticas de endoso](../organizations-roles-endorsement.md): define `FinanciadorMSP` como organización no custodial y reserva su comportamiento a DES-10/CC-8.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): la dispensación (T06) no involucra al financiador.
- [ADR-002: Topología de canales](002-topologia-canales.md): estado público de trazabilidad legible por miembros del canal; información comercial restringida a PDC.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): modelo de organización por establecimiento sobre el que se apoya la membresía no custodial del financiador.
- [modelo-datos.md](../modelo-datos.md), §4: exclusión de datos personales y de identificación del afiliado del activo de trazabilidad.
- Paper del proyecto, representación gráfica de la dispensación al paciente y su validación por parte de un organismo financiador: fuente de la figura citada; ver "Contexto" sobre por qué esta ADR no toma su adyacencia visual como evidencia de secuencia temporal.
- Paper del proyecto, sección "Organismos financiadores": fuente textual de la secuencia validación-de-trazabilidad-como-condición-de-pago, y del relevamiento académico sobre acceso de auditoría de PAMI a la base central — no las resoluciones citadas más abajo, que no lo establecen expresamente en su texto.
- Relevamiento de campo del proyecto, entrevista a un actor del sector con relación directa entre farmacias y financiadores: evidencia primaria de entrevista requerida por la issue #58. Corrobora desde la práctica que la validación de cobertura/trazabilidad de un financiador es un circuito posterior a la dispensación, orientado a resolver el pago, y documenta la exclusión deliberada de datos sensibles del afiliado pese a estar disponibles, en línea con la decisión de esta ADR sobre datos personales.
- Resolución PAMI 1735/2016 y Disposición PAMI 1/17: hablan de convalidar dispensaciones ya informadas y condicionar el pago de liquidaciones a la industria al cumplimiento del SNT. No establecen expresamente, en su texto, el acceso de auditoría de PAMI a la base central.
- [Ley 25.326 de Protección de Datos Personales](https://servicios.infoleg.gob.ar/infolegInternet/anexos/60000-64999/64790/texact.htm), art. 2 (define datos personales y sensibles) y art. 4 (principios de pertinencia y no exceso): esta ADR excluye del ledger los datos del afiliado como decisión de diseño respaldada en estos principios, no como mandato textual del art. 2, que solo define categorías sin imponer una arquitectura.
