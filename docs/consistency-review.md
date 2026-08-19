# Revisión de congruencia del proyecto — 2026-08-16

> **Estado de remediación (actualizado 2026-08-16, branch `fix/consistency-review`)**
>
> - **Resueltos en el repo**: C1 (template real), C2+E8a (CI con filtro de paths, actions v7, payloads alineados al contrato), C3/C4/C6/C9/C10 (api-contract 2.0.1, vía PR #80), C7 (ADR-001 y DES-6 Aceptados), C8 (nota en tabla de estados), C11 (READMEs reales), E1+D9 (exclusiones en alcance-prototipo), E3/D8 (measurement-protocol §3.4), E4 (DES-6 post-ADR-005), E6 (nota CUFE en ADR-003), E7-repo (lineamiento `motivo` en el contrato). Los pendientes de PR #78 (ciclo de vida de DestinatarioPendiente, invariante transient, aclaración de PDC lógica) quedaron resueltos en ADR-004.
> - **Derivados a decisión y ya resueltos (2026-08-17)**: C5 (actor `RECOVERY_OR_DISPOSAL_AGENT` sin mapeo) quedó resuelto por [ADR-009](adr/009-return-and-recovery-semantics.md), que lo define como el custodio actual registrado; E5 (identidad de ANMAT y financiadores) por [ADR-010](adr/010-non-custodial-identity.md), que extiende el registro con `agentType` no custodiales; E8b (paginación) quedó registrado como exclusión en `alcance-prototipo.md`. Las siete decisiones del roadmap son ahora ADR-006 a ADR-012.
> - **Requieren edición manual del trabajo escrito**: A1–A5, B1–B7, E2, E6/E7 (parte tesis) — instrucciones paso a paso en [`paper-update-instructions.md`](paper-update-instructions.md).
> - **Segunda ronda de review del PR #88 (2026-08-17)**: se corrigieron la premisa sobre colecciones implícitas de ADR-006, el contrato de configuración de las colecciones, el bootstrap del regulador en ADR-010, la cita del paper y el mapeo de T05 en ADR-009, el alcance probatorio de ADR-007, la superficie del contrato (v2.1.0) y los filtros de CI.
> - **Tercera ronda de review del PR #88 (2026-08-18)**: se rediseñó la materialización del endoso dinámico en ADR-007 —una política de endoso basado en estado se evalúa contra el estado **anterior** a la transacción y no puede condicionarse a la operación intentada—, se resolvió el cierre regulatorio contra la política de la colección (ADR-006), se ancló el bootstrap al `packageID` aprobado en el lifecycle (ADR-010), se fijaron las reglas de validación del receptor de devolución (ADR-009), se llevó el contrato a v2.2.0 con `Init` y `AuthorizeLabIntervention`, y se propagó el límite de metadatos de ADR-006 al manual de actualización (entrada 1.9).
> - **Cuarta ronda de review del PR #88 (2026-08-18)**: se corrigió el error de fondo que arrastraban las rondas anteriores — la política de endoso es de la **clave**, no de la función invocada —, con lo que la rama `AnmatMSP` de las políticas de clave desaparece (habilitaba el endoso unilateral del regulador en operaciones ordinarias), la clave de autorización de intervención de laboratorio pasa a exigir `AND(laboratorio, AnmatMSP)`, se declara la ventana de creación de una clave que impide imponer la exclusividad del laboratorio en `RegisterUnit`, y se corrige la atribución a la política de colección de una garantía sobre el custodio actual que no da. Contrato en v2.3.0.
> - **Quinta ronda de review del PR #88 (2026-08-19)**: la review aportó el mecanismo que faltaba — la política de endoso de una colección implícita pertenece a su organización dueña y rige desde el despliegue, de modo que escribir un **marcador de participación** en ella exige el endoso de su peer incluso en la primera escritura de una clave. Con ese patrón único se recupera el coendoso real del regulador en los eventos regulatorios (que había quedado apoyado en la firma de creador), se cierra la ventana de creación de `RegisterUnit` sin serializar las altas, y la clave de autorización de intervención vuelve a SBE regulatoria, eliminando el bloqueo permanente que producía una SBE conjunta al reemplazar una autorización vencida. Se corrigió además la afirmación central del trabajo, que era falsa en su versión amplia. Contrato en v2.4.0.

> - **Sexta ronda de review del PR #88 (2026-08-19)**: la review llegó como **propuesta concreta** sobre el head `4c4d4cb` y no como lista de hallazgos. Se corrigieron dos consecuencias no advertidas de la ronda anterior. **Política de endoso de chaincode**: enumeraba solo a las organizaciones custodiales, con lo que la primera escritura de una entrada del registro o de una clave de autorización —actos que DES-6 reserva a la autoridad— exigía el endoso accidental de una custodial cualquiera; pasa a `OR(custodiales, regulatoria)`, y ADR-007 gana el punto 6.j, que delimita qué gobierna esa política (el conjunto cerrado de primeras escrituras) y declara la invariante que lo sostiene: toda clave pública nueva se escribe junto al marcador de su organización responsable. **Revocación de una autorización de intervención**: el contrato afirmaba que la autoridad podía anularla reemitiéndola con un `expiraEn` ya alcanzado, pero su propia validación exige una fecha futura; se agrega `RevokeLabIntervention` con el campo `estado` (`ACTIVA`/`CONSUMIDA`/`REVOCADA`). Se corrigió además, por revisión propia, el esquema de clave del marcador de participación, definido solo por unidad y por lo tanto no construible para las operaciones del registro organización-establecimiento que el propio ADR-007 exige marcar. Contrato en v2.5.0. Se hicieron verdes los workflows de CI con un job de preflight que detecta si el módulo del chaincode ya existe.
> - **Revisión cruzada de los trece ADR (2026-08-19)**: revisión propia, no derivada de un review externo, de ADR-000 a ADR-012 entre sí y contra el contrato, DES-6 y el modelo de datos. Catorce hallazgos. **Contradicciones**: el contrato omitía al `DESTINATION_AGENT` que ADR-001 habilita en T09 y T13, con lo que un receptor no podía poner en cuarentena una unidad anómala al recibirla —durante el tránsito el custodio registrado es el emisor— y omitía al `LABORATORY` de T27 (contrato v2.6.0, y la misma precisión en DES-6); `modelo-datos.md` derivaba la membresía de las colecciones del registro del ledger, entrada que ADR-006 descarta expresamente por dependencia circular con el bootstrap; el checklist normativo de paridad de ADR-012 no cubría `AuthorizeLabIntervention`/`RevokeLabIntervention`, incorporadas al contrato después de escribirlo, de modo que las rondas de rechazo esperado habrían medido universos distintos en los dos SUT. **Punteros obsoletos**: seis ADR seguían declarando pendiente algo que un ADR posterior ya había decidido —ADR-002 y ADR-004 la elección del mecanismo de PDC (ADR-006), ADR-004 la semántica de la devolución (ADR-009), ADR-005 la semántica de traza legítima y la firma de la consulta (ADR-011 y el contrato), ADR-003 el transporte del destinatario y el catálogo del registro (ADR-004 y ADR-010), ADR-001 la granularidad de la transferencia (ADR-004)—, y ADR-006 y ADR-008 se remitían mutuamente como no resueltos. Los cinco ADR en estado *Aceptado* se anotaron con bloques «Actualización posterior» en el lugar exacto de la afirmación obsoleta, sin alterar ninguna decisión. **Higiene**: el checklist de sincronización del roadmap todavía afirmaba que cada caída de Raft es la de una organización distinta, la afirmación que ADR-007 corrigió.
> - **Séptima ronda de review del PR #88 (2026-08-19)**: seis P1, todos confirmados. **(1)** ADR-007 duplicaba la tabla normativa de ADR-001 enumerando los eventos iniciables por ANMAT: omitía siete transiciones habilitadas (T07, T08, T10, T11, T12, T26, T30) e incluía una que ADR-001 reserva al agente de recupero (T29); además listaba `PROHIBIDO` como terminal —ADR-001 admite T23 y T32 desde ahí— y omitía `ROBADO` y `EXTRAVIADO`. La regla pasa a enunciarse **por referencia** a la columna «Actor habilitado» de ADR-001, sin reproducir IDs. **(2)** El bootstrap de ADR-010 se apoyaba en una propiedad falsa del lifecycle: el `packageID` **no** integra la definición confirmada en el canal, es un parámetro local de `approveformyorg` que Fabric no exige que coincida entre organizaciones. Caído ese ancla, la raíz de confianza se reconstruye sobre lo que el `AND` de la secuencia 1 sí prueba —todas las fundacionales ejecutaron y coincidieron en quién es el regulador— más la verificación operativa del artefacto (`queryinstalled`/`queryapproved` contra el `packageID` versionado) en NET-4. Lo mismo invalidaba el «todos ejecutan el mismo binario» de ADR-008; se corrige y se agrega una **comprobación cruzada en la recepción**, que es donde `AND(emisor, receptor)` permite contrastar la matriz que el despacho evaluó por sí solo. **(3)** «Una única Fabric CA» confundía proceso servidor con CA lógica: compartir la **raíz** entre MSPs debilita la garantía criptográfica sobre la que ADR-003 se apoya para descartar la suplantación entre establecimientos. Pasa a un proceso servidor con una CA lógica y raíz por organización (`cafiles`); la limitación declarada es el operador compartido. **(4)** ADR-006 no definía clave para el receptor de una devolución T21–T24, que ADR-009 y el contrato prometen persistir: no nace de un despacho y no tiene `txIdDespacho`. Se agrega `ReturnOp`+[gtin, numeroSerie, txIdDevolucion], y su análogo `return_operations` en ADR-012. **(5)** Las consecuencias de ADR-006 rebajaban el endoso a «una de las partes» y a la disponibilidad de «al menos una», contradiciendo su propia decisión; el contrato arrastraba el mismo residuo. Durante el tránsito la clave pública lleva `AND(emisor, receptor)` y la política de colección es barrera adicional, no sustituto. **(6)** El esquema de ADR-012 no permitía implementar su propio checklist de paridad: `organizations` no admitía `REG:` y `unit_events` guardaba solo el estado resultante, sin `custodioActual`, con lo que ni `GetHistoryForKey` ni la comprobación 5 de ADR-011 eran replicables. Pasa a guardar el snapshot público completo. Contrato en v2.6.1.
Revisión estricta de congruencia entre el trabajo escrito (paper de congreso y avance de tesis, ambos en `docs/papers/`) y todos los artefactos del repositorio (ADRs, modelo de datos, DES-3, DES-6, contrato de API, protocolo de medición, CI). Cada hallazgo indica: **dónde está**, **qué está mal**, **por qué importa** y **cómo arreglarlo**, con el detalle suficiente para que pueda resolverlo un agente sin contexto previo.

Convención de nombres usada acá (solo para poder señalar el lugar exacto del problema; en la documentación del repo se sigue citando "el paper del proyecto" de forma unificada):

- **Paper (congreso)** = `docs/papers/paper-UADE-con-nombre-autores.pdf` (10 páginas).
- **Avance de tesis** = `docs/papers/PFI - Serra y Zarlenga - Entrega 25% (1).pdf` (50 páginas internas).

Severidad: 🔴 Crítico (bloquea/da vergüenza en la entrega final) · 🟠 Alto (contradicción real entre artefactos) · 🟡 Medio (inconsistencia o error acotado) · 🔵 Bajo (precisión conceptual / prolijidad).

---

## A. Contradicciones entre el trabajo escrito y el diseño del repositorio

### A1. 🟠 Una organización por categoría (paper) vs. una organización por establecimiento (ADR-003)

- **Dónde**: Paper (congreso), §3.3: "cada categoría de actor reconocida por el SNT se representa como una organización con identidad gestionada a través de un Proveedor de Servicios de Membresía (MSP)". Avance de tesis, §1.2.1, lista las MSP por categoría de actor. En cambio, `docs/adr/003-establishment-identity-gln-cufe.md` (Aceptado, rev. 2) decide exactamente lo contrario: **cada establecimiento (GLN/CUFE) es su propia organización Fabric**, y descarta explícitamente la MSP compartida por categoría (alternativa A). `docs/organizations-roles-endorsement.md` (DES-6) y ADR-002 asumen el modelo de ADR-003.
- **Por qué importa**: es la decisión arquitectónica central de identidad. Hoy el texto publicado describe una arquitectura que el diseño del repo ya abandonó con argumentos fuertes (la unidad de confidencialidad de Fabric es la organización). Un jurado que lea el paper y luego el prototipo va a encontrar dos arquitecturas distintas.
- **Cómo arreglar**: en la próxima versión del documento de tesis, reescribir la sección de arquitectura para reflejar ADR-003: cada categoría de actor sigue existiendo como *categoría normativa* (`agentType`), pero la organización Fabric es por establecimiento, con el registro organización-establecimiento como traducción `mspId → GLN/CUFE`. Si el paper de congreso ya fue enviado y no puede tocarse, documentar la evolución en la tesis ("el diseño inicial contemplaba una MSP por categoría; el análisis de confidencialidad condujo a una organización por establecimiento, ver ADR-003") para que la divergencia quede explicada y no parezca un descuido.

### A2. 🟠 Tres versiones incompatibles de los flujos de transferencia autorizados

- **Dónde**:
  1. Paper (congreso), §3.2: autoriza solo **5 pares**: laboratorio→droguería, laboratorio→farmacia, laboratorio→centro médico, droguería→farmacia, droguería→centro médico. No menciona distribuidoras ni operadores logísticos en los flujos (aunque §3.1 los lista como actores).
  2. Avance de tesis, Figura 2.1 (p. 14): muestra laboratorio→droguería, laboratorio→distribuidora, **droguería→distribuidora/operadora logística**, distribuidora→farmacia y distribuidora→hospital.
  3. `domain/authorized-transfers.json` (DES-3, fuente única de verdad declarada): **16 pares**, incluyendo droguería→droguería y farmacia→establecimiento asistencial, y **sin** droguería→distribuidora ni droguería→operador logístico.
- **Por qué importa**: la Figura 2.1 dibuja un flujo (droguería→distribuidora) que la matriz regulatoria del propio proyecto **rechaza** (defaultDecision DENY). Y el paper omite 11 de los 16 pares que el chaincode va a autorizar. El prototipo va a comportarse distinto de lo que ambos textos describen.
- **Cómo arreglar**: tratar `domain/authorized-transfers.json` como la verdad (es lo que dice `domain/README.md`) y corregir los otros dos artefactos: (a) rehacer la Figura 2.1 del avance de tesis para que cada flecha corresponda a un par de la matriz (o al menos no incluya pares denegados); (b) en la tesis/paper, reemplazar la enumeración de 5 pares por una referencia a la matriz completa de 16 pares, o aclarar explícitamente que el texto enumera un subconjunto ilustrativo. Si droguería→distribuidora/operador logístico es un flujo real que debería estar permitido (p. ej. logística tercerizada de una droguería), entonces el fix va al revés: agregar el par a la matriz con su referencia normativa y subir `schemaVersion` a MINOR. Hay que decidir una de las dos direcciones, no dejar las tres versiones.

### A3. 🟠 Aislamiento de información: el paper promete más de lo que el diseño da (a propósito)

- **Dónde**: Paper (congreso), §3.4: "El aislamiento de la información entre actores garantiza que ningún establecimiento acceda a información correspondiente a transacciones de las que no forma parte, con excepción de la autoridad de aplicación". Pero `docs/adr/002-topologia-canales.md` decide que el **estado mínimo de trazabilidad** (GTIN, serie, lote, vencimiento, custodio actual, estado) es **público para todos los miembros del canal**, y solo la información comercial/documental va a PDC. ADR-002 reconoce esto como una interpretación del artículo 9 que "no está en el paper de forma explícita".
- **Por qué importa**: leído literalmente, el paper afirma una propiedad (aislamiento total salvo ANMAT) que el prototipo deliberadamente no cumple: cualquier establecimiento del canal puede leer el custodio actual y el estado de cualquier unidad. La justificación de ADR-002 es sólida (validación independiente del receptor), pero hoy vive solo en el ADR.
- **Cómo arreglar**: incorporar al documento de tesis la sección "Interpretación normativa y clasificación de datos" de ADR-002 (dato regulatorio mínimo con visibilidad de canal vs. dato comercial en PDC), presentándola como decisión de diseño consciente y discutible, tal como el propio ADR exige en su sección "Consecuencias → Para la tesis". Ajustar la frase de §3.4 para que la garantía de aislamiento se predique de la **información comercial y documental**, no de todo dato de la transacción.

### A4. 🟠 Momento de la validación del financiador: la figura del paper contradice ADR-005

- **Dónde**: Paper (congreso), Figura 2 (§3.2): el flujo de dispensación muestra "Solicitud de dispensación" → "Validación de cobertura" (Organismo Financiador) → "Registrar dispensación" (SNT) → confirmaciones. Es decir, la validación del financiador aparece **antes** del registro de la dispensa y dentro de su camino. `docs/adr/005-rol-organismo-financiador.md` decide lo contrario: el financiador **no** endosa ni autoriza la dispensación; es un verificador de solo lectura **posterior**, como condición para liberar el pago.
- **Por qué importa**: ADR-005 descarta explícitamente la "validación previa" (alternativa A) con buenos argumentos, pero la única representación gráfica publicada del proceso muestra la alternativa descartada. ADR-005 intenta suavizarlo diciendo que "una lectura puramente visual admite ambas secuencias", pero la figura tiene flechas secuenciales: la validación ocurre antes del registro.
- **Cómo arreglar**: en la próxima iteración del documento, rehacer la Figura 2 mostrando dos carriles separados: (1) dispensación farmacia/centro médico → SNT (sin financiador en el camino), y (2) verificación posterior del financiador (lectura de la traza) → liberación de pago off-ledger. Alternativamente, mantener la figura como "circuito administrativo de cobertura" pero rotular explícitamente que esa autorización de cobertura es un circuito separado del SNT y que la validación de *trazabilidad* del financiador es posterior (como distingue ADR-005).

### A5. 🔵 Atribución de los financiadores y ANMAT a la Disposición 3683/2011

- **Dónde**: Paper (congreso), §3.1: "De acuerdo con la Disposición 3683/2011 de la ANMAT, la cadena de suministro contemplada por el Sistema Nacional de Trazabilidad involucra a los laboratorios..., a los organismos financiadores, como las obras sociales y el PAMI..., y a la propia ANMAT".
- **Qué está mal**: la Disposición 3683/2011 (art. 2) define como agentes a laboratorios, distribuidoras, operadores logísticos, droguerías, farmacias y establecimientos asistenciales. Los organismos financiadores no son agentes de esa disposición; obtienen acceso al SNT por normativa posterior — el propio avance de tesis lo trata correctamente por separado en "Agentes externos con acceso al SNT" (§2.1.3.1) y en la lista cronológica de normas (Resolución PAMI 1735/2016, Disposición PAMI 1/17, etc.).
- **Cómo arreglar**: reformular la frase para atribuir a la 3683/2011 solo los agentes que define, y a los financiadores el acceso de verificación/auditoría por su normativa propia (como ya hace el avance de tesis). Verificar contra el texto oficial antes de cerrar la redacción final.

---

## B. Errores internos del trabajo escrito

### B1. 🔴 Referencia [15] citada pero inexistente en el paper (congreso)

- **Dónde**: Paper (congreso), §3.3 ("...manteniendo en el ledger compartido únicamente el hash de la información privada para su validación [15]") y §3.4 ("...condición que se sostiene en la arquitectura de canales y colecciones de datos privados de la plataforma [13][15]"). La lista de referencias termina en [14].
- **Por qué importa**: referencia colgante en un paper con formato de congreso; lo primero que salta en una revisión editorial.
- **Cómo arreglar**: agregar la referencia [15] (por el contexto, corresponde a la documentación de Hyperledger Fabric sobre *Private Data*: https://hyperledger-fabric.readthedocs.io/en/release-2.5/private-data/private-data.html, con el mismo formato de las referencias 12–14) o renumerar las citas si la intención era citar una existente.

### B2. 🔴 Anexos A y B del avance de tesis contienen texto placeholder de la plantilla

- **Dónde**: Avance de tesis, Anexo A (p. 32): "No obstante, se presentó una demora en …, motivo por el cual …" — frase con puntos suspensivos sin completar y **sin ningún cronograma** debajo. Anexo B (p. 33): "Procurar que las encuestas sean representativas. Al menos 120 encuestados en una muestra homogénea. La encuesta, claramente, debe favorecer el por qué de la tesis. Si se realiza por Google Forms... el scope *enumerate* o el scope *itemize*." — eso son **instrucciones de la cátedra/plantilla**, no contenido, y no hay resultados de encuesta.
- **Por qué importa**: es contenido de plantilla sin completar en un documento entregable. En la entrega final esto es un rechazo directo.
- **Cómo arreglar**: Anexo A: insertar el cronograma real (Gantt o tabla de tareas con estado) y completar o borrar la frase de la demora. Anexo B: o bien realizar la encuesta y volcar resultados (mínimo 120 respuestas según la propia consigna), o bien eliminar el anexo y toda referencia a él si la encuesta no forma parte del plan; en ese caso revisar que el índice no la liste.

### B3. 🟡 Resolución 435/2011 mal atribuida en la bibliografía del avance de tesis

- **Dónde**: Avance de tesis, texto §2.1.2.1 (p. 5): "La resolución 435/2011 del **Ministerio de salud** argentino establece...". Bibliografía (p. 29): "ADMINISTRACIÓN NACIONAL DE MEDICAMENTOS, ALIMENTOS Y TECNOLOGÍA MÉDICA (**ANMAT**), 2011. *Resolución 435/2011*...".
- **Qué está mal**: la Resolución 435/2011 es del Ministerio de Salud (la dicta el ministerio; la Disposición 3683/2011 sí es de ANMAT). El texto y la bibliografía se contradicen entre sí, y la entrada bibliográfica atribuye la norma al organismo equivocado. Nota: ADR-001 del repo la cita correctamente como "Resolución MS 435/2011".
- **Cómo arreglar**: cambiar el autor institucional de esa entrada bibliográfica a "Ministerio de Salud de la Nación Argentina" (y ajustar la cita en el texto para que el sistema autor-año siga resolviendo). Verificar de paso que la URL apunte a la resolución y no a otra norma.

### B4. 🟡 Conceptos técnicos imprecisos en el marco teórico (avance de tesis, §2.1.5)

Cuatro afirmaciones que un jurado técnico puede objetar. Ninguna afecta el diseño del prototipo, pero conviene corregirlas porque el marco teórico fundamenta la elección de Fabric:

1. **Nonce como elemento general** (§2.1.5, p. 17, y Figura 2.2 p. 16): se afirma que "es fundamental entender que la función hace uso de un elemento arbitrario denominado 'nonce'... generalmente utilizada como método de validación en la decisión de consenso", y la Figura 2.2 ("cadena de bloques aplicada a la trazabilidad de medicamentos") incluye "Nonce" en el encabezado de cada bloque. El nonce es un artefacto de Proof of Work; ni las funciones hash en general ni Hyperledger Fabric (la plataforma elegida, con Raft) usan nonce. **Fix**: acotar la mención del nonce al párrafo de PoW y quitar el campo "Nonce" de la Figura 2.2, o rotular la figura como esquema genérico tipo Bitcoin en lugar de "aplicada a la trazabilidad de medicamentos".
2. **Firma digital como "encriptar con la clave privada"** (§2.1.5.1, p. 18): es el modelo didáctico de RSA; en general (y en ECDSA, que es lo que usa Fabric) firmar no es encriptar. Además, el texto asocia el concepto de *address* al caso de "encriptar con la clave pública", cuando la propia cita de NIST que sigue dice que la dirección se deriva de la **clave pública mediante una función hash** — no tiene relación con encriptar hacia un destinatario. **Fix**: describir la firma como operación específica (se firma con la privada, se verifica con la pública) y corregir el párrafo de direcciones para que derive de hash(clave pública), como dice NIST.
3. **Raft "toma conceptos de Prueba de Autoridad/Identidad"** (§2.1.6.2, Etapa 2, p. 24): Raft es un protocolo de replicación de log tolerante a fallas por caída (CFT); su elección de líder no proviene de PoA ni implica reputación o identidad puesta en juego. **Fix**: eliminar la asociación con PoA y describir Raft como consenso CFT con líder electo por mayoría (quórum), citando la doc de Fabric sobre ordering service (que ya está en la bibliografía).
4. **PoW como defensa anti-DoS** (§2.1.5.4, p. 19): la función principal del costo computacional en PoW es hacer costosa la producción de bloques (resistencia sybil / seguridad del consenso); presentarlo primariamente como mitigación de DoS es débil. **Fix**: reformular en términos de costo de publicación de bloques y resistencia a ataques de identidad múltiple.

Complementario 🔵: la sección de "resolución de conflictos de cadenas / cadena más larga" (§2.1.5.5) es correcta para consenso tipo Nakamoto, pero conviene una frase aclarando que en una red permisionada con ordenamiento determinístico (Fabric/Raft) no se producen forks de ese tipo — refuerza el argumento a favor de Fabric y evita que parezca que aplica al prototipo.

### B5. 🔵 "Nodos pares de anclaje" definidos dos veces con roles distintos (avance de tesis, §2.1.6.1, pp. 22–23)

Primero se listan "dos tipos de nodos pares": **de anclaje** y **de endoso**; inmediatamente después se listan "dos tipos de roles": **líderes** y **anclas** — anclaje/anclas son el mismo concepto (anchor peers) repetido en las dos listas con redacción distinta. **Fix**: unificar en una sola clasificación (p. ej.: todos los peers pueden asumir roles: endorsing peer, leader peer, anchor peer) siguiendo la página "Peers" de la doc de Fabric ya citada.

### B6. 🔵 Fechado "Hyperledger, 2020a–g" para documentación de Fabric release-2.5

- **Dónde**: bibliografía del avance de tesis (pp. 30–31). Las URLs apuntan a `release-2.5`, publicada en 2023 y actualizada después; el año "2020" no corresponde a esas páginas. El paper (congreso) cita las mismas fuentes como "(s. f.)" (sin fecha), lo cual es inconsistente entre los dos documentos además.
- **Cómo arreglar**: usar "(s. f.)" o el año real de la release en ambos documentos, de forma consistente.

### B7. 🔵 Afirmación sobre acceso de auditoría de PAMI presentada como hecho normativo

- **Dónde**: Avance de tesis, §2.1.3.1 "Organismos financiadores": "Para este fin, el instituto posee acceso de auditoría a la base de datos central."
- **Qué pasa**: ADR-005 ("Contexto utilizado") ya detectó que la Resolución PAMI 1735/2016 y la Disposición PAMI 1/17 **no establecen expresamente ese acceso en su texto**. El avance de tesis lo afirma sin matiz.
- **Cómo arreglar**: reformular como inferencia del relevamiento ("accede al SNT como agente externo autorizado para verificar y auditar dispensas", que sí está respaldado por la lista de agentes externos de la sección siguiente) o citar la norma específica que lo establezca, si existe.

---

## C. Inconsistencias y errores dentro del repositorio

### C1. 🔴 `docs/adr/000-template.md` no es una plantilla: es un ADR viejo que contradice a ADR-002

- **Dónde**: `docs/adr/000-template.md` contiene "ADR-000: Topología de canales en la red Hyperledger Fabric", Estado **Aceptado**, fecha 2026-07-25 — una versión antigua de la decisión que hoy es ADR-002. `docs/adr/README.md` lo lista como "Plantilla | Estructura base para nuevos ADRs".
- **Por qué importa**: (a) el índice miente sobre el contenido del archivo; (b) hay **dos ADRs "Aceptados" sobre la misma decisión con contenido divergente**: ADR-000 dice "ANMAT se incluye como miembro de **todas** las colecciones" y no distingue estado público de datos comerciales; ADR-002 (rev. 3) incluye a AnmatMSP "siempre que la información sea necesaria para fiscalización" y hace público el estado mínimo. Cualquier lector (o agente) que tome ADR-000 como vigente implementa la política equivocada.
- **Cómo arreglar**: reemplazar el contenido de `000-template.md` por una plantilla real (esqueleto con las secciones que usan los demás ADRs: Estado/Fecha/Autores, Contexto, Alternativas, Decisión, Justificación, Consecuencias, Contexto utilizado, con placeholders). Si se quiere conservar la historia de la decisión, el historial de git ya la tiene; no debe quedar como archivo "Aceptado" activo.

### C2. 🟠 CI roto por referencia a archivos inexistentes

- **Dónde**: `.github/workflows/chaincode-ci.yml` y `.github/workflows/chaincode-integration.yml` referencian `chaincode/go.mod`, `chaincode/go.sum` y `test/integration/chaincode-e2e.sh`. En el repo, `chaincode/` solo contiene un README con "Hello World!" y no existe el directorio `test/`.
- **Por qué importa**: ambos workflows corren en todo PR a `main` y van a fallar siempre (setup-go no encuentra `go-version-file`, el paso e2e no encuentra el script). Eso bloquea o ensucia cualquier PR.
- **Cómo arreglar**: opción simple y correcta hasta que exista código: agregar a ambos workflows un filtro `paths` (p. ej. `on.pull_request.paths: ['chaincode/**', '.github/workflows/chaincode-*.yml']`) para que solo corran cuando haya chaincode, y crear `test/integration/chaincode-e2e.sh` recién cuando exista el chaincode. Alternativa: crear el esqueleto de módulo Go en `chaincode/` (go.mod + un paquete mínimo compilable). Además, los payloads comentados del workflow de integración usan la interfaz vieja (`RegisterProduct`, `TransferCustody`, `custodian` como argumento posicional): cuando se descomenten, deben reescribirse contra `docs/api-contract.md` v2.0.0 (`RegisterUnit`, `DispatchTransfer` con destino por `transient`, custodio nunca como parámetro).

### C3. 🟡 Ejemplos del contrato de API con GTIN de dígito verificador inválido

- **Dónde**: `docs/api-contract.md` usa en todos los ejemplos el GTIN `07791234567890` (en `MedicationUnitView`, `RegisterUnit`, `DispatchTransfer`, `UnitEventRequest`...). El dígito verificador GS1 correcto para el prefijo `0779123456789` es **8**, no 0 (suma ponderada 3-1 = 132 → check = 8). El propio contrato define `INVALID_REQUEST` para "dígito verificador GTIN" inválido: un chaincode que implemente el contrato **rechazaría sus propios ejemplos**. Lo mismo pasa con el GTIN comentado del workflow de integración (`07795369000001`; el check correcto es 8). En cambio el GLN de ejemplo `7791234500017` sí es válido.
- **Cómo arreglar**: reemplazar el GTIN de ejemplo por uno válido — el más simple es `07791234567898` (mismo prefijo, check correcto) — en todas sus apariciones en `docs/api-contract.md` y en los payloads comentados de `.github/workflows/chaincode-integration.yml`.

### C4. 🟡 Catálogos de errores por operación incompletos en `docs/api-contract.md`

Comparando la sección "Autorización" de cada operación con su lista de "Errores":

- `RejectTransfer`: exige que el invocador sea el destinatario declarado **o** el emisor, pero su lista de errores no incluye `RECEIVER_MISMATCH` (que el catálogo global define explícitamente para "recepción/**rechazo**") ni `UNAUTHORIZED_CUSTODIAN`. No hay código declarado para "invocador no es ni destino ni emisor".
- `Dispense`: exige `snt.role=operator` pero no lista `UNAUTHORIZED_ROLE`; tampoco lista `INVALID_REQUEST`.
- `ReceiveTransfer`: no lista `INVALID_REQUEST`, `ORG_NOT_REGISTERED` ni `ORG_INACTIVE` (el receptor debe estar activo según su propia sección de autorización).
- `DispatchTransfer`: valida registro y `active` del invocador, pero no lista `ORG_NOT_REGISTERED`/`ORG_INACTIVE` (para el destino sí está cubierto por `INVALID_DESTINATION`).

**Cómo arreglar**: completar las listas de errores de esas cuatro operaciones para que toda condición de autorización declarada tenga su código de rechazo. Es un cambio PATCH/MINOR del contrato (no cambia firmas ni semántica de códigos existentes).

### C5. 🟡 Actor `RECOVERY_OR_DISPOSAL_AGENT` sin mapeo de autorización

- **Dónde**: ADR-001 define el actor lógico `RECOVERY_OR_DISPOSAL_AGENT` y lo habilita en T25 (reingreso a stock), T28, T29, T31, T32 y T33 (disposiciones finales). Ni ADR-003 (registro, `agentType`), ni DES-6 (roles/ABAC, políticas de endoso), ni el contrato de API definen **quién es** ese actor en términos de organización, `agentType` o `snt.role`. `docs/api-contract.md` dice "Agente de recupero/disposición" sin resolución.
- **Por qué importa**: cuando se implemente el chaincode (CC-*), no hay regla determinística para autorizar esas seis transiciones. Relacionado: en el estado `DEVUELTO`, ADR-004 deja `CustodioActual` en el emisor, así que tampoco es obvio si "quien verifica la devolución" coincide con el custodio del asset.
- **Cómo arreglar**: decidir y documentar (una línea en DES-6 alcanza, o un ADR corto): la opción más simple y consistente con el resto del diseño es resolver `RECOVERY_OR_DISPOSAL_AGENT` como "el custodio actual registrado del asset (u organización receptora de la devolución) con rol `operator`, o `AnmatMSP` según la transición" — pero tiene que quedar escrito, no implícito.

### C6. 🟡 Nota de dependencia desactualizada en `docs/api-contract.md`

- **Dónde**: tabla "Relación con las decisiones existentes" y "Política de versionado": "**Dependencia de merge**: ADR-004 (DES-9) todavía no está en `develop`; este contrato lo asume decidido" (ídem ADR-005).
- **Qué pasa**: en la rama actual (`integracion-des-5-9-10`) ADR-004 y ADR-005 están integrados y con Estado "Aceptado". La nota quedó vieja y confunde sobre qué está vigente.
- **Cómo arreglar**: eliminar ambas notas de dependencia de merge (o reformular como "ADR-004/ADR-005 integrados en la rama de integración el 2026-08-13/16").

### C7. 🟡 Estados de ADRs incoherentes entre sí

- **Dónde**: `docs/adr/001-maquina-estados-medicamento.md` sigue en Estado **"Propuesto"**, pero ADR-002 (Aceptado), ADR-003 (Aceptado), ADR-004 (Aceptado, que dice "ADR-001... ya fue aceptada por el equipo" en su Justificación — contradiciendo el encabezado de ADR-001) y ADR-005 (Aceptado) dependen de él. `docs/organizations-roles-endorsement.md` (DES-6) también figura "Propuesto" pero ADR-005 y el contrato de API lo usan como base normativa de roles y endoso.
- **Cómo arreglar**: si el equipo ya trabaja sobre ADR-001 y DES-6 como decisiones firmes (todo indica que sí), cambiar sus estados a "Aceptado" y actualizar la tabla de `docs/adr/README.md`. Si no, hay que dejar de referenciarlos como aceptados desde ADR-004.

### C8. 🔵 Resúmenes de la tabla de estados de ADR-001 incompletos respecto de la tabla de transiciones

- **Dónde**: ADR-001, tabla "Estados", columna "Operaciones ordinarias permitidas": para `EN_CUARENTENA` omite reingreso a stock (T26), vencimiento (T13), robo/extravío/deterioro (T14–T16); para `DEVUELTO` omite robo/extravío/deterioro (T14–T16). La tabla de transiciones sí las define.
- **Por qué importa**: menor, pero es el tipo de detalle donde un implementador que lea solo la tabla de estados restringe de más.
- **Cómo arreglar**: completar esas dos celdas, o agregar una nota al pie de la tabla de estados: "columna descriptiva no exhaustiva; la fuente de verdad de transiciones es la tabla de transiciones".

### C9. 🔵 Fence de código suelto al final de `docs/api-contract.md`

Última línea del archivo (línea 357): un ``` ``` `` huérfano después del bloque de versionado. Borrarlo.

### C10. 🔵 Referencia sin definir: "aprobación explícita de B" en `docs/api-contract.md`

El encabezado y la política de versionado exigen "aprobación explícita de **B**" para cambios incompatibles, pero en ningún lugar del repo se define quién o qué es "B" (¿un integrante? ¿el tutor? ¿un rol de la story DES-5?). Definirlo en el propio documento ("B = <nombre/rol>") o reemplazar por el nombre del responsable.

### C11. 🔵 READMEs placeholder "Hello World!"

`README.md` (raíz), `chaincode/README.md`, `baseline/README.md`, `network/README.md`, `benchmarks/README.md` y `client/README.md` contienen solo "Hello World!". Como mínimo el README raíz debería tener: título del PFI, autores, hipótesis en una línea, mapa de directorios (docs/, domain/, chaincode/, baseline/, network/, benchmarks/, client/) y enlace a `docs/README.md`. Los README de componentes pueden decir "pendiente de implementación — ver docs/api-contract.md / measurement-protocol.md" hasta que exista código.

---

## D. Verificaciones que pasaron (sin hallazgos)

Para que quede constancia de lo que se revisó y está congruente:

- **ADR-001 ↔ ADR-004 ↔ api-contract**: los estados, las transiciones T01–T33, el modelo de dos transacciones (despacho/recepción/rechazo) y el mapeo función→transición coinciden. El diagrama Mermaid de ADR-001 coincide 1:1 con la tabla de transiciones (verificado transición por transición).
- **ADR-002 ↔ modelo-datos.md ↔ api-contract**: la lista de campos públicos (GTIN, serie, lote, vencimiento, custodio, estado + timestamp) es idéntica en los tres; `DestinatarioPendiente` consistentemente en PDC y fuera del struct/vista pública; datos comerciales solo por `transient`.
- **ADR-003 ↔ DES-6 ↔ api-contract**: identidad por `cid.GetMSPID()` + registro, custodio persistido como `GLN:`/`CUFE:` canónico, registro operado solo por `AnmatMSP` con `regulatory-admin` — consistente en los tres.
- **DES-3 (`domain/authorized-transfers.json`)**: la tabla del README de `domain/` coincide con los 16 pares del JSON; todas las `normativeReferences` usadas existen en el catálogo; el JSON valida estructuralmente contra su schema (enums, campos requeridos, `allowed` const). El GLN de ejemplo del contrato (`7791234500017`) tiene dígito verificador GS1 válido.
- **measurement-protocol.md ↔ paper**: métricas (latencia con confirmación de commit, throughput exitoso, disponibilidad con fallas Raft 1-de-3 y 2-de-3), baseline relacional con API, entorno single-host con la advertencia de no extrapolar — todo alineado con la estrategia de evaluación del trabajo escrito. La mezcla de carga mixta suma 100%.
- **alcance-prototipo.md**: el tratamiento de fin de envase (reutiliza T06) y la exención de administraciones internas coinciden con lo relevado en el avance de tesis (§2.1.3.1, farmacias/establecimientos asistenciales).
- **Formatos GS1**: la regla del número de serie (≤20 alfanuméricos, no iniciar con "779" si usa los 20) y el vencimiento AAMMDD→ISO 8601 son consistentes entre avance de tesis (§2.1.3.2) y `modelo-datos.md`.
- **Paper (congreso), afirmaciones sobre Fabric/Raft**: líder por canal, quórum por mayoría, tolerancia de 1 caída en cluster de 3, transaction log con transacciones válidas e inválidas — correctas.

---

## E. Hallazgos adicionales (segunda pasada, 2026-08-16)

Segunda pasada enfocada en interacciones entre documentos y en decisiones referenciadas pero no tomadas. Los huecos de diseño que requieren una *decisión nueva* (no una corrección) están desarrollados en `docs/adr-roadmap.md`; acá solo se listan como hallazgo.

### E1. 🟠 Inicio de trazabilidad por droguería: relevado en la tesis, ni modelado ni excluido en el repo

- **Dónde**: Avance de tesis, §2.1.3.1 "Droguerías": "tienen la posibilidad de ser el eslabón de origen de la trazabilidad del medicamento, como los laboratorios. En caso de iniciar la trazabilidad, deben identificar al medicamento con un GLN de la droguería y un número de serie." En el repo, ADR-001 define `T01_REGISTER_UNIT` con actor **exclusivamente `LABORATORY`**, y `docs/alcance-prototipo.md` no menciona esta exclusión.
- **Por qué importa**: es exactamente el tipo de "pisada" que hay que evitar: el marco teórico documenta una capacidad normativa del SNT que el prototipo no soporta, y la restricción no está registrada como decisión consciente de alcance. Un lector cruzando ambos documentos concluye que el prototipo está incompleto sin saber si fue a propósito.
- **Cómo arreglar**: agregar una fila a la tabla de `docs/alcance-prototipo.md`: "Inicio de trazabilidad por droguería | Excluido. Solo `LABORATORY` ejecuta T01 en el prototipo v1 | El caso existe en la normativa relevada pero es minoritario y no altera la evaluación comparativa | Limitación a documentar en la tesis". Alternativa (más trabajo): extender T01 en ADR-001 para admitir `DRUGSTORE` como registrador. Cualquiera de las dos cierra el hueco; lo que no puede quedar es el silencio.

### E2. 🟡 Disposición ANMAT 7439/1999 fundamenta la matriz DES-3 pero no existe en el marco regulatorio de la tesis

- **Dónde**: `domain/authorized-transfers.json` usa `DISP_7439_1999_ART_2_5_7` (habilitación de distribuidoras / definición de operador logístico) como referencia normativa de 5 pares autorizados. La lista cronológica del marco regulatorio del avance de tesis (§2.1.3, p. 6) no incluye la 7439/1999. Lo mismo con el Decreto 1299/1997: la tesis lo lista pero no desarrolla los artículos 2, 4, 5 y 6 que la matriz usa como fundamento principal de casi todos los pares.
- **Por qué importa**: la matriz es la "fuente única de verdad" del prototipo y se apoya en normas que el capítulo normativo de la tesis no releva. En la defensa, cada regla del chaincode debería poder rastrearse al marco teórico.
- **Cómo arreglar**: incorporar al marco regulatorio de la tesis la Disposición 7439/1999 (una entrada en la lista cronológica + un párrafo en §2.1.3.1 sobre distribuidoras/operadores logísticos) y un desarrollo breve de los artículos 2, 4, 5 y 6 del Decreto 1299/1997, que son la base de los flujos comerciales autorizados.

### E3. 🟡 `docs/measurement-protocol.md` no fue actualizado tras ADR-004: "transferencia" sigue siendo una sola operación

- **Dónde**: la tabla de operaciones core mide "Transferencia de custodia" como **un** write, y los perfiles de carga (§6.2, §6.4 con 55% de mix) hablan de "transferencia válida" sin distinguir despacho de recepción. ADR-004 partió la transferencia en **dos transacciones** (`DispatchTransfer` + `ReceiveTransfer`) y exige que la baseline replique el modelo de dos fases.
- **Por qué importa**: sin definición, dos personas pueden implementar el benchmark distinto: ¿una "transferencia" del mix son 2 transacciones? ¿la latencia de transferencia es la del despacho, la de la recepción, o el end-to-end del par? ¿el throughput cuenta pares completados o transacciones individuales? Los resultados no serían comparables ni reproducibles.
- **Cómo arreglar**: agregar una subsección al protocolo (ver `docs/adr-roadmap.md`, decisión D8) que fije: (a) la unidad de medida de "transferencia" (recomendado: reportar latencia de cada transacción por separado **y** el end-to-end del par despacho→recepción); (b) cómo computa el mix de carga mixta (si 55% son pares, son ~2 writes por operación conceptual); (c) que la baseline expone los mismos dos pasos.

### E4. 🟡 DES-6 quedó desactualizado respecto de ADR-005

- **Dónde**: `docs/organizations-roles-endorsement.md` dice que el comportamiento del financiador "queda reservado para DES-10 y CC-8" (Decisión, punto 4) y que "`FinanciadorMSP` no puede invocar operaciones de transferencia, dispensación o eventos extraordinarios **hasta que DES-10/CC-8 definan su comportamiento**" (Reglas de autorización). DES-10 ya se resolvió: es ADR-005 (Aceptado), que define al financiador como verificador de solo lectura post-dispensa.
- **Cómo arreglar**: actualizar esas dos menciones de DES-6 para que referencien ADR-005 como decisión tomada ("conforme ADR-005: solo operaciones de lectura sobre el estado público; nunca escrituras ni custodia") en lugar de dejarlo como reserva futura.

### E5. 🟡 ANMAT y el financiador no tienen representación definida en el modelo de identidad del chaincode

- **Dónde**: el registro organización-establecimiento (ADR-003) solo cataloga establecimientos custodiales (`agentType` ∈ los 6 tipos de DES-3). `AnmatMSP` y `FinanciadorMSP` quedan explícitamente fuera del registro (DES-6: "las organizaciones no custodiales no deben resolverse como custodios"), pero ningún documento define **cómo el chaincode reconoce** que un invocador es ANMAT o un financiador: ¿MSP ID hardcodeado en el chaincode? ¿parámetro de instanciación? ¿una entrada especial del registro con `agentType` regulatorio?
- **Por qué importa**: las operaciones `REGULATORY_ONLY` del contrato (`ProhibitProduct`, `RegisterOrganization`, `SetOrganizationActive`) y el coendoso de ANMAT no pueden implementarse de forma determinística sin esta regla. Hardcodear `"AnmatMSP"` funciona en el prototipo pero contradice el espíritu de ADR-003 ("no acoplar el identificador de dominio al nombre interno de la MSP").
- **Cómo arreglar**: decisión corta a documentar (ver `docs/adr-roadmap.md`, decisión D5). No requiere código todavía, pero sí quedar escrita antes de CC-1.

### E6. 🔵 CUFE tratado como alternativa genérica cuando el relevamiento lo acota

- **Dónde**: ADR-003 y `modelo-datos.md` aceptan `GLN:` o `CUFE:` como identificador canónico de **cualquier** establecimiento. El avance de tesis (§2.1.3.2, "Registro de agentes") indica que el GLN es obligatorio para laboratorio/distribuidora/operador logístico/droguería y que el CUFE aplica a **laboratorios de producción pública** como alternativa.
- **Cómo arreglar**: o bien el chaincode valida que `CUFE` solo sea admisible para `agentType=LABORATORY` (y documentarlo en ADR-003), o bien ADR-003 agrega una nota de simplificación consciente ("el prototipo acepta CUFE para cualquier agentType; la normativa lo acota a producción pública"). Cualquiera de las dos evita la contradicción silenciosa con la tesis.

### E7. 🔵 Endoso de una sola organización en el registro inicial vs. el argumento de integridad del paper

- **Dónde**: Paper (congreso), §3.4: las comprobaciones "son ejecutadas y endosadas por los nodos de las **distintas organizaciones**". DES-6 define para `RegisterUnit` endoso del **laboratorio invocante solamente** (con justificación válida: no existe contraparte previa).
- **Cómo arreglar**: no cambiar el diseño; incorporar a la tesis la excepción y su justificación (la integridad multiorganizacional del alta se ejerce retrospectivamente en la primera transferencia). Es una frase, pero evita que el jurado encuentre la excepción antes que la explicación.
- **Nota adicional del mismo tipo**: `motivo` (texto libre de `UnitEventRequest`) viaja como argumento público del canal; conviene un lineamiento en el contrato ("no incluir datos comerciales ni personales en `motivo`") para no erosionar la clasificación de ADR-002 por la puerta de atrás.

### E8. 🔵 Menudencias de CI y contrato

- `chaincode-ci.yml` usa `actions/checkout@v6` / `setup-go@v6`; `chaincode-integration.yml` usa `@v7`. Unificar versiones para que el dependabot/mantenimiento no diverja.
- `QueryUnitsByGTIN` no define paginación; con el dataset de 50.000 unidades del protocolo de medición, una consulta por un GTIN popular puede devolver miles de resultados en una sola respuesta. Definir paginación (bookmark de Fabric) o documentar el límite como restricción del prototipo.

---

## Priorización sugerida

| Orden | Hallazgos | Esfuerzo |
|---|---|---|
| 1 | B1, B2 (paper/tesis entregables) | Bajo |
| 2 | C1 (ADR-000 falso template), C2 (CI roto) | Bajo |
| 3 | A2 (flujos: decidir fuente de verdad y alinear figura/paper/matriz), E1 (registro por droguería: excluir o modelar) | Medio |
| 4 | A1, A3, A4 (actualizar arquitectura e interpretación en el documento de tesis), E2 (7439/1999 y Dec. 1299/1997 al marco regulatorio) | Medio — es redacción de tesis |
| 5 | C3–C7, E3–E5 (contrato API, protocolo de medición post-ADR-004, DES-6 post-ADR-005, identidad de ANMAT/financiador, actor de recupero, estados de ADRs) | Bajo–Medio |
| 6 | B3–B7, A5, C8–C11, E6–E8 (precisión y prolijidad) | Bajo |

Las decisiones de diseño que faltan tomar (a diferencia de las correcciones de arriba) están planificadas en [`docs/adr-roadmap.md`](adr-roadmap.md).
