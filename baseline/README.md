# Baseline centralizada (pendiente)

Este directorio contendrá la línea base centralizada del trabajo: una API de servicios sobre base de datos relacional que implementa los mismos procesos core del SNT, con las mismas validaciones, para la comparación experimental definida en [`docs/measurement-protocol.md`](../docs/measurement-protocol.md).

Para preservar la paridad funcional con el prototipo Fabric, la baseline debe:

- replicar el modelo de transferencia en dos transacciones (despacho/recepción) de ADR-004;
- consumir la matriz [`domain/authorized-transfers.json`](../domain/authorized-transfers.json) como fuente única de reglas de transferencia;
- mantener la misma semántica de errores (códigos estables) del contrato [`docs/api-contract.md`](../docs/api-contract.md).
