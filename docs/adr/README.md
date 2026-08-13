# Architecture Decision Records

Este directorio contiene las decisiones de arquitectura que guían el diseño y la implementación del prototipo.

## Archivos

| Archivo | Estado | Descripción |
|---|---|---|
| [`000-template.md`](000-template.md) | Plantilla | Estructura base para nuevos ADRs. |
| [`001-maquina-estados-medicamento.md`](001-maquina-estados-medicamento.md) | Propuesto | ADR-001: máquina de estados del medicamento, eventos, actores lógicos, precondiciones y estados terminales. |
| [`002-topologia-canales.md`](002-topologia-canales.md) | Aceptado (revisión 3) | ADR-002: topología de canales y uso de Private Data Collections. |
| [`003-establishment-identity-gln-cufe.md`](003-establishment-identity-gln-cufe.md) | Aceptado (revisión 2) | ADR-003: identidad de establecimientos mediante GLN/CUFE y organización Fabric por establecimiento. |
| [`005-rol-organismo-financiador.md`](005-rol-organismo-financiador.md) | Aceptado | ADR-005: rol del organismo financiador como verificador de trazabilidad de solo lectura, posterior a la dispensa. |

## Convenciones

- Cada decisión de arquitectura no trivial debe registrarse como un ADR independiente.
- La documentación se redacta en español.
- Los nombres de archivo se mantienen en inglés.
- Las fuentes técnicas o regulatorias externas se incluyen en anexos del documento correspondiente.
- No listar ADRs futuros como archivos existentes hasta que su issue produzca el documento correspondiente.