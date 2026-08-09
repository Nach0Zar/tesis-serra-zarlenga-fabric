# Alcance del prototipo

## Objetivo

Este documento registra exclusiones conscientes del prototipo del PFI para la issue DES-11. Su objetivo no es redefinir la máquina de estados, el contrato de API, la matriz de transferencias, los roles de red ni la implementación de Fabric o de la baseline, sino dejar trazables las simplificaciones que luego alimentan DOC-7 sobre limitaciones.

El criterio adoptado para la versión inicial es representar los procesos core ya documentados para el flujo downstream del SNT: registro de unidades, transferencias de custodia, dispensación ordinaria y eventos extraordinarios definidos en los artefactos de diseño correspondientes. Las exclusiones listadas abajo no niegan que el SNT real contemple esos comportamientos; solo fijan que no se implementan en el prototipo v1.

## Decisiones de alcance

| Punto relevado | Decisión v1 | Justificación | Consecuencia |
|---|---|---|---|
| Fraccionamiento hospitalario / fin de envase | Excluido. El prototipo no modela administraciones internas ni un evento específico de fin de envase para establecimientos asistenciales. | El prototipo opera sobre unidades trazables y flujos de custodia. Modelar fraccionamiento requeriría decidir nuevos eventos, datos de inventario interno y semántica de cierre de envase, lo que excede DES-11 y se pisaría con decisiones de máquina de estados, contrato público y paridad Fabric/baseline. | Los establecimientos asistenciales pueden seguir apareciendo como custodios o agentes de dispensación según los artefactos vigentes, pero la administración interna hospitalaria queda fuera del comportamiento verificable. La dispensación ambulatoria ordinaria sigue representada por el flujo de dispensación. |
| Validación contra Vademécum Nacional, REM o catálogo oficial de productos | Excluida. El prototipo asume que el GTIN y el producto ingresados pertenecen al universo trazable válido. | Integrar o sincronizar una fuente oficial de productos agregaría una dependencia externa y reglas de vigencia que no forman parte de las issues de diseño e implementación actuales. Para las pruebas y benchmarks, el dataset sintético debe construirse con productos considerados válidos por premisa. | Registrar una unidad en el prototipo no demuestra por sí mismo que el producto exista o esté vigente en REM/Vademécum. Esta es una limitación explícita de alcance y debe mencionarse al interpretar resultados o conclusiones. |

## Contexto utilizado

- `docs/adr/001-maquina-estados-medicamento.md`: define la dispensación y los estados/eventos del medicamento que no se alteran por este documento.
- `domain/README.md`: define el alcance de transferencias ordinarias y los agentes, incluidos establecimientos asistenciales, sin cubrir fraccionamiento interno.
