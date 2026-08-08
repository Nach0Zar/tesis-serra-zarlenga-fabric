# ADR-003: Identidad de establecimientos mediante GLN/CUFE

- **Estado**: Aceptado (revisión 2)
- **Fecha**: 2026-08-08
- **Autores**: Serra, Zarlenga

---

## Contexto

La red Fabric del prototipo es permisionada: los actores participan mediante identidades emitidas por una autoridad de membresía y el chaincode aplica reglas determinísticas de autorización. La Disposición ANMAT 3683/2011 exige que ningún establecimiento acceda a información de operaciones de las que no forma parte, y que ANMAT conserve capacidad de auditoría sobre la totalidad de los eventos.

En Fabric, la unidad de confidencialidad real no es un atributo de identidad ni un campo cifrado: es el **peer**. Para que una transacción se confirme, los peers exigidos por la política de endoso deben leer los campos relevantes en claro para poder validar la lógica de negocio (¿el custodio declarado coincide con el actual? ¿el estado del producto permite la operación?). Un peer que procesa esa validación tiene, en ese momento, acceso al contenido en claro, más allá de cómo se almacene después. En consecuencia, garantizar que un establecimiento no acceda a información de otro requiere que cada establecimiento controle su propio límite organizacional dentro de la red: su propia MSP.

Esta conclusión reemplaza la premisa de la versión anterior de este ADR, que trataba a las MSP como categorías operativas compartidas por muchos establecimientos. Bajo esa premisa, ni el cifrado de campos en capa de aplicación ni un registro de establecimientos en el ledger resuelven el problema de fondo: el cifrado no impide que el peer que endosa la transacción procese el dato en claro, y hacerlo de otra forma saca la validación fuera de la cadena, degradando exactamente las propiedades (validación multiorganizacional determinística, auditabilidad completa de ANMAT) que justifican usar blockchain frente al modelo centralizado.

## Alternativas

**A. Categoría de actor como MSP compartida, con atributos X.509 por establecimiento)**

- Cada categoría de actor (`FarmaciaMSP`, `DrogueriaMSP`, etc.) es una única organización Fabric.
- Los certificados de enrolamiento individuales llevan atributos (`snt.establishment.id`, `snt.establishment.id_type`) que identifican al establecimiento dentro de la categoría.
- Un registro de establecimientos en el ledger resuelve existencia, habilitación y `agentType`.
- Se descarta porque la confidencialidad por establecimiento que exige la normativa no puede aplicarse por debajo del nivel de organización en Fabric. Cualquier peer de la MSP compartida puede recibir y procesar datos privados de operaciones ajenas al establecimiento que dice representar el atributo del certificado. El atributo distingue identidad para fines de autorización de escritura, pero no aísla acceso de lectura a nivel de plataforma.

**B. Organización Fabric por establecimiento**

- Cada establecimiento (identificado por GLN o CUFE) es su propia organización Fabric, con su propia MSP.
- La identidad de custodio se expresa directamente mediante `cid.GetMSPID()`, sin necesidad de atributos de certificado adicionales para resolver "quién es el establecimiento".
- El límite de confidencialidad de Private Data Collections y canales coincide exactamente con el límite de establecimiento que exige la normativa, sin brechas residuales.
- El costo de esta alternativa es de gobernanza y automatización, no de arquitectura: cada alta o baja de establecimiento requiere una transacción de actualización de configuración de canal, y las políticas que refieren a "cualquier actor de una categoría" deben enumerarse o generarse por herramientas en lugar de referenciar una única MSP.
- Se adopta.

**C. Cifrado de campos sensibles en capa de aplicación sobre MSP por categoría**

