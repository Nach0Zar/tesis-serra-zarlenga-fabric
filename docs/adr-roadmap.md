# Plan de decisiones de arquitectura pendientes (ADR roadmap)

- **Fecha**: 2026-08-16 (actualizado el 2026-08-17)
- **Insumo**: revisión de congruencia completa del proyecto ([`consistency-review.md`](consistency-review.md)), los 5 ADRs vigentes al momento del relevamiento, DES-2/3/5/6/7 y el trabajo escrito.

> **Estado: plan ejecutado.** Las siete decisiones D1–D7 quedaron registradas como ADR-006 a ADR-012 (estado *Propuesto*, pendientes de aprobación del equipo) y las dos decisiones documentales D8–D9 se aplicaron sobre `measurement-protocol.md` y `alcance-prototipo.md`. Este documento se conserva como registro del razonamiento que originó cada decisión y como checklist de sincronización con el trabajo escrito.

Este documento lista las decisiones de diseño que **todavía no están tomadas** y que son necesarias para que el prototipo pueda implementarse y evaluarse sin improvisar, y para que la tesis final pueda defenderse sin huecos. No son correcciones (esas están en `consistency-review.md`): son decisiones nuevas, cada una referenciada como pendiente por algún documento ya aceptado.

## Principio rector: el repo manda, pero no pisa al trabajo escrito

El repositorio está más actualizado que el trabajo escrito, y esa brecha va a seguir creciendo. Para que no se convierta en contradicción:

1. **Toda nueva ADR que se aparte de algo afirmado en el trabajo escrito debe incluir una sección "Divergencia con el trabajo escrito"** que cite la afirmación original, explique por qué el diseño evolucionó y qué debe actualizarse en la próxima iteración del documento. ADR-002 ("Advertencia sobre esta interpretación") y ADR-005 ("Contexto", puntos sobre la figura del paper) ya hacen esto; debe ser regla, no excepción.
2. **Ninguna ADR debe redefinir conceptos que el trabajo escrito ya fijó bien** (definiciones de trazabilidad, marco regulatorio, estándares GS1, fundamentos de Fabric): los ADRs los referencian, no los reescriben. Si un ADR necesita corregir un concepto del marco teórico, eso es un hallazgo para `consistency-review.md`, no una redefinición local.
3. **Cada afirmación arquitectónica del trabajo escrito debe terminar mapeada a un ADR** que la implemente o que documente la divergencia. La tabla final de este documento sirve de checklist para la redacción del capítulo de diseño de la tesis.

## Mapa de referencia: stories ↔ artefactos

Para que cualquier agente ubique rápido qué existe y qué falta:

