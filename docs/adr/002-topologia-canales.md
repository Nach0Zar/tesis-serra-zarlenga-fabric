# ADR-002: Topología de canales en la red Hyperledger Fabric

- **Estado**: Aceptado (revisión 3)
- **Fecha**: 2026-08-08
- **Autores**: Serra, Zarlenga

---

## Contexto

El prototipo del PFI busca representar procesos core del Sistema Nacional de Trazabilidad de Medicamentos (SNT) sobre una red blockchain permisionada basada en Hyperledger Fabric. El alcance documentado en la tesis se limita al flujo downstream regulado por ANMAT e incluye registro de lotes, transferencias entre agentes y dispensación, con evaluación frente a una línea base centralizada.

La arquitectura del SNT vigente opera bajo administración centralizada de ANMAT. La Disposición ANMAT 3683/2011 exige que el sistema permita a la autoridad tomar conocimiento en tiempo real de irregularidades, anomalías o desviaciones, y que ningún establecimiento acceda a información de la cadena de distribución correspondiente a transacciones de otros establecimientos de las que no forme parte. Por lo tanto, la topología de Fabric debe resolver tres requisitos en tensión:

1. **Visibilidad regulatoria**: ANMAT debe poder auditar la trazabilidad relevante de las unidades alcanzadas por el prototipo.
2. **Confidencialidad por establecimiento**: cada establecimiento no debe observar información comercial o documental de operaciones en las que no participa.
3. **Validación independiente por futuros participantes**: cualquier organización que sea un destinatario legítimo posible de una transferencia (según la matriz de transferencias autorizadas de DES-3) debe poder verificar el estado actual de un medicamento antes de aceptar su custodia, sin depender exclusivamente de la palabra del custodio actual ni de una consulta a ANMAT en tiempo real.

El tercer requisito no está en el paper de forma explícita, pero se deriva de cómo Fabric valida transacciones: la garantía de integridad del modelo depende de que múltiples organizaciones puedan ejecutar la misma lógica de validación de forma determinística e independiente. Si el estado necesario para esa validación no es accesible para quien tiene que validarlo, esa organización solo puede confiar en el endoso de terceros en lugar de verificar por sí misma — lo cual debilita la propiedad de integridad que el paper usa para justificar blockchain frente al modelo centralizado.

El paper del proyecto identifica la topología de canales como una decisión de diseño y considera tres enfoques: canal único, múltiples canales y canal único con colecciones de datos privados. También indica que las PDC permiten compartir datos solo con un subconjunto autorizado sin crear un canal separado, manteniendo en el ledger común un hash verificable de la información privada, y enmarca explícitamente esa necesidad de privacidad como **confidencialidad comercial frente a competidores**, no como ocultamiento del hecho mismo de la trazabilidad.

Un punto que este ADR asume explícitamente, resuelto en ADR-003: la unidad mínima de confidencialidad que Fabric puede aplicar de forma nativa, tanto en la membresía de un canal como en la membresía de una colección privada, es la organización (MSP). No existe mecanismo nativo para restringir lectura por debajo de ese nivel dentro de una misma organización. Por eso ADR-003 decide representar cada establecimiento (GLN o CUFE) como su propia organización Fabric, en lugar de agrupar establecimientos de una misma categoría bajo una MSP compartida. Esta topología de canales se diseña asumiendo esa granularidad.

## Interpretación normativa y clasificación de datos

### La tensión

El artículo 9 de la Disposición ANMAT 3683/2011 exige que "ningún establecimiento acceda a información de la cadena de distribución correspondiente a transacciones de otros establecimientos de las que no forma parte". Leído sin matices, esto podría exigir que absolutamente todo dato asociado a una transacción —incluido el estado actual del medicamento— quede restringido a las organizaciones que fueron parte de la última operación sobre esa unidad.

Aplicar esa lectura de forma estricta genera un problema funcional, no solo de diseño: cuando una Droguería transfiere un medicamento a una Farmacia con la que nunca operó antes, esa Farmacia necesita poder verificar, antes de aceptar la custodia, que el producto no está vencido, robado, en cuarentena o retirado del mercado. Si esa información quedó restringida a las partes de transacciones anteriores en las que la Farmacia no participó, la Farmacia no tiene forma de validarlo por sí misma — solo puede confiar en que la Droguería, el custodio saliente, declaró la verdad. Eso traslada la garantía de integridad desde "cualquier organización relevante puede verificar de forma independiente" hacia "hay que confiar en el endoso de la contraparte saliente", que es un modelo de confianza más débil y menos defendible como mejora frente al sistema centralizado actual.

### Interpretación adoptada

Se adopta una lectura que distingue dos categorías de datos:

1. **Estado mínimo de trazabilidad** (identificador del medicamento, lote, fecha de vencimiento, custodio actual, estado del producto): se interpreta que este conjunto de datos no constituye, por sí solo, "información de una transacción" en el sentido competitivo que el artículo 9 busca proteger. Es el dato regulatorio mínimo que el propio SNT centralizado ya recolecta y que cualquier eslabón autorizado de la cadena necesita para cumplir su obligación normativa de verificar la legitimidad de lo que recibe. Este conjunto tiene visibilidad amplia dentro del canal: cualquier organización miembro del canal puede leerlo.
2. **Información comercial y documental** (precio, condiciones comerciales, cantidades negociadas, número de factura o remito, y cualquier dato que permita inferir relaciones o volúmenes comerciales entre partes específicas): esta es la información que el paper describe como objeto de "confidencialidad comercial frente a competidores". Se restringe exclusivamente a las organizaciones participantes de la operación más `AnmatMSP`, mediante Private Data Collections u otro mecanismo de datos privados.

Esta interpretación se apoya en el propio paper del proyecto, que justifica las PDC explícitamente en términos de confidencialidad comercial, y en la necesidad funcional de que cualquier destinatario legítimo pueda validar de forma independiente antes de aceptar una custodia — una capacidad que la arquitectura pierde si se aplica la lectura estricta del artículo 9 a todo el estado del medicamento.

### Lo que se descarta explícitamente

Se consideró la alternativa de restringir también el estado mínimo de trazabilidad a las partes de cada transacción (equivalente a la Opción B discutida en el diseño: todo dato, estructural incluido, restringido a las partes reales más ANMAT). Se descarta como decisión por defecto porque:

- Obliga a que la validación de un nuevo destinatario dependa del endoso de la contraparte saliente en lugar de una verificación propia contra el ledger, debilitando la garantía de integridad multiorganizacional.
- No puede resolverse haciendo de ANMAT un intermediario de validación en tiempo real: eso reintroduciría un punto único de falla y un cuello de botella de throughput, y requeriría que el chaincode consulte un sistema externo durante su ejecución, lo cual rompe el determinismo que exige el modelo de endoso de Fabric.

### Advertencia sobre esta interpretación

Esta es una interpretación, no un hecho normativo incuestionable. La Disposición no distingue explícitamente "dato estructural" de "dato comercial". La tesis debe documentar esta interpretación como una decisión de diseño consciente, con su justificación funcional y su respaldo en el propio marco teórico del proyecto, y debe quedar disponible como punto de discusión frente al tutor o a una eventual auditoría regulatoria real, donde ANMAT podría exigir una lectura más estricta.

## Alternativas

### A. Canal único compartido por todos los actores, sin distinción de datos

- Todas las organizaciones participan de un mismo canal y mantienen un ledger común, sin ningún dato restringido mediante colecciones privadas.
- ANMAT conserva máxima visibilidad operacional porque toda transacción queda en el mismo canal.
- La complejidad operativa inicial es baja: un canal, un chaincode desplegado sobre ese canal y una superficie de administración reducida.
- Se descarta porque, aun aceptando que el estado mínimo de trazabilidad puede tener visibilidad amplia, la información comercial y documental de cada operación seguiría expuesta a organizaciones ajenas a ella. Esa exposición contradice el requisito de confidencialidad comercial que el propio paper identifica como necesario.

### B. Múltiples canales

- Cada subconjunto de organizaciones con necesidad de privacidad opera en un canal separado.
- La confidencialidad entre subconjuntos mejora porque cada canal tiene su propio ledger y sus propios miembros.
- La auditoría regulatoria se fragmenta: ANMAT tendría que integrarse a todos los canales relevantes o recibir información por mecanismos externos al ledger donde ocurre la operación.
- Con una organización por establecimiento, esta alternativa empeora en lugar de mejorar: aislar cada relación comercial en su propio canal implicaría un canal por cada par de establecimientos con relación de custodia, lo que crece combinatoriamente con la cantidad de establecimientos y no es una base administrable ni siquiera como referencia de diseño para producción.
- Además, un canal separado también fragmentaría el estado mínimo de trazabilidad, lo que reintroduce el problema de que un futuro destinatario no pueda validar de forma independiente si nunca fue miembro del canal donde se registraron los movimientos previos del medicamento.
- Se descarta porque maximiza el aislamiento a costa de complejidad operativa, de una auditoría menos simple de defender y de la capacidad de validación independiente de futuros destinatarios.

### C. Canal único, con estado mínimo de trazabilidad público dentro del canal y Private Data Collections para información comercial

