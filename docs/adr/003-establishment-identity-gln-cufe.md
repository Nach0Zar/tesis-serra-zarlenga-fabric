# ADR-003: Identidad de establecimientos mediante GLN/CUFE

- **Estado**: Aceptado
- **Fecha**: 2026-08-01
- **Autores**: Serra, Zarlenga

---

## Contexto

La red Fabric del prototipo es permisionada: los actores participan mediante identidades emitidas por una autoridad de membresía y el chaincode aplica reglas determinísticas de autorización. Ese modelo permite representar organizaciones y roles, pero no alcanza por sí solo para distinguir establecimientos físicos individuales cuando varios establecimientos pertenecen a la misma categoría de actor.

El problema aparece en operaciones de custodia. Una organización como `FarmaciaMSP` puede representar a muchas farmacias distintas. Si el asset guardara solo la MSP como custodio, dos farmacias diferentes dentro de la misma MSP serían indistinguibles para el chaincode. La validación de que el invocador es el custodio actual necesita un identificador de establecimiento, no solo una organización Fabric.

En este prototipo, las MSP representan categorías operativas de la red y no cada establecimiento físico. Esa decisión permite simular el ámbito productivo con una red local acotada. No implica que un despliegue productivo deba usar una única MSP para todas las entidades legales de una categoría; en producción, la frontera de una MSP debería evaluarse como dominio administrativo y de confianza.

El identificador de establecimiento del SNT se modela con GLN o CUFE. La identidad Fabric debe vincular criptográficamente al invocador con ese identificador, y el ledger debe permitir validar que el establecimiento destinatario existe, corresponde al tipo de agente registrado y se encuentra habilitado para operar.

## Alternativas

**A. GLN/CUFE como atributo del certificado X.509**

- Cada identidad de cliente incluye atributos de establecimiento emitidos por la CA.
- El chaincode extrae los atributos desde el certificado del invocador.
- Evita que el cliente declare arbitrariamente su propio GLN/CUFE en el payload.
- No resuelve por sí sola la validación del destinatario: el receptor puede ser un establecimiento no registrado, inactivo o declarado con un tipo de agente incorrecto si no existe una fuente de verdad adicional.

**B. Una organización Fabric por establecimiento**

- Cada establecimiento tendría su propia MSP.
- La identidad de custodio podría expresarse directamente mediante MSP.
- Se rechaza para el prototipo porque no escala operativamente: exige administrar organizaciones, certificados, políticas, peers, colecciones y actualizaciones de configuración por cada establecimiento físico.
- También rompe el criterio de representar categorías de actores como organizaciones de red en una simulación local acotada.

**C. Registro de establecimientos como assets en ledger**

- El ledger mantiene un registro mínimo de establecimientos habilitados.
- El chaincode puede validar tipo de agente, MSP asociada y estado activo del destinatario.
- No resuelve por sí solo la suplantación del invocador si el establecimiento origen viaja como parámetro de request.
- Funciona como complemento de atributos X.509, no como reemplazo.

## Decisión

Se adopta un **modelo híbrido**:

1. La identidad del invocador se toma del certificado X.509 emitido por la CA.
2. El certificado del cliente debe incluir atributos de identificador de establecimiento.
3. El chaincode debe extraer esos atributos con la librería `cid`.
4. El ledger debe mantener un registro mínimo de establecimientos para validar destinatarios, habilitación y coherencia entre establecimiento, tipo de agente y MSP.

Las MSP representan categorías operativas del prototipo, no establecimientos físicos individuales. El custodio persistido en los assets debe ser un identificador canónico de establecimiento:

```text
GLN:<13 dígitos>
CUFE:<13 dígitos>
```

El MSP no debe usarse como custodio del asset.

## Contrato de identidad

Las identidades de cliente que operan sobre custodia ordinaria deben incluir estos atributos en el certificado de enrolamiento:

| Atributo | Valores admitidos | Uso |
|---|---|---|
| `snt.establishment.id` | 13 dígitos numéricos | Identificador del establecimiento invocador. |
| `snt.establishment.id_type` | `GLN`, `CUFE` | Tipo de identificador del establecimiento. |