- Los campos comercialmente sensibles se cifran con una clave que solo poseen las partes involucradas antes de escribirse en una colección privada compartida por categoría.
- No resuelve la confidencialidad de los campos que el chaincode necesita leer en claro para validar la operación (custodio actual, estado del producto, tipo de agente), porque esos campos deben llegar en claro al peer que endosa.
- Introduce una dependencia frágil: la auditabilidad de ANMAT solo se preserva si se la incluye explícitamente como destinataria de cada sobre cifrado; omitirla en un caso rompe la garantía regulatoria sin que el chaincode pueda detectarlo.
- Se descarta como mecanismo de confidencialidad primario. No se descarta como complemento posible para datos que genuinamente no requieren validación cruzada entre organizaciones (por ejemplo, condiciones comerciales que ninguna regla de chaincode necesita evaluar), pero eso queda fuera del alcance de este ADR.

## Decisión

Se adopta el **modelo B: una organización Fabric (MSP) por establecimiento**, identificado por GLN o CUFE.

1. Cada establecimiento habilitado por el SNT (laboratorio, droguería, distribuidor/operador logístico, farmacia, centro médico) es una organización Fabric independiente, con su propia MSP.
2. La MSP administra sus propias identidades de cliente para el personal u operadores de ese establecimiento; la identidad de establecimiento a efectos de custodia es la organización, no el atributo de un certificado individual.
3. El ledger mantiene un registro liviano que traduce el identificador interno de organización (`mspId`) al identificador canónico de dominio (GLN o CUFE), junto con su categoría normativa (`agentType`) y su estado de habilitación (`active`).
4. El custodio persistido en los assets es el identificador canónico `GLN:<13 dígitos>` o `CUFE:<13 dígitos>`, resuelto a partir del `mspId` del invocador mediante el registro. El `mspId` no se persiste como custodio.

Se decide no acoplar el identificador de dominio (GLN/CUFE) al nombre interno de la MSP en la configuración de red. El nombre de la MSP es un detalle de configuración de Fabric; el registro en el ledger es la única fuente de verdad que vincula ese nombre con el identificador regulatorio. Esto permite operar renombrados o migraciones de configuración de red sin alterar el identificador de dominio que ya quedó persistido en el historial de custodia.

## Registro de organización-establecimiento

El ledger debe contener un registro mínimo por organización habilitada:

| Campo | Uso |
|---|---|
| `mspId` | Identificador de la organización Fabric del establecimiento. |
| `id` | GLN o CUFE del establecimiento. |
| `idType` | `GLN` o `CUFE`. |
| `agentType` | Categoría normativa del establecimiento. |
| `active` | Indica si el establecimiento puede operar. |

A diferencia de la versión anterior de este ADR, la relación entre `mspId` y `id` es uno a uno: cada organización representa exactamente un establecimiento. El registro ya no necesita resolver ambigüedad entre múltiples establecimientos de una misma MSP, porque esa ambigüedad no existe en este modelo.

El registro no debe incluir razón social, domicilio, CUIT, datos personales, datos clínicos ni información comercial no necesaria para la validación de custodia.

Las altas, bajas o cambios de `active` deben ser ejecutados por una identidad regulatoria o administrativa del prototipo (asociada a `AnmatMSP`), no por el establecimiento afectado. El alta de una nueva organización en el registro debe ser consistente con su alta en la configuración del canal: una organización que aparece en el registro como activa pero no fue incorporada a la configuración del canal no puede transaccionar, y viceversa, una organización incorporada al canal sin alta en el registro no debe poder operar como custodio válido. Un despliegue productivo requeriría automatizar ambos pasos como una única operación de onboarding disparada por el proceso de habilitación regulatoria de ANMAT, para evitar que queden desincronizados.

## Regla de validación en chaincode

Para operaciones donde el invocador actúa como custodio:

1. Obtener el MSP del invocador con `cid.GetMSPID()`.
2. Consultar el registro de organización-establecimiento por `mspId` y verificar que exista una entrada y que `active` sea verdadero.
3. Tomar `id`, `idType` y `agentType` desde el registro y construir el identificador canónico `<idType>:<id>`.
4. Verificar que `agentType` pertenezca al catálogo admitido.
5. Comparar el identificador canónico resuelto contra el custodio actual del asset.
6. Rechazar la transacción si el invocador no coincide con el custodio actual, si su organización no está registrada o si no está activa.

