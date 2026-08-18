# Trazabilidad farmacéutica sobre Hyperledger Fabric — PFI UADE

Proyecto Final de Ingeniería (UADE, 2026). El trabajo evalúa la siguiente hipótesis: ¿es técnicamente factible implementar los procesos core del Sistema Nacional de Trazabilidad de Medicamentos (SNT) de ANMAT sobre Hyperledger Fabric, cumpliendo la normativa vigente y con mejoras medibles en disponibilidad, integridad y auditoría respecto del sistema centralizado actual? Para responderla se construye un prototipo Fabric y una línea base centralizada funcionalmente equivalente, comparadas bajo el protocolo de medición del repositorio.

## Autores

- Serra, Juan Cruz Bautista
- Zarlenga, Ignacio

Tutora: Dra. Mg. Ing. María Roxana Martínez — UADE, 2026.

## Mapa del repositorio

| Directorio | Contenido |
|---|---|
| [`docs/`](docs/README.md) | Documentación de diseño, protocolos y ADRs. |
| [`domain/`](domain/README.md) | Matriz regulatoria de transferencias autorizadas (origen → destino). |
| [`chaincode/`](chaincode/README.md) | Smart contract `snt` en Go — pendiente de implementación. |
| [`baseline/`](baseline/README.md) | Línea base centralizada para la comparación — pendiente. |
| [`network/`](network/README.md) | Configuración de la red Fabric del prototipo — pendiente. |
| [`benchmarks/`](benchmarks/README.md) | Workloads y resultados de medición — pendiente. |
| [`client/`](client/README.md) | Cliente que consume el contrato del chaincode — pendiente. |

## Documentos principales

- [`docs/README.md`](docs/README.md) — índice de la documentación de diseño.
- [`docs/adr/README.md`](docs/adr/README.md) — índice de Architecture Decision Records.
- [`docs/api-contract.md`](docs/api-contract.md) — contrato congelado de la interfaz pública del chaincode `snt` (v2.1.0).

## Convenciones

- La documentación se redacta en español; los nombres de archivo se mantienen en inglés.
