# Red Fabric del prototipo (pendiente)

Este directorio contendrá la configuración de la red Hyperledger Fabric del prototipo, conforme [ADR-007](../docs/adr/007-network-topology.md):

- canal `snt-channel`, único, con Private Data Collections (ADR-002); las colecciones se generan programáticamente según [ADR-006](../docs/adr/006-private-data-collections.md);
- una organización Fabric por establecimiento (ADR-003), más `AnmatMSP` y las organizaciones financiadoras;
- servicio de ordenamiento Raft de 3 nodos aportados por 3 organizaciones distintas;
- un peer por organización con LevelDB, y una única instancia de Fabric CA (simplificación con límite de confianza declarado);
- despliegue en dos secuencias de lifecycle: bootstrap del registro regulatorio bajo política estricta ([ADR-010](../docs/adr/010-non-custodial-identity.md)) y definición operativa.

Pendiente de implementación en las issues NET-1 a NET-6.
