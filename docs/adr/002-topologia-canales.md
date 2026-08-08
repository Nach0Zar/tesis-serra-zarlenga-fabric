# ADR-002: Topología de canales en la red Hyperledger Fabric

- **Estado**: Propuesto
- **Fecha**: 2026-08-02
- **Autores**: Serra, Zarlenga

---

## Contexto

El prototipo del PFI busca representar procesos core del Sistema Nacional de Trazabilidad de Medicamentos (SNT) sobre una red blockchain permisionada basada en Hyperledger Fabric. El alcance documentado en la tesis se limita al flujo downstream regulado por ANMAT e incluye registro de lotes, transferencias entre agentes y dispensación, con evaluación frente a una línea base centralizada.

La arquitectura del SNT vigente opera bajo administración centralizada de ANMAT. La Disposición ANMAT 3683/2011 exige que el sistema permita a la autoridad tomar conocimiento en tiempo real de irregularidades, anomalías o desviaciones, y que ningún establecimiento acceda a información de la cadena de distribución correspondiente a transacciones de otros establecimientos de las que no forme parte. Por lo tanto, la topología de Fabric debe resolver dos requisitos en tensión:

1. **Visibilidad regulatoria**: ANMAT debe poder auditar la trazabilidad relevante de las unidades alcanzadas por el prototipo.
2. **Confidencialidad comercial**: los establecimientos no deben observar datos privados de operaciones en las que no participan.

El paper del proyecto identifica la topología de canales como una decisión de diseño y considera tres enfoques: canal único, múltiples canales y canal único con colecciones de datos privados. También indica que las PDC permiten compartir datos solo con un subconjunto autorizado sin crear un canal separado, manteniendo en el ledger común un hash verificable de la información privada.

## Alternativas

### A. Canal único compartido por todos los actores

- Todas las organizaciones participan de un mismo canal y mantienen un ledger común.
- ANMAT conserva máxima visibilidad operacional porque toda transacción queda en el mismo canal.
- La complejidad operativa inicial es baja: un canal, un chaincode desplegado sobre ese canal y una superficie de administración reducida.
- Se descarta porque expone información comercial entre organizaciones que no forman parte de una operación determinada. Esa exposición contradice el requisito normativo de no acceso a transacciones ajenas.

### B. Múltiples canales

- Cada subconjunto de organizaciones con necesidad de privacidad opera en un canal separado.
- La confidencialidad entre subconjuntos mejora porque cada canal tiene su propio ledger y sus propios miembros.
- La auditoría regulatoria se fragmenta: ANMAT tendría que integrarse a todos los canales relevantes o recibir información por mecanismos externos al ledger donde ocurre la operación.
- La complejidad crece con la cantidad de relaciones, establecimientos y casos de uso. La tesis delimita un prototipo, pero el dominio regulatorio representa una red con muchos actores, por lo que una malla de canales no es una base adecuada para escalar la decisión.
- Se descarta porque maximiza el aislamiento a costa de complejidad operativa y de una auditoría menos simple de defender.

### C. Canal único con Private Data Collections

- Todas las organizaciones del prototipo comparten un canal común para preservar una traza auditable y una administración de red acotada.
- Los datos privados de una operación se almacenan en colecciones accesibles solo por las organizaciones autorizadas.
- Las organizaciones no autorizadas no reciben el dato privado, pero conservan en el ledger común el hash que permite verificar integridad.
- ANMAT participa en las colecciones necesarias para cumplir su rol de autoridad de aplicación y auditoría.
- La complejidad es media: requiere diseñar colecciones y políticas de membresía, pero evita multiplicar canales.

## Decisión

Se adopta la alternativa **C: canal único con Private Data Collections**.

El prototipo tendrá un canal común para los participantes de la red Fabric y usará PDC para los datos que no deban ser visibles por organizaciones ajenas a la operación. La regla arquitectónica para las colecciones es:

- miembros privados: organizaciones participantes directas de la operación;
- miembro auditor: `AnmatMSP`, cuando la información sea necesaria para fiscalización regulatoria;
- no miembros: organizaciones sin participación directa, que solo deben conservar la evidencia hasheada disponible en el ledger común.

