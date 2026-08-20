# ADR-011: Criterios de verificación de traza para el organismo financiador

- **Estado**: Propuesto
- **Fecha**: 2026-08-17
- **Autores**: Serra, Zarlenga

---

## Contexto

ADR-005 modeló al organismo financiador como una organización no custodial de solo lectura cuya "validación de trazabilidad" es una consulta claim-driven sobre el estado público y el historial de la unidad dispensada, como condición para liberar el pago (off-ledger). Pero ADR-005 dejó deliberadamente abierta la semántica de esa consulta (Decisión, punto 2): fijó qué debe confirmar como mínimo (existencia de la unidad, estado `DISPENSADO`, agente dispensador habilitado) y declaró explícitamente que **no** define qué constituye una "traza irregular" más allá de eso — por ejemplo, si una transferencia fue autorizada según la matriz de DES-3 vigente al momento de esa transferencia, si algún actor estuvo habilitado en el momento del evento y no solo actualmente, o si falta algún evento esperado en la secuencia. "Definir esa semántica verificable en detalle es un prerequisito para CC-8".

Esta ADR resuelve exactamente esa reserva: es la decisión D6 de `docs/adr-roadmap.md` (issue #86, DES-17) y el prerequisito declarado de CC-8 (#62). Sin ella, CC-8 no tiene criterio de aceptación, BASE-3 (#39) no sabe qué replicar, y la afirmación del trabajo escrito — "validación informática del trazado de los productos listados... condición excluyente para la liberación de pagos" — queda sin materialización verificable.

Las piezas sobre las que se decide ya están fijadas:

- **ADR-001** define la máquina de estados versionada (`1.0.0`): la tabla de transiciones es la fuente de verdad de qué secuencias de estados son posibles, y `DISPENSADO` es terminal.
- **ADR-003** define el registro organización-establecimiento (identidad GLN/CUFE, `agentType`, `active` actual) como única fuente de resolución de identidad, y el patrón "Límites de garantía" que esta ADR replica.
- **ADR-005** fija el modo de acceso: estado público del canal e historial vía `GetHistoryForKey` (solo modificaciones confirmadas), verificación claim-driven por serial, vínculo afiliado↔unidad off-ledger.
- **ADR-008** decidió la distribución de la matriz de transferencias (`go:embed`, paquete compartido chaincode/baseline), la persistencia de `ruleId` + versión por despacho, y la declaración de **matriz única durante toda la evaluación de v1** — respuesta directa a la pregunta de ADR-005 sobre la "matriz vigente al momento de la transferencia".
- **`docs/api-contract.md`** (DES-5, congelado 2.0.x) ya expone `ReadUnit` y `GetUnitHistory` como las operaciones de lectura del flujo del financiador, con su catálogo de errores estable.

El problema de diseño es elegir, dentro de lo que el estado público y el historial permiten verificar determinísticamente, qué comprobaciones componen "traza legítima" y qué devuelve la consulta: un veredicto evaluable o un historial crudo para que el financiador juzgue por su cuenta.

## Alternativas

**A. Historial crudo: el financiador juzga off-ledger**

- La consulta del financiador se limita a `GetUnitHistory`; cada financiador implementa sus propios criterios de legitimidad sobre el historial devuelto.
- Minimiza el chaincode: no agrega lógica de verificación, solo lectura ya existente.
- Traslada la lógica normativa fuera del prototipo: la checklist de "traza legítima" dejaría de ser una regla determinística del sistema para ser una interpretación de cada consumidor, exactamente lo contrario del principio que el trabajo escrito defiende (la validación normativa se traslada al chaincode).
- Rompe la paridad de la comparación: la baseline (BASE-3) no tendría un comportamiento definido que replicar, y la "validación informática del trazado" del trabajo escrito quedaría sin criterio verificable en ninguna de las dos implementaciones.
- Se descarta **como única salida**. `GetUnitHistory` sigue disponible como operación general de lectura del contrato (no se elimina nada de DES-5); lo que se descarta es que sea la única respuesta del prototipo a la verificación del financiador.

**B. Veredicto estructurado con checklist determinística embebida**

- El prototipo define una checklist cerrada de comprobaciones, evaluadas por la propia consulta sobre el estado público y el historial, y devuelve un veredicto estructurado: legítima o no, qué comprobación falló y por qué.
- La semántica de "traza legítima" queda fijada por diseño, es idéntica para cualquier financiador, es replicable línea por línea por la baseline, y le da al financiador lo que su proceso real necesita: una condición de pago evaluable, no un dump para interpretar.
- Costo: la checklist solo puede verificar lo que el ledger y el registro efectivamente guardan; sus puntos ciegos deben declararse como límites explícitos, no silenciarse.
- Se adopta.

**C. Veredicto estructurado con validación de habilitación histórica**

- Igual que B, pero verificando además que cada actor del historial estuviera `active` **en el momento de su evento**, no solo actualmente.
- Es la verificación más fiel a una auditoría regulatoria completa.
- El registro organización-establecimiento de ADR-003 persiste el estado `active` **actual**; no versiona habilitaciones. Validar habilitación histórica exigiría versionar el registro (o reconstruir su historia vía `GetHistoryForKey` sobre sus claves, con semántica de correlación temporal entre historiales que ningún documento define), un rediseño que el prototipo v1 no necesita para su hipótesis.
- Se descarta en v1 porque requiere versionado del registro; la limitación resultante se documenta como límite explícito de la verificación (ver "Límites de la verificación").

## Decisión

Se adopta la **alternativa B**: la verificación claim-driven de ADR-005 se materializa como una **checklist determinística de cinco comprobaciones**, evaluadas en orden sobre el estado público y el historial (`GetHistoryForKey`) de la unidad identificada por GTIN + número de serie.

### Checklist de verificación

1. **Existencia.** La unidad (GTIN+serie) existe en el world state. Si no existe, el veredicto es `NO_ENCONTRADA`.
2. **Estado dispensado.** El estado actual de la unidad es `DISPENSADO` (estado terminal de ADR-001). Si no lo es, el veredicto es `NO_DISPENSADA`, incluyendo en el detalle el estado actual observado — la unidad puede ser legítima y simplemente no haber sido dispensada aún, o estar en un estado bloqueante o terminal distinto; el financiador no tiene condición de pago en ninguno de esos casos.
3. **Dispensador habilitado.** El custodio registrado en la entrada del historial que produjo el estado `DISPENSADO` tiene, en el registro organización-establecimiento (ADR-003), `agentType` ∈ {`PHARMACY`, `HEALTHCARE_FACILITY`} — los únicos tipos habilitados para T06 según ADR-001/DES-6. Si no, el veredicto es `DISPENSADOR_INVALIDO`.
4. **Camino de estados válido.** La secuencia completa de estados del historial corresponde a un camino válido de la máquina de estados de ADR-001: cada par consecutivo de estados observados es una transición declarada en la tabla de ADR-001 (y el primer estado es `EN_LABORATORIO`, único estado inicial de la máquina). La comprobación es recomputable determinísticamente contra la tabla de transiciones embebida en el binario — mismo criterio de distribución que ADR-008 fija para la matriz. Si algún par consecutivo no es una transición declarada, el veredicto es `SECUENCIA_INVALIDA`.
5. **Pares de transferencia autorizados.** Cada cambio de `CustodioActual` observado en el historial corresponde a un par (`agentType` origen → `agentType` destino) autorizado por la matriz embebida de transferencias (ADR-008), resolviendo el `agentType` de cada custodio mediante el registro organización-establecimiento actual. Si algún par no está autorizado, el veredicto es `TRANSFERENCIA_NO_AUTORIZADA`, incluyendo en el detalle el par observado.

### Salida: veredicto estructurado

La consulta devuelve un **veredicto estructurado**, no un historial crudo:

```json
{
  "legitima": false,
  "verificaciones": [ { "check": "...", "resultado": "...", "detalle": "..." } ],
  "motivo": "SECUENCIA_INVALIDA"
}
```

- `legitima` es `true` si y solo si las cinco comprobaciones pasan; en ese caso `motivo` queda vacío.
- `verificaciones` reporta cada comprobación con su resultado y detalle; `motivo` es el veredicto nombrado de la primera comprobación que falla, en el orden declarado.
- La racionalidad de esta forma: el financiador necesita una **condición de pago evaluable** ("¿la traza es legítima sí o no, y si no, por qué?"), y el veredicto estructurado es lo que la baseline puede replicar comprobación por comprobación (BASE-3), preservando la paridad que exige el protocolo de medición. Un historial crudo no es ni lo uno ni lo otro.

### Naturaleza de la operación y alcance de la firma

- La operación es de **solo lectura**: no muta estado ni genera endoso de escritura, conforme a ADR-005 y al rol `financier-auditor` de DES-6.
- La **firma concreta** ya está definida en el contrato DES-5 (`api-contract.md`): `VerifyTrace(ctx, gtin, numeroSerie) (*TraceVerdict, error)`, incorporada como agregado MINOR en la versión 2.1.0. Esta ADR fija la **semántica** — las cinco comprobaciones, su orden, sus veredictos nombrados y la forma del resultado —; el contrato fija la firma. CC-8 (#62) **implementa** ambas, no las diseña.
- **Autorización de la consulta**: `agentType=FINANCIER` con `snt.role=financier-auditor`, o `agentType=REGULATOR` con rol de auditoría (ADR-010). Un invocador registrado y activo cuyo `agentType` no sea ninguno de los dos recibe `UNAUTHORIZED_AGENT_TYPE`: no es un veredicto de traza sino un rechazo de autorización, y por eso no forma parte de los valores de `motivo`.

## Justificación

- **La checklist cubre exactamente lo que ADR-005 dejó abierto, con lo que las decisiones posteriores ya resolvieron.** Las tres preguntas del punto 2 de la Decisión de ADR-005 quedan respondidas: la validación de pares contra la matriz "vigente en su momento" se resuelve con la matriz única embebida de ADR-008 (comprobación 5, con su límite heredado declarado); la habilitación histórica de actores se descarta en v1 y se declara límite (alternativa C); la integridad de la secuencia de eventos se verifica recomputando el camino contra la máquina de ADR-001 (comprobación 4).
- **Cada comprobación es determinística y recomputable.** Las cinco operan sobre datos confirmados (world state, `GetHistoryForKey`, registro organización-establecimiento) y reglas versionadas embebidas (tabla de ADR-001, matriz de ADR-008). Dos evaluaciones sobre el mismo historial y el mismo registro producen el mismo veredicto, en el chaincode y en la baseline.
- **El orden de evaluación es el del costo y la precedencia lógica.** Existencia y estado actual se responden con el estado público (`ReadUnit`); recién si la unidad está dispensada tiene sentido auditar el historial completo. Un veredicto temprano (`NO_ENCONTRADA`, `NO_DISPENSADA`) es además la señal correcta para el proceso de pago: no hay nada que auditar todavía.
- **El veredicto estructurado materializa la frase del trabajo escrito.** "Validación informática del trazado... condición excluyente para la liberación de pagos" exige una salida evaluable como condición; la alternativa A la degradaba a interpretación de cada consumidor.
- **Los límites se declaran en lugar de simularse resueltos.** El diseño hereda puntos ciegos de decisiones previas (registro sin historia de habilitación, matriz única, historial sin transacciones rechazadas, vínculo afiliado↔unidad off-ledger); el mismo criterio de "Límites de garantía" de ADR-003 obliga a enumerarlos para que la tesis pueda citarlos como limitaciones conscientes y no como omisiones.

## Límites de la verificación

Qué **no** detecta la verificación, en el estilo de los "Límites de garantía" de ADR-003. Estos límites deben acompañar a la operación en su documentación de contrato y en la tesis:

- **Habilitación histórica de actores.** El registro organización-establecimiento persiste el estado `active` **actual** (ADR-003); la verificación no valida que cada actor del historial estuviera activo en el momento de su evento, solo resuelve identidades y `agentType` contra el registro vigente. Es una limitación heredada del diseño del registro; revisarla exigiría versionar el registro (ver alternativa C y "Queda pendiente").
- **Versión histórica de la matriz.** v1 opera con una única versión de matriz durante toda la evaluación (ADR-008, Decisión, punto 4); la comprobación 5 re-evalúa los pares del historial contra la matriz embebida vigente. Si la matriz cambiara entre eventos, la re-evaluación usaría la versión vigente, no la que autorizó originalmente cada despacho — límite ya declarado por ADR-008 y heredado por esta ADR.
- **Transacciones inválidas o rechazadas.** `GetHistoryForKey` devuelve únicamente las modificaciones confirmadas de la clave; los intentos rechazados (por endoso, validación o estado) nunca llegan al world state y no aparecen en el historial (ya aclarado por ADR-005). La verificación audita lo que ocurrió, no lo que se intentó.
- **Vínculo afiliado↔unidad.** Vive off-ledger por decisión de ADR-005 (Ley 25.326): la verificación no puede comprobar que el serial reclamado pertenezca efectivamente a un afiliado del financiador invocante. Es el supuesto de confianza documentado por ADR-005 ("Alcance de lectura y relación con ADR-002"), que esta ADR no modifica.
- **Autenticidad física del producto.** La verificación acredita la traza registrada, no la posesión física efectiva, la autenticidad material del envase ni la ausencia de clonación del código serializado — límites heredados textualmente de los "Límites de garantía" de ADR-003.

## Consecuencias

- **Para CC-8 (#62)**: la story queda implementable contra una firma ya congelada y con criterio de aceptación concreto — las cinco comprobaciones en su orden, los veredictos nombrados (`NO_ENCONTRADA`, `NO_DISPENSADA`, `DISPENSADOR_INVALIDO`, `SECUENCIA_INVALIDA`, `TRANSFERENCIA_NO_AUTORIZADA`) y la forma del veredicto estructurado. No le queda ninguna decisión de diseño de contrato. Los tests de "traza válida / traza irregular" que la issue exige se derivan directamente de cada comprobación.
- **Para DES-5 (`api-contract.md`)**: la operación ya está incorporada al contrato (versión 2.1.0, con `UNAUTHORIZED_AGENT_TYPE` agregado en 2.2.0). Cualquier cambio posterior de su semántica exige revisar esta ADR y el contrato en el mismo PR.
- **Para BASE-3 (#39) y la baseline**: la baseline replica el mismo veredicto, comprobación por comprobación, consumiendo la misma tabla de ADR-001 y el mismo paquete de matriz de ADR-008, para preservar la paridad funcional de la comparación (DES-7).
- **Para ANMAT (rol `auditor`)**: la misma checklist es utilizable como herramienta de auditoría regulatoria sobre cualquier unidad — la semántica de "traza legítima" no es exclusiva del financiador, aunque su caso de uso (condición de pago) sea el que la motivó.
- **Se gana**: semántica de "traza legítima" cerrada, determinística y replicable; CC-8 y BASE-3 desbloqueadas con criterio de aceptación verificable; la afirmación del trabajo escrito sobre la validación informática del trazado queda materializada con límites honestos.
- **Se pierde / costo**: la verificación solo ve lo que el ledger y el registro guardan — sin habilitación histórica, sin versiones históricas de matriz, sin intentos rechazados, sin vínculo afiliado↔unidad (ver "Límites de la verificación"); el veredicto agrega lógica de verificación al chaincode y a la baseline que debe mantenerse en paridad.
- **Queda pendiente**: el versionado del registro organización-establecimiento, si una revisión futura quisiera validar habilitación histórica de actores (reabriría la alternativa C).

## Divergencia con el trabajo escrito

No hay divergencia. La checklist deriva de las validaciones que el propio trabajo escrito enumera para los procesos del prototipo — custodia (comprobaciones 3 y 5), unicidad e identificación unívoca de la unidad (comprobación 1), estado del producto (comprobaciones 2 y 4) y flujo autorizado (comprobación 5) — y materializa la "validación informática del trazado... condición excluyente para la liberación de pagos" como veredicto evaluable. Esta ADR no promete detectar lo que el diseño no ve: los puntos ciegos (habilitación histórica, versiones de matriz, intentos rechazados, vínculo afiliado↔unidad, autenticidad física) quedan declarados en "Límites de la verificación" para que la tesis los liste como limitaciones conscientes.

## Contexto utilizado

- Issue GitHub #86: DES-17 · ADR-011: Criterios de verificación de traza del financiador (prerequisito de CC-8), consultada el 2026-08-17.
- Issue GitHub #62: CC-8 · Verificación de trazabilidad por el financiador, consultada el 2026-08-17.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): tabla de transiciones y estados terminales — la comprobación 4 recomputa caminos válidos contra esta tabla; T06 y sus actores habilitados fundamentan la comprobación 3.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): registro organización-establecimiento (`agentType`, `active` actual) usado para resolver identidades en las comprobaciones 3 y 5; patrón "Límites de garantía" replicado en esta ADR.
- [ADR-005: Rol del organismo financiador en la dispensación](005-rol-organismo-financiador.md): preguntas abiertas del punto 2 de su Decisión que esta ADR responde; verificación claim-driven, semántica de `GetHistoryForKey` y supuesto de confianza afiliado↔unidad.
- [ADR-008: Distribución y versionado de la matriz de transferencias en chaincode y baseline](008-transfer-matrix-distribution.md): matriz embebida y paquete compartido usados por la comprobación 5; declaración de matriz única en v1 y su límite heredado.
- [`docs/api-contract.md`](../api-contract.md): `ReadUnit`, `GetUnitHistory`, catálogo de errores y política de versionado (el agregado de la firma es MINOR).
- [`docs/adr-roadmap.md`](../adr-roadmap.md): decisión D6, alcance y riesgos que esta ADR resuelve.
