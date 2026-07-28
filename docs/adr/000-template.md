# ADR-000: Topología de canales en la red Hyperledger Fabric

- **Estado**: Aceptado
- **Fecha**: 2026-07-25
- **Autores**: Serra, Zarlenga

---

## Contexto

La red Hyperledger Fabric permite segmentar la información mediante canales. Cada canal tiene su propio ledger, y solo las organizaciones miembro de un canal pueden ver sus transacciones. El SNT impone dos requisitos en tensión:

1. **Visibilidad regulatoria**: ANMAT debe poder auditar la totalidad de los eventos de la cadena.
2. **Confidencialidad comercial**: un laboratorio no debe ver las operaciones entre una droguería y una farmacia competidora.

La decisión de topología de canales impacta directamente en la arquitectura de la red, el diseño del chaincode y las políticas de endoso, por lo que no puede diferirse a la implementación.

## Alternativas

**A. Canal único compartido por todos los actores**
- Todos los nodos tienen visibilidad completa del ledger.
- ANMAT puede auditar sin mecanismos adicionales.
- Elimina la confidencialidad comercial entre competidores.
- Menor complejidad operativa.

**B. Canal por par de organizaciones (malla)**
- Máxima confidencialidad: cada transacción es visible solo para las partes involucradas.
- ANMAT requeriría pertenecer a todos los canales o recibir reportes fuera de la red, rompiendo el modelo de auditoría distribuida.
- Complejidad de gestión exponencial con el número de actores.

**C. Canal único con colecciones de datos privados (PDC)**
- Un solo canal para todas las organizaciones.
- Los datos sensibles de cada transacción se comparten solo con el subconjunto de organizaciones autorizado mediante colecciones privadas.
- El ledger compartido almacena únicamente el hash de los datos privados, permitiendo la verificación de integridad sin exponer el contenido.
- ANMAT se incluye como miembro de todas las colecciones en su rol de auditoría.
- Complejidad media: requiere definir una política de colecciones por tipo de transacción.

## Decisión

Se adopta la **alternativa C**: canal único con colecciones de datos privados.

## Justificación

Las colecciones de datos privados permiten conciliar los dos requisitos en tensión sin multiplicar la complejidad operativa de la red. La Disposición ANMAT 3683/2011 establece explícitamente que "ningún establecimiento acceda a información de la cadena de distribución correspondiente a transacciones de otros establecimientos de las que no forman parte", lo que descarta el canal único sin PDC (alternativa A). La malla de canales (alternativa B) hace imposible el rol de auditoría centralizada de ANMAT sin romper el modelo distribuido.

Hyperledger Fabric 2.5 soporta nativamente PDC con endoso y validación sobre el hash, lo que mantiene la inmutabilidad del ledger para los datos privados sin requerir infraestructura adicional.

## Consecuencias

- **Se gana**: confidencialidad comercial entre competidores, auditoría unificada de ANMAT, un único ledger a mantener.
- **Se pierde**: simplicidad de un canal abierto; el diseño del chaincode debe definir explícitamente las políticas de colecciones para cada tipo de evento del SNT.
- **Queda pendiente**: definir la granularidad de las colecciones (¿una por tipo de transferencia?, ¿una por par de organizaciones?) y validar que el rendimiento bajo carga no se vea afectado significativamente respecto al canal abierto.