Para transferencias, el destino puede viajar como parámetro de request porque no representa la identidad del invocador. Antes de actualizar el custodio, el chaincode debe validar que el destino exista en el registro (por `mspId` o por identificador canónico, según cómo lo declare el cliente), esté activo y tenga un tipo de agente compatible con la matriz de transferencias autorizadas.

Este modelo elimina la necesidad de leer atributos de certificado (`cid.GetAttributeValue`) para resolver identidad de establecimiento, porque `cid.GetMSPID()` ya identifica unívocamente al establecimiento a través del registro. La suplantación de un establecimiento por otro deja de ser un caso que el chaincode deba validar con lógica propia: requeriría que el atacante posea una identidad válida emitida por la CA de una organización que no es la suya, lo cual está cubierto por las garantías criptográficas base de la membresía Fabric, no por una regla adicional de este ADR.

## Justificación

El modelo anterior separaba "quién invoca" (certificado) de "contra quién opera" (registro), pero ambos mecanismos operaban dentro de un límite de confidencialidad que ya había fallado un nivel más abajo: la organización compartida. Este modelo corrige eso alineando el límite de identidad de negocio (establecimiento) con el único límite de confidencialidad que Fabric puede aplicar sin degradar la validación multiorganizacional: la organización.

La decisión mantiene el principio de mínimo dato: el registro solo almacena los campos necesarios para autorización, traducción de identidad y trazabilidad técnica. La información sensible o no necesaria queda fuera del modelo.

El costo que introduce esta decisión es de gobernanza y automatización de red (alta de organizaciones, actualización de configuración de canal, mantenimiento de políticas que referencian categorías completas de actores), no de arquitectura de identidad. Ese costo es consistente con el propio proceso regulatorio: ANMAT ya exige auditar y autorizar a cada actor antes de que pueda operar, por lo que un onboarding gobernado (vía actualización de configuración de canal) es una representación más fiel del proceso real que un alta liviana de atributos.

## Límites de garantía

Este ADR define cómo vincular una organización Fabric con un establecimiento y cómo validar esa identidad contra el estado del ledger. El modelo acredita que una identidad autorizada de una organización específica declaró una operación y que esa operación respetó las reglas determinísticas disponibles para el chaincode.

El modelo no acredita por sí solo:

- posesión física efectiva del medicamento;
- autenticidad material del envase o del soporte de trazabilidad;
- ausencia de clonación del código serializado;
- cumplimiento de condiciones ambientales o cadena de frío;
- que el escaneo se haya realizado físicamente en el establecimiento declarado;
- exactitud del dato de entrada si el proceso externo de captura fue incorrecto;
- vigencia regulatoria real si el registro del prototipo no está sincronizado con la fuente externa;
- que la infraestructura de hosting de un peer (si se opera de forma centralizada o tercerizada para varios establecimientos) no acceda a datos en claro durante el procesamiento; esto depende de garantías operativas y contractuales fuera del alcance de este ADR.

Además, aun con el límite de confidencialidad correctamente ubicado en la organización, los identificadores GLN/CUFE y la secuencia de movimientos pueden revelar metadatos comerciales, relaciones entre establecimientos o patrones operativos frente a quienes sí son miembros legítimos de una colección o canal. Las decisiones de privacidad, canales y colecciones privadas se gobiernan en el ADR-002.

## Consecuencias