Estos atributos deben emitirse mediante el mecanismo de atributos reconocido por la librería de identidad de Fabric utilizada por el chaincode. No se consideran equivalentes los valores cargados en `CN`, `OU`, `Subject` u otras extensiones arbitrarias si `cid.GetAttributeValue` no puede leerlos con los nombres definidos en este contrato. Una CA externa solo es compatible si reproduce ese mecanismo de atributos, o si se implementa y audita explícitamente un extractor alternativo.

El certificado no incluye `agentType`. La categoría normativa del establecimiento es autoritativa en el registro del ledger para evitar duplicación, certificados obsoletos y divergencias ante cambios de habilitación o categoría.

Las identidades regulatorias, administrativas o de consulta no son custodios ordinarios de medicamentos. Sus permisos se definen fuera de este contrato de establecimiento custodio.

## Registro mínimo de establecimientos

El ledger debe contener un registro mínimo por establecimiento habilitado:

| Campo | Uso |
|---|---|
| `id` | GLN o CUFE del establecimiento. |
| `idType` | `GLN` o `CUFE`. |
| `agentType` | Categoría normativa del establecimiento. |
| `mspId` | MSP autorizada a emitir identidades para ese establecimiento. |
| `active` | Indica si el establecimiento puede operar. |

El registro no debe incluir razón social, domicilio, CUIT, datos personales, datos clínicos ni información comercial no necesaria para la validación de custodia.

En el prototipo, este registro funciona como una fuente regulatoria simulada y controlada. Las altas, bajas o cambios deben ser ejecutados por una identidad regulatoria o administrativa del prototipo, no por el establecimiento afectado. Un despliegue productivo requeriría gobierno formal del alta, validación contra fuentes regulatorias externas, auditoría de cambios, separación de funciones y manejo de errores de sincronización.

## Regla de validación en chaincode

Para operaciones donde el invocador actúa como custodio:

1. Obtener el MSP del invocador con `cid.GetMSPID`.
2. Obtener `snt.establishment.id` y `snt.establishment.id_type` con `cid.GetAttributeValue`.
3. Rechazar la transacción si falta algún atributo, si el identificador no tiene 13 dígitos, si el tipo no es `GLN` o `CUFE`, o si el identificador no supera las validaciones del tipo correspondiente.
4. Para `GLN`, validar el dígito verificador GS1 además del largo y carácter numérico.
5. Para `CUFE`, validar largo, carácter numérico y existencia en el registro de establecimientos; no se define una regla de dígito verificador adicional sin fuente normativa específica.
6. Construir el identificador canónico del invocador como `<idType>:<id>`.
7. Consultar el registro de establecimientos y verificar que el establecimiento exista, esté activo y tenga `mspId` coherente con la identidad invocante.
8. Tomar `agentType` desde el registro del ledger y verificar que pertenezca al catálogo admitido.
9. Comparar el identificador canónico del invocador contra el custodio actual del asset.
10. Rechazar la transacción si el invocador no coincide con el custodio actual.

Para transferencias, el destino puede viajar como parámetro de request porque no representa la identidad del invocador. Antes de actualizar el custodio, el chaincode debe validar que el destino exista en el registro de establecimientos, esté activo y tenga un tipo de agente compatible con la matriz de transferencias autorizadas.

La inhabilitación de un establecimiento (`active=false`) y la revocación de un certificado son controles distintos. Una operación autorizada requiere simultáneamente una identidad válida según la MSP, un certificado no revocado, atributos de establecimiento presentes y bien formados, establecimiento existente, establecimiento activo y vínculo coherente entre establecimiento y MSP.

## Justificación

El modelo híbrido separa dos problemas distintos:

- **Quién invoca**: se resuelve con la identidad criptográfica del certificado.
- **Contra quién opera**: se resuelve con el registro mínimo de establecimientos.