- Todas las organizaciones del prototipo comparten un canal común.
- El estado mínimo de trazabilidad del medicamento (identificador, lote, vencimiento, custodio actual, estado del producto) se escribe directamente en el ledger compartido del canal, legible por cualquier organización miembro.
- La información comercial y documental de cada operación se almacena en colecciones privadas accesibles solo por las organizaciones participantes de esa operación más `AnmatMSP`. Las organizaciones no autorizadas conservan en el ledger común el hash que permite verificar integridad, sin acceso al contenido.
- Con una organización por establecimiento, la membresía de una colección puede listar exactamente a las organizaciones de los establecimientos involucrados en la operación más `AnmatMSP`, sin incluir por extensión a otros establecimientos no participantes.
- La complejidad es media: requiere distinguir en el modelo de datos qué campos son de estado público y cuáles son privados, diseñar colecciones y políticas de membresía para estos últimos, y un mecanismo para generar esa membresía a partir del registro de organización-establecimiento de ADR-003.

## Decisión

Se adopta la alternativa **C**.

El prototipo tendrá un canal común para los participantes de la red Fabric. El modelo de datos de cada asset se divide en dos partes:

- **Estado público del canal**: GTIN, número de serie, lote, fecha de vencimiento, custodio actual (identificador canónico GLN/CUFE) y estado del producto. Se escribe y lee mediante las operaciones estándar del world state del canal, sin PDC. Cualquier organización miembro del canal puede leerlo y validar contra él antes de aceptar una operación.
- **Datos privados de la operación**: condiciones comerciales, cantidades negociadas, número de factura o remito, y cualquier otro dato que permita inferir relaciones o volúmenes comerciales entre partes específicas. Se almacenan en Private Data Collections. La regla de membresía es:
  - miembros privados: las organizaciones de los establecimientos participantes directos de la operación;
  - miembro auditor: `AnmatMSP`, siempre que la información sea necesaria para fiscalización regulatoria;
  - no miembros: cualquier otra organización, que solo conserva la evidencia hasheada disponible en el ledger común.

Esta decisión no fija nombres definitivos de colecciones, el esquema exacto de campos públicos y privados, ni archivos `collections_config.json`. Esos detalles pertenecen a NET-5 y DES-5, y deben implementarse respetando esta regla de visibilidad y la clasificación de datos de la sección anterior.

## Justificación

La alternativa elegida es la que mejor concilia los tres requisitos en tensión. Un canal único sin ninguna distinción de datos favorece la auditoría y la validación independiente, pero no satisface la confidencialidad comercial requerida por la Disposición ANMAT 3683/2011 según la interpretación adoptada. Múltiples canales favorecen el aislamiento, pero fragmentan tanto la auditoría regulatoria como la capacidad de validación independiente de futuros destinatarios, y bajo el modelo de una organización por establecimiento escalan peor que las PDC.

Hyperledger Fabric documenta que los canales sirven cuando transacciones y ledgers completos deben mantenerse confidenciales dentro de un conjunto de organizaciones, mientras que las colecciones privadas son adecuadas cuando las transacciones deben compartirse en un canal pero solo un subconjunto debe acceder a determinados datos. Separar el estado mínimo de trazabilidad (público dentro del canal) de la información comercial (en PDC) usa cada mecanismo para lo que fue diseñado: el canal preserva la propiedad de validación multiorganizacional independiente que justifica usar blockchain, y las PDC resuelven específicamente la confidencialidad comercial que el paper identifica como necesaria.

La decisión también conserva coherencia con el paper y la tesis del proyecto: ambos enmarcan Fabric como red privada y permisionada, con canales, MSP, chaincode y mecanismos de privacidad como herramientas para representar reglas regulatorias sin adoptar una red pública ni una base centralizada tradicional.

## Colecciones privadas

Las PDC se definen en este ADR como mecanismo de privacidad selectiva dentro del canal común, aplicable exclusivamente a la información comercial y documental clasificada en la sección de interpretación normativa. El estado mínimo de trazabilidad no pasa por PDC.

Política de diseño para NET-5:

