# ADR-005: Rol del organismo financiador en la dispensación

- **Estado**: Aceptado
- **Fecha**: 2026-08-13
- **Autores**: Serra, Zarlenga

---

## Contexto

DES-6 introdujo `FinanciadorMSP` como una organización no custodial dentro de la red del prototipo, con un rol reservado (`financier-auditor`) que "no habilita escrituras ni custodia", y difirió explícitamente la definición de su comportamiento concreto a DES-10 y CC-8. Esta ADR resuelve esa reserva: define qué hace el organismo financiador (PAMI, obras sociales) en el prototipo.

El marco normativo y el relevamiento del proyecto delimitan el rol del financiador:

- El paper (§3.2, Fig. 3) representa la dispensación al paciente "y su validación por parte de un organismo financiador".
- El Instituto Nacional de Servicios Sociales para Jubilados y Pensionados (PAMI) condiciona la cobertura financiera al cumplimiento del SNT: efectúa una validación informática de la trazabilidad de los productos e impone la correcta trazabilidad como condición excluyente para la liberación de pagos a los laboratorios, para lo cual posee acceso de auditoría a la base de datos central (Resolución PAMI 1735/2016, Disposición PAMI 1/17).
- El financiador integra el conjunto de agentes externos con acceso al SNT que obtienen usuario para verificar y auditar las dispensas realizadas a sus beneficiarios, sin ser custodios ni participar de la circulación física del medicamento.
- La Ley 25.326 de Protección de Datos Personales excluye del ledger los datos personales del afiliado, en línea con la restricción que ya fija `modelo-datos.md` (§4) y con el alcance de dispensación de CC-4.

El problema de diseño es que estos requisitos están en tensión: el financiador debe poder validar la trazabilidad para liberar un pago, pero no debe ser custodio, no debe acceder a la información comercial de operaciones ajenas (ADR-002) y no debe incorporar datos personales del afiliado al ledger.

## Alternativas

**A. Validación previa: el financiador endosa o autoriza la dispensación**

- El financiador participa como coendosante o autorizador de la transacción de dispensación, de modo que la dispensa no se confirma sin su intervención.
- No representa el proceso real: la validación de trazabilidad del financiador es una condición para **liberar el pago**, que ocurre con posterioridad a la dispensación física al paciente, no como precondición para entregarla.
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
2. La "validación por parte del organismo financiador" se materializa como una **consulta de lectura** sobre el estado público de trazabilidad y el historial (`GetHistoryForKey`) de la unidad, que confirma que la unidad existe, recorrió la cadena autorizada y fue dispensada por un agente habilitado y en estado `DISPENSADO`.
3. La verificación es **dirigida por reclamo (claim-driven)**: el financiador consulta la traza de los seriales específicos que se le presentan en la facturación off-ledger, no enumera ni recorre la totalidad de las dispensaciones del canal.
4. **El pago es off-ledger** y no se representa como transacción del prototipo.
5. **No se persiste ningún dato personal del afiliado en el ledger**, ni siquiera obra social o número de afiliado. El vínculo afiliado ↔ unidad dispensada vive en el sistema de facturación externo; el financiador parte de ese vínculo off-ledger y usa el ledger solo para confirmar la legitimidad de la traza del serial.

## Alcance de lectura y relación con ADR-002

El financiador es una organización miembro del canal con acceso de lectura al **estado público de trazabilidad** definido por ADR-002 (identificador, lote, vencimiento, custodio actual, estado). Ese estado público no contiene información comercial, por lo que la lectura del financiador nunca expone precios, condiciones ni documentos de operaciones ajenas.

El financiador **no es miembro de las Private Data Collections** de operaciones en las que no participa. No accede a la información comercial ni documental (factura, remito, cantidades) de las transferencias entre otros establecimientos, conforme a ADR-002.

Impacto para NET-5: la configuración de red debe otorgar al financiador lectura del estado público del canal sin incorporarlo como miembro de las colecciones privadas comerciales. No se define en esta ADR ninguna colección privada específica del financiador; si un requerimiento futuro la justificara, sería una decisión aparte fuera de este alcance.

## Datos del afiliado y Ley 25.326

