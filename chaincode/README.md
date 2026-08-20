# Chaincode `snt` (pendiente de implementación)

Este directorio contendrá el smart contract `snt` del prototipo: un contrato Go implementado con `contractapi`, cuya interfaz pública está congelada en [`docs/api-contract.md`](../docs/api-contract.md) (v2.6.1).

La lógica debe respetar:

- la máquina de estados del medicamento definida en ADR-001;
- la identidad de establecimientos por MSP/GLN-CUFE definida en ADR-003;
- la transferencia como dos transacciones (despacho/recepción) definida en ADR-004;
- la matriz regulatoria de transferencias [`domain/authorized-transfers.json`](../domain/authorized-transfers.json).
