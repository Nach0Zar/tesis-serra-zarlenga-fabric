# Evaluación y toma de decisiones de arquitectura

- **Alcance**: ADR-001 a ADR-005
- **Fecha de consolidación**: 2026-08-17
- **Autores de las decisiones**: Serra, Zarlenga
- **Naturaleza**: síntesis transversal del proceso de evaluación

---

## 1. Propósito

Este documento explica cómo fueron evaluadas y tomadas en consideración las ADR-001 a ADR-005. No reemplaza las decisiones individuales: explicita el método común reconstruido a partir de sus contextos, alternativas, fuentes, justificaciones, consecuencias y dependencias.

La reconstrucción distingue entre:

- **evidencia directa**: normativa, documentación oficial, paper, tesis, entrevistas, issues y artefactos versionados;
- **restricciones de diseño**: alcance, privacidad, auditabilidad, determinismo, identidad, endoso y paridad experimental;
- **inferencia arquitectónica**: consecuencias derivadas de Fabric o de la interacción entre decisiones;
- **decisión del equipo**: alternativa seleccionada con beneficios, costos y pendientes.

No se presuponen reuniones, votaciones ni ponderaciones numéricas que no estén documentadas.

## 2. Evidencia considerada

| Categoría | Fuentes | Uso |
|---|---|---|
| Normativa SNT | Resolución MS 435/2011; disposiciones ANMAT 3683/2011 y 963/2015 | Eventos, identificación, trazabilidad, confidencialidad y responsabilidad regulatoria. |
| Normativa complementaria | Resolución PAMI 1735/2016; Disposición PAMI 1/17; Ley 25.326 | Validación del financiador, pago y minimización de datos personales. |
| Evidencia del proyecto | Paper, avance de tesis y alcance del prototipo | Alineación con la hipótesis, flujo downstream y procesos incluidos. |
| Relevamiento de campo | Entrevista vinculada con farmacias y financiadores | Contraste del flujo de dispensa, auditoría y pago con la práctica. |
| Documentación técnica | Hyperledger Fabric, Fabric CA y APIs de chaincode | Viabilidad de canales, PDC, MSP, endoso, historial e identidad. |
| Artefactos internos | Issues DES, ADR, modelo de datos, matriz de transferencias, contrato API y protocolo de medición | Coherencia, dependencias y trazabilidad hacia implementación y baseline. |

Cuando las fuentes admitieron interpretaciones distintas se priorizó la evidencia textual y contextual más específica. ADR-005, por ejemplo, no tomó la adyacencia visual de una figura como prueba de temporalidad: la contrastó con el texto del paper, normativa de PAMI y entrevista.

## 3. Criterios de evaluación

Las alternativas fueron comparadas cualitativamente. No existe una matriz de puntajes documentada; la selección se basó en el cumplimiento conjunto de estos criterios:

1. **Fidelidad normativa y de dominio**: representar el SNT sin atribuir a la normativa obligaciones inexistentes.
2. **Trazabilidad y auditabilidad**: permitir reconstruir el ciclo de vida y distinguir hechos relevantes.
3. **Confidencialidad y minimización**: proteger información comercial y excluir datos personales innecesarios.
4. **Integridad multiorganizacional**: permitir validación determinística por participantes independientes.
5. **Correspondencia con Fabric**: respetar la granularidad real de canales, MSP, PDC, endoso e identidad.
6. **Viabilidad operativa**: considerar canales, organizaciones, onboarding, políticas y mantenimiento.
7. **Coherencia interna**: respetar ADR previas o declarar explícitamente su modificación.
8. **Paridad experimental**: implementar la misma semántica en Fabric y la baseline.
9. **Determinismo**: hacer las reglas explícitas, reproducibles y aptas para chaincode.
10. **Trade-offs**: registrar complejidad, pérdidas, riesgos y cuestiones pendientes.

## 4. Proceso reconstruido

El patrón común observado fue:

1. delimitar el problema y lo que queda fuera de alcance;
2. identificar restricciones normativas, funcionales, técnicas y experimentales;
3. formular alternativas plausibles;
4. descartar alternativas por incumplimientos concretos;
5. seleccionar la que mejor satisface el conjunto de restricciones prioritarias;
6. comprobar compatibilidad con decisiones existentes;
7. propagar consecuencias hacia red, chaincode, datos, API, baseline y pruebas;
8. registrar costos y pendientes;
9. conservar trazabilidad mediante issues, fuentes y referencias cruzadas;
10. revisar la decisión si cambia la evidencia o sus supuestos.

## 5. Evaluación por ADR

### ADR-001: Máquina de estados

**Problema:** representar el ciclo de vida de una unidad y restringir operaciones futuras.

Se compararon estados mínimos (`ACTIVA/BLOQUEADA/TERMINAL`), event sourcing sin estado canónico y una máquina explícita. La tercera alternativa fue propuesta porque ANMAT diferencia eventos con efectos operativos distintos y el chaincode necesita validar estado, evento, actor y precondiciones de forma determinística. Se sacrifica simplicidad, pero se gana semántica regulatoria, control directo y paridad con la baseline.

Condiciona el estado persistido, el contrato público, la matriz de transferencias y la resolución de actores. ADR-004 conserva `EN_TRANSITO`.

> **Control de estado:** el encabezado y el índice mantienen ADR-001 como **Propuesto**. Aunque ADR-004 menciona aceptación por el equipo, el estado de la propia ADR debe considerarse autoritativo hasta una actualización explícita.