El ledger no almacena datos personales del afiliado. La verificación claim-driven permite que el financiador confirme la legitimidad de una dispensa **sin** que el número de afiliado ni la obra social se registren en cadena:

- El sistema de facturación externo mantiene el vínculo "serial X fue dispensado al afiliado Y de la cobertura Z".
- El financiador toma el serial del reclamo y consulta el estado público y el historial de esa unidad en el ledger.
- Si la traza es legítima (producción, cadena autorizada, dispensación por agente habilitado), la condición de trazabilidad para liberar el pago queda satisfecha.

Esto mantiene la coherencia con `modelo-datos.md` (§4), que ya excluye del activo la identificación individual del afiliado más allá de lo necesario para la cobertura, y con el alcance de dispensación de CC-4. La decisión de mantener el número de afiliado fuera incluso de una colección privada es deliberada: el prototipo no necesita el vínculo on-ledger para representar la verificación de trazabilidad, y evitarlo elimina el dato sensible del alcance del ledger por completo.

## Consecuencias

- **Para DES-6**: confirma el rol `financier-auditor` como de solo lectura. El financiador no invoca transferencias, dispensación ni eventos extraordinarios, tal como DES-6 anticipaba.
- **Para CC-8**: la implementación del comportamiento del financiador se reduce a operaciones de consulta sobre el estado público y el historial. No introduce funciones que muten estado. Las firmas concretas quedan para DES-5/CC-8.
- **Para ADR-002 / NET-5**: el financiador obtiene lectura del estado público del canal, sin membresía en colecciones privadas comerciales ajenas.
- **Para CC-4 / modelo-datos.md**: no se agrega ningún campo al struct `MedicationUnit` ni se incorpora dato de afiliado al ledger. La dispensación no cambia su modelo por esta ADR.
- **Para la baseline**: la línea base centralizada debe replicar la verificación de trazabilidad de solo lectura del financiador para preservar la paridad funcional de la comparación (DES-7).
- **Se gana**: fidelidad al proceso real (validación posterior para liberar pago), footprint on-ledger mínimo del financiador, cero dato personal del afiliado en cadena, sin dependencia de disponibilidad del financiador para operar la dispensación.
- **Se pierde / costo**: el prototipo no representa el flujo de pago ni la conciliación económica; la verificación depende de un vínculo afiliado↔unidad que permanece fuera del ledger y, por lo tanto, fuera de las garantías de integridad de la cadena.
- **Queda pendiente**: si una iteración futura quisiera representar on-ledger la relación entre una dispensa y una cobertura (por ejemplo, para auditar volúmenes por financiador sin exponer al afiliado), debería evaluarse una colección privada específica bajo una ADR propia; queda fuera del alcance del prototipo v1.

## Contexto utilizado

- Issue GitHub #58: DES-10 · Rol del organismo financiador en la dispensación, consultada el 2026-08-13.
- [DES-6: Organizaciones, MSP, roles, ABAC y políticas de endoso](../organizations-roles-endorsement.md): define `FinanciadorMSP` como organización no custodial y reserva su comportamiento a DES-10/CC-8.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): la dispensación (T06) no involucra al financiador.
- [ADR-002: Topología de canales](002-topologia-canales.md): estado público de trazabilidad legible por miembros del canal; información comercial restringida a PDC.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): modelo de organización por establecimiento sobre el que se apoya la membresía no custodial del financiador.
- [modelo-datos.md](../modelo-datos.md), §4: exclusión de datos personales y de identificación del afiliado del activo de trazabilidad.
- Paper del proyecto, §3.2 y Fig. 3: dispensación al paciente y su validación por parte de un organismo financiador.
- Avance de tesis, secciones "Organismos Financiadores" y "Agentes externos con acceso al SNT": PAMI valida la trazabilidad como condición excluyente para liberar pagos y posee acceso de auditoría; los financiadores acceden al SNT para verificar y auditar dispensas a sus beneficiarios.
- Resolución PAMI 1735/2016 y Disposición PAMI 1/17: condicionamiento de la cobertura y del pago al cumplimiento del SNT.
- [Ley 25.326 de Protección de Datos Personales](https://servicios.infoleg.gob.ar/infolegInternet/anexos/60000-64999/64790/texact.htm), art. 2: exclusión de datos personales y sensibles del afiliado.