- Cada dato privado debe pertenecer a una colección cuya membresía incluya solo a las organizaciones (establecimientos) con necesidad legítima de lectura de esa operación puntual.
- `AnmatMSP` debe incluirse cuando el dato privado sea necesario para auditar irregularidades, desviaciones o cumplimiento regulatorio.
- Ninguna organización sin participación directa en la operación debe ser miembro de la colección por conveniencia operativa.
- La evidencia pública del canal debe limitarse al estado mínimo de trazabilidad y a lo que Fabric registra para validación e integridad, incluyendo hashes de datos privados, sin usar el canal común para replicar contenido comercial.
- Dado que el número de organizaciones crece con el número de establecimientos (ADR-003), la membresía de las colecciones no debe mantenerse a mano por operación. NET-5 debe evaluar mecanismos como colecciones implícitas por organización (con el dato replicado hacia cada parte involucrada más ANMAT) o datos transitorios con solo el hash persistido, además de colecciones explícitas multi-organización, y definir cómo se genera esa membresía de forma programática a partir del registro de organización-establecimiento y de la matriz de transferencias autorizadas de DES-3.

  **Actualización posterior — la evaluación ya se hizo**: [ADR-006](006-private-data-collections.md) decidió **colecciones explícitas por par de organizaciones** con membresía {org A, org B, `AnmatMSP`}, descartando las implícitas (rompen el despacho unilateral del emisor y la lectura conjunta que exige el endoso de ADR-004) y los datos transitorios (no dejan registro histórico auditable). La generación programática toma como entrada un **manifiesto de organizaciones versionado en el repositorio**, y no el registro del ledger: ese registro se siembra después del despliegue, de modo que tomarlo como entrada crearía una dependencia circular con la secuencia de bootstrap de ADR-007. Lo que sigue abierto para NET-5 (#24) es implementar la herramienta generadora, no elegir el mecanismo.
- ~~La granularidad exacta de colecciones debe definirse en NET-5~~ — la fijó [ADR-006](006-private-data-collections.md): **una colección por par de organizaciones** entre las que la matriz de DES-3 autorice una transferencia en alguna dirección, con membresía {org A, org B, `AnmatMSP`} y nombre `transfer_<mspIdA>_<mspIdB>` ordenado lexicográficamente. Lo que NET-5 (#24) implementa es la herramienta que la genera.

Esta política no crea nuevas reglas de autorización de negocio. Los pares origen-destino autorizados siguen gobernados por DES-3, los roles y organizaciones por DES-6, y el contrato público del chaincode por DES-5.

## Consecuencias

- **Para el modelo de datos**: el asset de cada medicamento debe separar explícitamente campos de estado público (canal) de campos privados (PDC). Esa separación exacta de campos es responsabilidad de DES-5/DES-6, a partir de la clasificación de esta sección.
- **Para chaincode**: las funciones que lean o escriban estado mínimo de trazabilidad operan sobre el world state estándar del canal. Las funciones que operen sobre información comercial deberán usar las APIs de datos privados de Fabric. El ADR no define firmas, parámetros, errores ni payloads; eso corresponde a DES-5 y a las issues de implementación de chaincode.
- **Para red**: NET-5 deberá materializar esta decisión en configuración de colecciones y demostrar que una organización no autorizada no ve el dato comercial privado pero sí conserva la evidencia hasheada, que cualquier organización puede validar el estado mínimo de trazabilidad de forma independiente, y que ANMAT accede a lo necesario para auditar. Con una organización por establecimiento, NET-5 también deberá resolver cómo se genera y mantiene la membresía de colecciones privadas a medida que se incorporan nuevas organizaciones a la red.
- **Para endoso y membresía**: las políticas concretas de endoso, MSP, ABAC y roles quedan en DES-6 y NET-6. DES-4 solo establece la regla de visibilidad que esas políticas no deben contradecir.
- **Para evaluación**: la decisión introduce mayor complejidad que un canal único sin distinción de datos y puede afectar latencia o throughput, principalmente por el costo de escritura y propagación de datos privados. La medición de ese costo queda para DES-7 y las issues de evaluación.
- **Para la tesis**: la interpretación normativa adoptada en este ADR (estado mínimo de trazabilidad no restringido por establecimiento, información comercial sí) debe documentarse explícitamente como decisión de diseño consciente y discutible, no como lectura unívoca de la Disposición ANMAT 3683/2011.
- **Para evolución del prototipo**: la limitación que quedaba pendiente en la revisión anterior de este ADR (verificar que la granularidad de PDC evite acceso indebido entre establecimientos) sigue resuelta por diseño en ADR-003. La tarea pendiente para NET-5 es de **automatización**, no de brecha de confidencialidad en el estado mínimo de trazabilidad; la elección de mecanismo dejó de estar abierta cuando ADR-006 adoptó las colecciones explícitas por par. ADR-006 declara además un límite que este ADR no había advertido: el **nombre** de la colección viaja en claro en el read-write set del bloque, de modo que un observador del canal infiere que las organizaciones A y B operaron entre sí aunque no lea el contenido — el aislamiento cubre el dato, no el metadato de relación.

## Contexto utilizado

- Referencia normativa `DISP_3683_2011_ART_9`: [Disposición ANMAT 3683/2011](https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/actualizacion), artículo 9 sobre seguridad, restricciones, alertas, conocimiento en tiempo real de ANMAT y restricción de acceso a transacciones ajenas. La interpretación de alcance de "información" en este artículo se documenta en la sección "Interpretación normativa y clasificación de datos" de este ADR.
- Documentación oficial de Hyperledger Fabric: [Channels](https://hyperledger-fabric.readthedocs.io/en/latest/channels.html) y [Private data](https://hyperledger-fabric.readthedocs.io/en/latest/private-data/private-data.html).
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md), revisión 2: define el modelo de una organización Fabric por establecimiento que esta topología de canales asume.
