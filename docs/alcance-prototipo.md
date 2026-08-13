# Alcance del prototipo

## Objetivo

Este documento registra decisiones conscientes de alcance del prototipo del PFI. Su objetivo no es redefinir la máquina de estados, el contrato de API, la matriz de transferencias, los roles de red ni la implementación de Fabric o de la baseline, sino dejar trazables inclusiones simplificadas y exclusiones que luego servirán de base para definir las limitaciones.

El criterio adoptado para la versión inicial es representar los procesos core ya documentados para el flujo downstream del SNT: registro de unidades, transferencias de custodia, dispensación ordinaria, fin de envase hospitalario y eventos extraordinarios definidos en los artefactos de diseño correspondientes. Las exclusiones listadas abajo no niegan que el SNT real contemple esos comportamientos; solo fijan que no se implementan en el prototipo v1.

## Decisiones de alcance

| Punto relevado | Decisión v1 | Justificación | Consecuencia |
|---|---|---|---|
| Fraccionamiento hospitalario / fin de envase | Incluido como simplificación. El prototipo representa que el envase sale de circulación reutilizando `T06_DISPENSE` (`EN_CUSTODIA` -> `DISPENSADO`) cuando el custodio es un establecimiento asistencial. | El avance de tesis releva que los establecimientos asistenciales deben notificar el fraccionamiento o fin de envase, aunque no reporten cada administración interna a pacientes institucionalizados. ADR-001 ya permite que `DISPENSING_AGENT` sea una farmacia o establecimiento asistencial y no exige persistir datos personales del paciente. | No se agrega un estado, evento ni payload específico para fin de envase. La limitación a documentar es la falta de distinción semántica entre venta/dispensación ambulatoria y fin de envase hospitalario dentro del evento vigente. |
| Administraciones internas a pacientes institucionalizados | Excluidas. El prototipo no modela cada administración interna hospitalaria, dosis, cama, servicio ni paciente institucionalizado. | El avance de tesis registra que esas administraciones internas están eximidas de reporte al SNT; el hecho trazable relevante para el prototipo es el fin de envase, cubierto en la fila anterior. | No se persisten datos clínicos ni personales asociados a administraciones internas. Se debe diferenciar esta exclusión del fin de envase, que queda incluido de forma simplificada. |
| Validación contra Vademécum Nacional, REM o catálogo oficial de productos | Excluida. El prototipo asume que el GTIN y el producto ingresados pertenecen al universo trazable válido. | Integrar o sincronizar una fuente oficial de productos agregaría una dependencia externa y reglas de vigencia que no forman parte de las issues de diseño e implementación actuales. Para las pruebas y benchmarks, el dataset sintético debe construirse con productos considerados válidos por premisa. | Registrar una unidad en el prototipo no demuestra por sí mismo que el producto exista o esté vigente en REM/Vademécum. Esta es una limitación explícita de alcance y debe mencionarse al interpretar resultados o conclusiones. |

