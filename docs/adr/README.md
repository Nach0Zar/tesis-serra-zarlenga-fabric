# Architecture Decision Records

Este directorio contiene las decisiones de arquitectura que guían el diseño y la implementación del prototipo.

## Archivos

| Archivo | Estado | Descripción |
|---|---|---|
| [`000-template.md`](000-template.md) | Plantilla | Estructura base para nuevos ADRs. |
| [`001-maquina-estados-medicamento.md`](001-maquina-estados-medicamento.md) | Aceptado | ADR-001: máquina de estados del medicamento, eventos, actores lógicos, precondiciones y estados terminales. |
| [`002-topologia-canales.md`](002-topologia-canales.md) | Aceptado (revisión 3) | ADR-002: topología de canales y uso de Private Data Collections. |
| [`003-establishment-identity-gln-cufe.md`](003-establishment-identity-gln-cufe.md) | Aceptado (revisión 2) | ADR-003: identidad de establecimientos mediante GLN/CUFE y organización Fabric por establecimiento. |
| [`004-transfer-dispatch-reception.md`](004-transfer-dispatch-reception.md) | Aceptado | ADR-004: transferencia como dos transacciones separadas (despacho/recepción); semántica de `DestinatarioPendiente`. |
| [`005-rol-organismo-financiador.md`](005-rol-organismo-financiador.md) | Aceptado | ADR-005: rol del organismo financiador como verificador de trazabilidad de solo lectura, posterior a la dispensa. |
| [`006-private-data-collections.md`](006-private-data-collections.md) | Propuesto | ADR-006: colecciones privadas explícitas por par de organizaciones, generadas programáticamente; claves y ciclo de vida del registro de operación; marcadores de participación en colecciones implícitas como segundo uso de datos privados. |
| [`007-network-topology.md`](007-network-topology.md) | Propuesto | ADR-007: topología física — Raft de 3 orderers en 3 organizaciones, LevelDB, un servidor Fabric CA con una CA lógica y raíz propia por organización, canal `snt-channel`, bootstrap en dos secuencias de lifecycle, y materialización del endoso por tres mecanismos combinados: política de chaincode `OR(custodiales, regulatoria)`, state-based endorsement por clave y marcadores de participación en colecciones implícitas. |
| [`008-transfer-matrix-distribution.md`](008-transfer-matrix-distribution.md) | Propuesto | ADR-008: matriz DES-3 distribuida por `go:embed` y paquete Go compartido con la baseline; `ruleId` y versión persistidos por despacho. |
| [`009-return-and-recovery-semantics.md`](009-return-and-recovery-semantics.md) | Propuesto | ADR-009: devolución como evento único sin cambio de custodia; `RECOVERY_OR_DISPOSAL_AGENT` resuelto como custodio actual. |
| [`010-non-custodial-identity.md`](010-non-custodial-identity.md) | Propuesto | ADR-010: identidad de ANMAT y financiadores mediante `agentType` no custodiales en el registro; bootstrap regulatorio en la init del chaincode. |
| [`011-financier-trace-verification.md`](011-financier-trace-verification.md) | Propuesto | ADR-011: checklist determinística de cinco comprobaciones y veredicto estructurado para la verificación de traza del financiador. |
| [`012-baseline-design.md`](012-baseline-design.md) | Propuesto | ADR-012: baseline centralizada en Go + PostgreSQL con paquete compartido; checklist normativo de paridad funcional. |
| [`013-acquirer-authenticity-verification.md`](013-acquirer-authenticity-verification.md) | Propuesto | ADR-013: checklist determinística de cuatro comprobaciones y veredicto estructurado para la verificación de autenticidad del adquirente; comparte con ADR-011 las comprobaciones de cadena de custodia. |

## Convenciones

- Cada decisión de arquitectura no trivial debe registrarse como un ADR independiente.
- La documentación se redacta en español.
- Los nombres de archivo se mantienen en inglés.
- Las fuentes técnicas o regulatorias externas se incluyen en anexos del documento correspondiente.
- No listar ADRs futuros como archivos existentes hasta que su issue produzca el documento correspondiente.