Esta separación evita la suplantación del custodio actual, porque el GLN/CUFE del invocador no se acepta desde el payload. También permite validar destinatarios sin crear una organización Fabric por cada establecimiento físico.

La decisión mantiene el principio de mínimo dato: el ledger solo almacena los campos necesarios para autorización y trazabilidad técnica. La información sensible o no necesaria queda fuera del modelo.

## Límites de garantía

Este ADR define cómo vincular una identidad Fabric con un establecimiento y cómo validar esa identidad contra el estado del ledger. El modelo acredita que una identidad autorizada declaró una operación y que esa operación respetó las reglas determinísticas disponibles para el chaincode.

El modelo no acredita por sí solo:

- posesión física efectiva del medicamento;
- autenticidad material del envase o del soporte de trazabilidad;
- ausencia de clonación del código serializado;
- cumplimiento de condiciones ambientales o cadena de frío;
- que el escaneo se haya realizado físicamente en el establecimiento declarado;
- exactitud del dato de entrada si el proceso externo de captura fue incorrecto;
- vigencia regulatoria real si el registro del prototipo no está sincronizado con la fuente externa.

Además, aun minimizando datos, los identificadores GLN/CUFE y la secuencia de movimientos pueden revelar metadatos comerciales, relaciones entre establecimientos o patrones operativos. Las decisiones de privacidad, canales y colecciones privadas se gobiernan fuera de este ADR.

## Consecuencias

- El modelo de asset debe persistir el custodio como GLN/CUFE canónico, no como MSP.
- El contrato público no debe requerir que el cliente envíe el GLN/CUFE del invocador en operaciones donde este actúa como custodio.
- Las transferencias deben recibir un destinatario identificable por GLN/CUFE y validarlo contra el registro de establecimientos.
- La red debe emitir certificados con atributos de establecimiento legibles por `cid.GetAttributeValue`. Para el prototipo, Fabric CA es la opción por defecto porque soporta atributos en certificados de enrolamiento.
- `agentType` se obtiene desde el registro de establecimientos, no desde el certificado.
- Los tests de chaincode deben incluir casos de suplantación: un cliente con un GLN/CUFE intenta operar como otro establecimiento y la transacción es rechazada.
- Los tests de chaincode deben distinguir establecimiento inactivo de certificado revocado cuando exista soporte de red para revocación.
- El diseño de despacho, recepción, eventos compensatorios, idempotencia y modelo histórico queda fuera de este ADR.
- La baja o inhabilitación de un establecimiento debe modelarse actualizando `active`, no alterando historiales de custodia ya persistidos.

## Anexo: fuentes externas

| Fuente | Sección consultada | Uso en esta decisión |
|---|---|---|
| Disposición ANMAT 3683/2011, Argentina.gob.ar: https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/actualizacion | Artículos 2, 3, 5 y 6 | Define actores del SNT, uso de estándares GS1, datos de distribución e implementación por establecimiento. |
| Disposición ANMAT 963/2015, Argentina.gob.ar: https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-963-2015-241473/texto | Artículos 5 y 15 | Exige identificación de agentes mediante GLN o CUFE y documentación de origen/destino por establecimiento físico. |
| GS1 GLN: https://www.gs1.org/standards/id-keys/gln | Descripción de GLN | Fundamenta GLN como identificador de ubicaciones y partes. |
| GS1 check digit: https://www.gs1.org/services/how-calculate-check-digit-manually | Cálculo del dígito verificador | Fundamenta la validación de dígito verificador para identificadores GS1. |
| Hyperledger Fabric CA attributes: https://hyperledger-fabric-ca.readthedocs.io/en/latest/users-guide.html | Registro, enrolamiento, atributos y revocación | Fundamenta la emisión y revocación de identidades con atributos en certificados de enrolamiento. |
| Hyperledger Fabric chaincode `cid`: https://pkg.go.dev/github.com/hyperledger/fabric-chaincode-go/pkg/cid | `GetMSPID`, `GetAttributeValue` | Fundamenta la extracción de MSP y atributos desde la identidad del invocador. |