Esta decisión no fija nombres definitivos de colecciones, campos de datos privados ni archivos `collections_config.json`. Esos detalles pertenecen a NET-5 y deben implementarse respetando esta regla de visibilidad.

## Justificación

La alternativa elegida es la que mejor concilia la tensión central de DES-4. Un canal único sin PDC favorece la auditoría, pero no satisface la confidencialidad comercial requerida por la Disposición ANMAT 3683/2011. Múltiples canales favorecen el aislamiento, pero introducen una fragmentación que dificulta sostener una auditoría regulatoria unificada y aumenta la carga operativa de red.

Hyperledger Fabric documenta que los canales sirven cuando transacciones y ledgers completos deben mantenerse confidenciales dentro de un conjunto de organizaciones, mientras que las colecciones privadas son adecuadas cuando las transacciones deben compartirse en un canal pero solo un subconjunto debe acceder a determinados datos. Esa propiedad coincide con el escenario del SNT: ANMAT necesita capacidad de auditoría transversal, pero cada establecimiento solo debe acceder a información de operaciones donde participa.

La decisión también conserva coherencia con el paper y la tesis del proyecto: ambos enmarcan Fabric como red privada y permisionada, con canales, MSP, chaincode y mecanismos de privacidad como herramientas para representar reglas regulatorias sin adoptar una red pública ni una base centralizada tradicional.

## Colecciones privadas

Las PDC se definen en este ADR como mecanismo de privacidad selectiva dentro del canal común.

Política de diseño para NET-5:

- Cada dato privado debe pertenecer a una colección cuya membresía incluya solo a las organizaciones con necesidad legítima de lectura.
- `AnmatMSP` debe incluirse cuando el dato privado sea necesario para auditar trazabilidad, irregularidades, desviaciones o cumplimiento regulatorio.
- Ninguna organización competidora o ajena a la operación debe ser miembro de la colección por conveniencia operativa.
- La evidencia pública del canal debe limitarse a lo que Fabric registra para validación e integridad, incluyendo hashes de datos privados, sin usar el canal común para replicar el contenido sensible.
- La granularidad exacta de colecciones debe definirse en NET-5 a partir de esta regla, una vez confirmados los MSP de DES-6 y las decisiones de identidad de DES-8.

Esta política no crea nuevas reglas de autorización de negocio. Los pares origen-destino autorizados siguen gobernados por DES-3, los roles y MSP por DES-6, y el contrato público del chaincode por DES-5.

## Consecuencias

- **Para chaincode**: las funciones que operen sobre información privada deberán usar las APIs de datos privados de Fabric. El ADR no define firmas, parámetros, errores ni payloads; eso corresponde a DES-5 y a las issues de implementación de chaincode.
- **Para red**: NET-5 deberá materializar esta decisión en configuración de colecciones y demostrar que una organización no autorizada no ve el dato privado pero sí conserva la evidencia hasheada, y que ANMAT accede a lo necesario para auditar.
- **Para endoso y membresía**: las políticas concretas de endoso, MSP, ABAC y roles quedan en DES-6 y NET-6. DES-4 solo establece la regla de visibilidad que esas políticas no deben contradecir.
- **Para evaluación**: la decisión introduce mayor complejidad que un canal único abierto y puede afectar latencia o throughput. La medición de ese costo queda para DES-7 y las issues de evaluación.
- **Para evolución del prototipo**: si DES-8 modifica la representación de establecimientos mediante GLN/CUFE dentro de una misma organización, NET-5 deberá verificar que la granularidad de PDC elegida siga evitando acceso indebido entre establecimientos.

## Contexto utilizado

- Referencia normativa `DISP_3683_2011_ART_9`: [Disposición ANMAT 3683/2011](https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/actualizacion), artículo 9 sobre seguridad, restricciones, alertas, conocimiento en tiempo real de ANMAT y restricción de acceso a transacciones ajenas.
- Documentación oficial de Hyperledger Fabric: [Channels](https://hyperledger-fabric.readthedocs.io/en/latest/channels.html) y [Private data](https://hyperledger-fabric.readthedocs.io/en/latest/private-data/private-data.html).