- El modelo de red debe representar cada establecimiento habilitado como una organización Fabric independiente, con su propia MSP.
- El modelo de asset debe persistir el custodio como GLN/CUFE canónico, resuelto desde el registro a partir del `mspId` del invocador, no como MSP ni como atributo de certificado.
- El contrato público del chaincode no requiere que el cliente envíe el GLN/CUFE del invocador en operaciones donde este actúa como custodio.
- Las transferencias deben recibir un destinatario identificable por GLN/CUFE o `mspId` y validarlo contra el registro de organización-establecimiento.
- La CA de la red debe soportar la emisión de identidades para un número creciente de organizaciones. Para el prototipo, Fabric CA sigue siendo la opción por defecto; una única instancia de CA puede emitir identidades para múltiples organizaciones, por lo que este cambio no exige una CA por establecimiento.
- El alta y baja de establecimientos deja de ser una operación exclusiva del ledger: requiere coordinar una actualización de configuración de canal (agregar o remover la organización) junto con el alta o baja en el registro de organización-establecimiento.
- Las políticas de endoso, transferencia y colecciones privadas que antes referenciaban una MSP por categoría (`FarmaciaMSP`) deben rediseñarse para referenciar organizaciones individuales o resolverse mediante generación programática de políticas a partir del registro, ya que Fabric no ofrece una forma nativa de referenciar "cualquier organización de esta categoría".
- Los tests de chaincode ya no necesitan casos de suplantación de establecimiento dentro de una misma organización (ese escenario no existe en este modelo); en su lugar deben cubrir: organización no registrada, organización registrada pero inactiva, y organización registrada pero con `agentType` incompatible con la operación.
- El diseño de despacho, recepción, eventos compensatorios, idempotencia y modelo histórico queda fuera de este ADR.
- La baja o inhabilitación de un establecimiento debe modelarse actualizando `active`, no alterando historiales de custodia ya persistidos, y no debe implicar necesariamente la remoción inmediata de la organización de la configuración del canal si aún es necesario preservar su capacidad de lectura para auditoría.
- Un despliegue productivo requiere automatizar el pipeline de onboarding (registro regulatorio → alta de organización en configuración de canal → alta en el registro de organización-establecimiento → actualización de políticas dependientes) como una única operación gobernada, y evaluar modelos de hosting de peers compartido para establecimientos que no operan infraestructura propia.

## Anexo: fuentes externas

| Fuente | Sección consultada | Uso en esta decisión |
|---|---|---|
| Disposición ANMAT 3683/2011, Argentina.gob.ar: https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/actualizacion | Artículos 2, 3, 5, 6 y 9 | Define actores del SNT, uso de estándares GS1, datos de distribución, implementación por establecimiento y restricción de acceso a transacciones ajenas. |
| Disposición ANMAT 963/2015, Argentina.gob.ar: https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-963-2015-241473/texto | Artículos 5 y 15 | Exige identificación de agentes mediante GLN o CUFE y documentación de origen/destino por establecimiento físico. |
| GS1 GLN: https://www.gs1.org/standards/id-keys/gln | Descripción de GLN | Fundamenta GLN como identificador de ubicaciones y partes. |
| GS1 check digit: https://www.gs1.org/services/how-calculate-check-digit-manually | Cálculo del dígito verificador | Fundamenta la validación de dígito verificador para identificadores GS1. |
| Hyperledger Fabric CA attributes: https://hyperledger-fabric-ca.readthedocs.io/en/latest/users-guide.html | Registro, enrolamiento, atributos y revocación | Fundamenta que una única CA puede emitir identidades para múltiples organizaciones. |
| Hyperledger Fabric channel configuration: https://hyperledger-fabric.readthedocs.io/en/latest/config_update.html | Actualización de configuración de canal | Fundamenta el costo de gobernanza de alta y baja de organizaciones en un canal. |
| Hyperledger Fabric private data: https://hyperledger-fabric.readthedocs.io/en/latest/private-data/private-data.html | Distribución de datos privados entre peers de organizaciones miembro | Fundamenta que la confidencialidad de una colección privada se resuelve a nivel de organización, no de identidad individual dentro de una organización. |
| Hyperledger Fabric chaincode `cid`: https://pkg.go.dev/github.com/hyperledger/fabric-chaincode-go/pkg/cid | `GetMSPID` | Fundamenta la extracción de la organización del invocador como base de la identidad de establecimiento. |