| Story | Artefacto | Estado |
|---|---|---|
| DES-1 | `adr/001-maquina-estados-medicamento.md` | Existe (Aceptado) |
| DES-2 | `modelo-datos.md` | Existe |
| DES-3 | `domain/authorized-transfers.json` + schema | Existe |
| DES-4 | `adr/002-topologia-canales.md` | Existe |
| DES-5 | `api-contract.md` | Existe (congelado 2.7.1) |
| DES-6 | `organizations-roles-endorsement.md` | Existe (Aceptado, actualizado post-ADR-005) |
| DES-7 | `measurement-protocol.md` | Existe (actualizado post-ADR-004, §3.4) |
| DES-8 | `adr/003-establishment-identity-gln-cufe.md` | Existe |
| DES-9 | `adr/004-transfer-dispatch-reception.md` | Existe |
| DES-10 | `adr/005-rol-organismo-financiador.md` | Existe |
| DES-11 | Validación contra Vademécum/REM | Resuelto como exclusión en `alcance-prototipo.md` |
| DES-12 | ADR-006: Diseño de colecciones privadas | **Resuelta** → [`adr/006-private-data-collections.md`](adr/006-private-data-collections.md) (issue #81) |
| DES-13 | ADR-007: Topología física de la red | **Resuelta** → [`adr/007-network-topology.md`](adr/007-network-topology.md) (issue #82) |
| DES-14 | ADR-008: Matriz DES-3 en chaincode/baseline | **Resuelta** → [`adr/008-transfer-matrix-distribution.md`](adr/008-transfer-matrix-distribution.md) (issue #83) |
| DES-15 | ADR-009: Devolución y custodia en DEVUELTO | **Resuelta** → [`adr/009-return-and-recovery-semantics.md`](adr/009-return-and-recovery-semantics.md) (issue #84; desbloquea EXT-4 #30) |
| DES-16 | ADR-010: Identidad de no custodiales | **Resuelta** → [`adr/010-non-custodial-identity.md`](adr/010-non-custodial-identity.md) (issue #85) |
| DES-17 | ADR-011: Verificación de traza del financiador | **Resuelta** → [`adr/011-financier-trace-verification.md`](adr/011-financier-trace-verification.md) (issue #86; desbloquea CC-8 #62) |
| DES-18 | ADR-012: Diseño de la baseline | **Resuelta** → [`adr/012-baseline-design.md`](adr/012-baseline-design.md) (issue #87) |
| NET-5 (#24), NET-2 (#21), EXT-4 (#30), CC-8 (#62) | Issues de implementación | Actualizadas el 2026-08-16 con sus dependencias de decisión |
| — | D8 (medición bifásica) y D9 (exclusiones de alcance) | **Resueltas el 2026-08-16** en `measurement-protocol.md` §3.4 y `alcance-prototipo.md` |

---

## Decisiones pendientes

Numeradas D1–D9 en orden de dependencia. Los códigos ADR-006+ son sugerencia; lo que importa es el contenido y el orden.

### D1 · ADR-006 — Diseño de colecciones privadas (resuelve NET-5)

- **Qué decide**: el mecanismo concreto de PDC para la información comercial/documental y el `DestinatarioPendiente`: (a) colecciones implícitas por organización (`_implicit_org_<MSPID>`), escribiendo el dato en la implícita del emisor, la del receptor y la de ANMAT; (b) colecciones explícitas generadas programáticamente desde el registro organización-establecimiento; o (c) datos transitorios con solo hash persistido. Define también: nombres/convención de colecciones, qué queda como evidencia hash en el ledger común, políticas `memberOnlyRead`/`memberOnlyWrite`, y cómo se actualiza la membresía cuando ADR-003 incorpora una organización nueva.
- **Por qué es necesaria**: ADR-002 la delega explícitamente ("La granularidad exacta de colecciones debe definirse en NET-5"); ADR-004 depende de ella para `DestinatarioPendiente` (la validación de `ReceiveTransfer` lee la PDC); el contrato de API ya fija el transporte (`transient` claves `commercial` y `destinatario`) pero no dónde aterriza. **Sin esta decisión no se puede implementar `DispatchTransfer`/`ReceiveTransfer`.**
- **Restricción clave a evaluar**: la membresía "emisor + receptor + AnmatMSP" es *por operación*, pero las colecciones explícitas de Fabric se definen *por chaincode* en el momento del deploy. Con una organización por establecimiento (ADR-003), las colecciones explícitas por par crecen combinatoriamente. Eso empuja fuerte hacia colecciones implícitas por organización (opción a), que es la recomendación a validar.
- **Riesgo de pisar el trabajo escrito**: bajo. El trabajo escrito afirma "hash de la información privada en el ledger compartido" — cualquier opción lo cumple; documentar cuál y cómo.
- **Desbloquea**: CC-3 (transferencias), NET-5, la demo de confidencialidad ("una organización no autorizada no ve el dato comercial") que ADR-002 exige.

### D2 · ADR-007 — Topología física de la red del prototipo (resuelve NET-2/NET-4/NET-6)

- **Qué decide**: cuántos peers por organización; cuántos orderers Raft y **qué organizaciones los aportan**; una CA vs. varias; nombre del canal y del chaincode (hoy solo existen en el workflow de CI: `mychannel`/`snt` — decidir si son los definitivos); proceso de bootstrap del dataset mínimo de organizaciones (las 7 MSP de ejemplo de DES-6 + su alta en el registro); y cómo se materializan las políticas de endoso de DES-6 (endorsement policy de chaincode vs. state-based endorsement vs. validación en chaincode).
- **Por qué es necesaria**: es la última pieza entre el diseño y cualquier `configtx.yaml`. DES-6 la delega ("NET-2, NET-5 y NET-6 deben decidir cómo expresarla"), ADR-003 deja pendiente la coordinación alta-en-canal/alta-en-registro, y el protocolo de medición (§8.1) ya asume un cluster Raft de 3 con caída de 1 y de 2 — alguien tiene que decidir dónde viven esos 3 orderers.
- **Riesgo de pisar el trabajo escrito**: **alto, y es el punto más delicado del plan.** El paper afirma que el esquema "habilita que distintas organizaciones contribuyan nodos al servicio de ordenamiento, reduciendo la dependencia de un único administrador central", y la eliminación del punto único de falla es uno de los tres argumentos de la hipótesis. Si el prototipo levanta los 3 orderers bajo una única organización de ordenamiento (lo que hace `test-network` por defecto), la prueba de disponibilidad Raft sigue siendo válida como CFT, pero la afirmación de descentralización administrativa queda sin respaldo experimental. La ADR debe elegir: (a) repartir los orderers entre AnmatMSP y ≥2 organizaciones más (fiel al paper, más trabajo de red), o (b) un solo proveedor de ordenamiento con una sección "Divergencia con el trabajo escrito" que lo declare simplificación del prototipo y acote la conclusión correspondiente en la tesis. **Actualización posterior**: ADR-007 eligió (a) y resolvió además el límite de certificación con un servidor compartido que hospeda una CA lógica y raíz propia por organización; la simplificación declarada es el operador común, no una raíz de confianza común.
- **Desbloquea**: NET-2/4/5/6, escenarios Raft-1/Raft-2/Peer-1a–Peer-1d del protocolo de medición.

### D3 · ADR-008 — Distribución y versionado de la matriz DES-3 en chaincode y baseline

- **Qué decide**: cómo consumen chaincode y baseline `domain/authorized-transfers.json` sin duplicar reglas (mandato de `domain/README.md`): (a) `go:embed` del JSON en el binario del chaincode (determinístico, simple; actualizar la matriz = upgrade de chaincode), (b) matriz persistida en el ledger, administrada por `AnmatMSP` con `regulatory-admin` (actualizable por transacción; requiere diseño de versionado on-ledger), o (c) híbrido. Decide además si cada transferencia persiste el `id` de la regla y la versión de matriz que la autorizó.
- **Por qué es necesaria**: ADR-005 dejó escrita la pregunta exacta que esto responde: para auditar una traza, ¿una transferencia se juzga por "la matriz de DES-3 **vigente al momento de esa transferencia**"? Eso solo es verificable si la versión de la regla queda registrada (opción de persistir `ruleId` + `schemaVersion` en el evento, cualquiera sea el mecanismo de distribución). También lo necesita la baseline para la paridad exigida por el protocolo de medición.
- **Recomendación a validar**: `go:embed` + persistir `ruleId`/versión en cada despacho. Es lo más simple, determinístico, y deja la traza auditable sin diseñar administración de matriz on-ledger que el prototipo no necesita.
- **Riesgo de pisar el trabajo escrito**: bajo; el trabajo escrito no fija el mecanismo. Cuidar la frase del paper sobre que la validación normativa "se traslada al chaincode" — ambas opciones la cumplen.
- **Desbloquea**: CC-3, BASE-2, y la semántica de verificación del financiador (D6).

### D4 · ADR-009 — Semántica de devolución y custodia en `DEVUELTO` (resuelve EXT-4)

- **Qué decide**: (1) quién queda como `CustodioActual` tras cada camino a `DEVUELTO` (T05 rechazo en tránsito vs. T21–T24 devoluciones desde custodia/cuarentena/retiro/vencido) — hoy solo T05 tiene respuesta (queda el emisor, por ADR-004); en T21 la unidad viaja físicamente de vuelta al proveedor pero ningún documento dice si la custodia registrada cambia, cuándo, ni mediante qué evento; (2) si la devolución entre actores en custodia confirmada es también un par despacho/recepción (simétrico a ADR-004) o un evento único; (3) la resolución del actor lógico `RECOVERY_OR_DISPOSAL_AGENT` de ADR-001 en términos de organizaciones/roles de DES-6 (cierra el hallazgo C5): quién puede ejecutar T25 (reingreso) y T28–T33 (disposición final) y con qué endoso; (4) si el chaincode distingue "rechazo en tránsito" de "devolución post-custodia" (pregunta que ADR-004 dejó expresamente a EXT-4).
- **Por qué es necesaria**: sin esto, 7 de las 33 transiciones de ADR-001 (T21–T25, T28, T33 al menos) no tienen regla de autorización implementable, y el contrato de API tiene operaciones (`ReturnProduct`, `Restock`, `FinalDisposition`) cuyo "actor habilitado" es ambiguo.
- **Riesgo de pisar el trabajo escrito**: medio. El paper describe el "Caso devolución" como "entrega y recepción de un medicamento como devolución **entre dos actores**" y el "Caso reingreso a stock" con validaciones concretas (no vencido, que quien reingresa **sea el actual custodio**, no destruido). La ADR debe reutilizar esas validaciones textuales — en particular, la frase del paper sobre el custodio sugiere que `RECOVERY_OR_DISPOSAL_AGENT` debería resolverse hacia "custodio actual registrado", lo cual simplificaría todo. Si se decide otra cosa, sección de divergencia.
- **Desbloquea**: CC-5/CC-6 (eventos extraordinarios y resolución), tests de la máquina de estados completa.

### D5 · ADR-010 — Identidad de las organizaciones no custodiales (ANMAT y financiador)

- **Qué decide**: cómo reconoce el chaincode a `AnmatMSP` y `FinanciadorMSP` (cierra el hallazgo E5): (a) MSP ID fijado por convención/configuración de despliegue, (b) entradas especiales del registro organización-establecimiento con `agentType` extendido (`REGULATOR`, `FINANCIER`) y `idType` propio, o (c) parámetro de instanciación del chaincode. Decide también si puede existir **más de un** financiador (el relevamiento habla de PAMI, obras sociales y prepagas en plural; DES-6 modela una sola `FinanciadorMSP` de ejemplo) y, si sí, cómo se dan de alta.
- **Por qué es necesaria**: todas las operaciones `REGULATORY_ONLY` del contrato y el coendoso regulatorio de DES-6 dependen de esta resolución; hoy es un hueco entre ADR-003 (registro solo custodial) y DES-6 (roles sin anclaje de identidad para no custodiales).
- **Recomendación a validar**: opción (b) — extender el catálogo del registro con tipos no custodiales. Mantiene el principio de ADR-003 (el registro como única fuente de verdad identidad↔organización, sin acoplarse al nombre de MSP) y da respuesta natural a múltiples financiadores. Requiere una nota: los no custodiales nunca son destino válido de la matriz DES-3 (que se mantiene en 6 `agentType`).
- **Riesgo de pisar el trabajo escrito**: bajo. El trabajo escrito lista financiadores en plural — la opción (b) es la más fiel.
- **Desbloquea**: CC-1 (registro), CC-7 (operaciones regulatorias), CC-8.

### D6 · ADR-011 — Criterios de verificación de traza del financiador (prerequisito de CC-8)

- **Qué decide**: la checklist determinística de "traza legítima" que ADR-005 dejó explícitamente pendiente ("Definir esa semántica verificable en detalle es un prerequisito para CC-8"): existencia de la unidad; estado `DISPENSADO`; agente dispensador con `agentType` habilitado; ¿cada transferencia del historial cumplía la matriz **vigente en ese momento** (depende de D3)?; ¿cada actor estaba `active` **al momento del evento** o alcanza con el estado actual?; ¿la secuencia de estados es un camino válido de ADR-001?; y qué devuelve la consulta (veredicto estructurado vs. historial crudo para que el financiador juzgue).
- **Por qué es necesaria**: es la única funcionalidad del financiador en el prototipo; sin criterios definidos, CC-8 no tiene criterio de aceptación y la afirmación del trabajo escrito ("validación informática del trazado... condición excluyente para la liberación de pagos") queda sin materialización verificable.
- **Riesgo de pisar el trabajo escrito**: bajo si la checklist se deriva de las validaciones que el paper ya enumera en §3.4 (custodia, unicidad, estado del producto). Atención a no prometer detección de irregularidades que el diseño no puede ver (p. ej. habilitación histórica de actores si el registro solo guarda el estado `active` actual — si es el caso, declararlo límite explícito, estilo "Límites de garantía" de ADR-003).
- **Desbloquea**: CC-8 y el guion de la demo de auditoría.

### D7 · ADR-012 — Diseño de la baseline centralizada y paridad funcional

- **Qué decide**: el stack concreto (el protocolo de medición ya asume PostgreSQL + API REST; falta decidirlo formalmente), el esquema relacional mínimo (unidad, historial de eventos, registro de establecimientos), cómo emula la identidad por organización (p. ej. una credencial/API key por establecimiento como análogo de la MSP), y el checklist de paridad: misma máquina de estados (ADR-001), misma matriz (vía D3), mismo modelo de dos fases (ADR-004), mismos códigos de error del contrato, misma exclusión de datos personales. Define también qué **no** replica (endoso multiparte, PDC) porque es precisamente lo que se compara.
- **Por qué es necesaria**: la comparación cuantitativa es la mitad de la hipótesis ("mejoras medibles... respecto al sistema centralizado actual"). El protocolo de medición exige "paridad funcional" y "mismos procesos core" pero ningún documento define qué significa eso en concreto; sin este diseño, cualquier diferencia de resultados es atacable ("compararon contra una baseline mal hecha").
- **Riesgo de pisar el trabajo escrito**: medio. El trabajo escrito describe la baseline como "interfaz de servicios sobre una base de datos relacional que implementa los mismos procesos core... bajo idénticas condiciones de carga" — la ADR debe citarlo como requisito y cuidar un punto: la baseline representa el *modelo* centralizado, no al SNT real de ANMAT; la tesis no puede afirmar que midió contra el sistema actual, sino contra un análogo. Dejarlo escrito en la ADR y en las limitaciones.
- **Desbloquea**: BASE-1/BASE-2, DES-7 ejecutable.

### D8 · Actualización de DES-7 — Medición de la transferencia de dos fases (decisión chica, no requiere ADR nuevo)

- **Qué decide**: cómo se cuenta la transferencia partida por ADR-004 en los benchmarks (cierra el hallazgo E3): recomendación — reportar `DispatchTransfer` y `ReceiveTransfer` como transacciones separadas (latencia/throughput propios) **más** la métrica derivada end-to-end del par; en la carga mixta, "transferencia 55%" = pares completos (despacho seguido de recepción por el worker correspondiente); la baseline expone y mide los mismos dos pasos.
- **Cómo materializarla**: una subsección nueva en `measurement-protocol.md` (§2 y §6) referenciando ADR-004. No amerita ADR aparte, pero sí debe decidirse **antes** de escribir los workloads de Caliper.

### D9 · Actualización de `alcance-prototipo.md` — Exclusiones pendientes de registrar (decisión chica)

Tres decisiones de alcance que hoy están tomadas de facto pero no registradas, y que deben agregarse como filas de la tabla de alcance para que la tesis pueda listarlas como limitaciones conscientes:

1. **Inicio de trazabilidad por droguería** (hallazgo E1): excluido en v1 — T01 solo `LABORATORY` — o modelado; decidir y registrar.
2. **Idempotencia de la recepción** (pendiente declarado por ADR-004): qué pasa si el receptor invoca `ReceiveTransfer` dos veces (la segunda debe fallar con `NOT_IN_TRANSIT` — confirmarlo como comportamiento esperado y testearlo, o diseñar algo más).
3. **Tránsito indefinido** (pendiente declarado por ADR-004): una unidad puede quedar en `EN_TRANSITO` para siempre si el receptor nunca confirma ni rechaza; registrar que el prototipo no implementa timeout/expiración y que la resolución operativa sería un evento extraordinario o devolución del emisor.

---

## Orden de ejecución (ejecutado el 2026-08-17)

Las flechas eran dependencias duras entre decisiones. Se conserva el grafo como registro del orden en que se tomaron:

```text
D5 (identidad no custodiales) ──┐
D3 (matriz en chaincode) ───────┼──► D6 (verificación financiador) ──► CC-8
D1 (PDC) ──► D2 (topología red) ┴──► CC-1..CC-7 / NET-*
D4 (devolución/EXT-4) ─────────────► CC-5/CC-6
D7 (baseline) ◄── D3, D8 ──────────► BASE-*, benchmarks
D8, D9: inmediatas (solo documentación), sin dependencias
```

Las nueve decisiones quedaron resueltas: D8 y D9 sobre los documentos existentes, D1–D7 como ADR-006 a ADR-012 (estado *Propuesto*). El trabajo pendiente ya no es de decisión sino de implementación: las issues de las áreas CC-*, NET-*, EXT-*, BASE-* y EVAL-* fueron actualizadas con las dependencias que cada ADR resuelve.

## Checklist de sincronización con el trabajo escrito

Cada afirmación arquitectónica del trabajo escrito quedó mapeada a la decisión que la implementa. La columna de resultado indica si la decisión la cumple (✔) o se aparta de ella (→ divergencia documentada en el ADR y pendiente de reflejar en el documento, ver [`paper-update-instructions.md`](paper-update-instructions.md)):

| Afirmación del trabajo escrito | Decisión | Resultado |
|---|---|---|
| Hash de datos privados en el ledger común, confidencialidad comercial | D1 · ADR-006 | ✔ cumple (hash del registro de operación en el read-write set del canal) |
| Distintas organizaciones contribuyen nodos de ordenamiento; sin administrador único | D2 · ADR-007 | ✔ cumple — se eligió deliberadamente la opción fiel (3 orderers en 3 organizaciones) en lugar de simplificar |
| Raft 3 nodos tolera 1 caída; prueba de disponibilidad | D2 · ADR-007 + DES-7 | ✔ cumple **con alcance acotado**: los 3 orderers pertenecen a 3 organizaciones de ordenamiento distintas, de modo que la descentralización es verificable en la configuración del canal. Lo que los escenarios Raft-1/Raft-2 miden es la tolerancia a la caída de **nodos lógicamente asignados** a organizaciones distintas, en host único y con fallos simulados a nivel de contenedor — no la caída física ni administrativa de una organización completa (ADR-007, Justificación; issue #55) |
| Validación normativa trasladada al chaincode, rechazo determinístico | D3 · ADR-008 | ✔ cumple (matriz embebida en el binario aprobado por las organizaciones) |
| Casos devolución y reingreso a stock con sus validaciones | D4 · ADR-009 | → **divergencia**: el reingreso aplica la validación del trabajo escrito, pero la devolución se simplifica a evento único (el texto la describe entre dos actores) |
| Financiadores (en plural: PAMI, obras sociales) con acceso de verificación | D5 · ADR-010, D6 · ADR-011 | ✔ cumple (múltiples financiadores soportados de forma nativa) |
| Validación de trazabilidad como condición de pago | D6 · ADR-011 | ✔ cumple (veredicto estructurado como condición evaluable), con límites declarados |
| Comparación contra prototipo centralizado equivalente, idénticas condiciones | D7 · ADR-012, D8 | ✔ cumple, con la precisión de que la baseline es un análogo del modelo centralizado y no el SNT real |
| Droguería como posible eslabón de origen de la trazabilidad | D9.1 | → **exclusión consciente** registrada en `alcance-prototipo.md` |
| CUFE acotado a laboratorios de producción pública | — (nota en ADR-003) | → **simplificación consciente**: el prototipo lo admite para cualquier `agentType` |
| Una CA por organización (modelo de confianza distribuido) | D2 · ADR-007 | → una CA lógica y raíz propia por organización en un servidor compartido; límite declarado: operador común |