### ADR-002: Topología y privacidad

**Problema:** conciliar auditoría regulatoria, confidencialidad comercial y validación independiente.

Se compararon canal único abierto, múltiples canales y canal común con estado mínimo compartido más PDC. El canal abierto exponía información comercial; la malla fragmentaba auditoría y estado, y crecía combinatoriamente. Se eligió canal común + PDC: identificador, lote, vencimiento, custodio y estado quedan verificables dentro de la red permisionada; información comercial y documental se restringe a participantes y ANMAT, conservando su hash.

El costo aceptado es diseñar, mantener y probar clasificación y membresía de colecciones.

### ADR-003: Identidad GLN/CUFE

**Problema:** definir la unidad real de identidad y aislamiento.

Se compararon MSP por categoría con atributos, MSP por establecimiento y cifrado en aplicación. Fabric aplica privacidad a nivel de organización, no de atributo individual; por ello una MSP compartida no aísla establecimientos. El cifrado tampoco resuelve datos que el chaincode debe leer.

Se eligió una organización/MSP por establecimiento y un registro `mspId -> GLN/CUFE, agentType, active`. El custodio se persiste con identidad canónica de dominio, desacoplada del nombre técnico de MSP. Se acepta mayor costo de onboarding y políticas a cambio de aislamiento real.

### ADR-004: Despacho y recepción

**Problema:** modelar la transferencia como operación atómica o como dos hechos.

La operación atómica era más simple, pero eliminaba `EN_TRANSITO`, no representaba la separación normativa entre distribución y recepción y dificultaba el rechazo. Se eligieron dos transacciones: el emisor despacha y el receptor recibe o rechaza.

Durante el tránsito el custodio público continúa siendo el emisor. El destinatario pendiente se envía mediante `transient` y se guarda en PDC, evitando exponer una relación no consumada. Se gana auditabilidad y fidelidad logística; se aceptan complejidad transitoria, dependencia de PDC y pendientes de idempotencia/timeout.

### ADR-005: Organismo financiador

**Problema:** decidir si el financiador autoriza antes, verifica después o queda fuera.

El texto del paper, la normativa PAMI y la entrevista ubican la verificación de trazabilidad como condición de pago posterior, no como precondición de la entrega física. Hacerlo coendosante agregaría latencia, disponibilidad obligatoria y una escritura incompatible con su rol no custodial.

Se eligió verificación posterior de solo lectura, dirigida por reclamo. El financiador consulta seriales conocidos off-ledger y confirma existencia, estado `DISPENSADO` y agente habilitado. Pago y vínculo afiliado-unidad permanecen fuera del ledger; no se incorporan datos del afiliado ni acceso a PDC comerciales ajenas.

## 6. Matriz consolidada

| ADR | Tensión | Alternativa elegida | Beneficio prioritario | Costo aceptado |
|---|---|---|---|---|
| 001 | Simplicidad vs. semántica | Máquina explícita | Validación determinística | Más reglas y transiciones |
| 002 | Validación vs. confidencialidad | Canal común + PDC | Estado verificable y datos restringidos | Gestión de colecciones |
| 003 | Gestión simple vs. aislamiento | MSP por establecimiento | Privacidad alineada con GLN/CUFE | Onboarding complejo |
| 004 | Atomicidad vs. fidelidad logística | Despacho + recepción | Auditoría y rechazo | Estado transitorio e idempotencia |
| 005 | Integración vs. desacoplamiento | Lectura posterior | Privacidad y disponibilidad | Pago fuera del ledger |

## 7. Dependencias

Las decisiones forman una cadena:

- ADR-001 define **qué puede ocurrir**.
- ADR-002 define **qué puede ver cada participante**.
- ADR-003 define **quién representa a cada establecimiento**.
- ADR-004 define **cómo ocurre una transferencia en el tiempo**.
- ADR-005 define **cómo consume la traza un actor no custodial**.

Modificar una puede obligar a revisar otras. Eliminar `EN_TRANSITO` afecta ADR-001/004; compartir MSP afecta ADR-002/003; exigir endoso del financiador afecta ADR-005, DES-6, latencia y disponibilidad; publicar el destinatario pendiente afecta ADR-002/004.

## 8. Validación hacia implementación

Una decisión se considera trasladada al prototipo cuando puede rastrearse hacia:

- criterios de aceptación de su issue;
- modelo de datos y contrato público;
- configuración de red, MSP, PDC y endoso;
- implementación equivalente en chaincode y baseline;
- pruebas positivas, negativas y de privacidad;
- protocolo de medición, si afecta el proceso comparado.

Aceptar una ADR no equivale a validarla empíricamente. Siguen pendientes, entre otros:

- impacto de PDC sobre rendimiento;
- automatización de onboarding y membresía;
- idempotencia y transferencias indefinidamente en tránsito;
- semántica exacta de una traza irregular para CC-8;
- actualización formal del estado de ADR-001.

## 9. Regla para ADR futuras

Toda ADR debería contener: contexto y alcance; fuentes y distinción entre obligación, evidencia e inferencia; alternativas; criterios de descarte; decisión; justificación; consecuencias positivas y negativas; dependencias; pendientes; y estado formal.

Si se incorpora una matriz cuantitativa, criterios y pesos deben fijarse antes de puntuar para evitar justificar retrospectivamente una opción ya elegida.
