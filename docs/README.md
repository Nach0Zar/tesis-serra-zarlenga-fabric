# Documentación

Este directorio agrupa documentos de diseño, decisiones de arquitectura y protocolos que guían la implementación del prototipo.

## Contenido

| Ruta | Contenido |
|---|---|
| `adr/` | Architecture Decision Records aceptados o en revisión. |

## Decisiones de arquitectura

- [ADR-002: Topología de canales en la red Hyperledger Fabric](adr/002-topologia-canales.md): define canal único con estado mínimo de trazabilidad público y Private Data Collections para información comercial, conciliando auditoría regulatoria de ANMAT, confidencialidad comercial y validación independiente por futuros destinatarios.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](adr/003-establishment-identity-gln-cufe.md): define una organización Fabric por establecimiento, con un registro en ledger que traduce cada organización a su identificador canónico GLN/CUFE.

## Convenciones

- La documentación se redacta en español.
- Los nombres de archivo se mantienen en inglés.
- Las fuentes técnicas o regulatorias externas se incluyen en anexos del documento correspondiente.
