# ADR-010: Identidad de las organizaciones no custodiales (autoridad regulatoria y financiadores)

- **Estado**: Propuesto
- **Fecha**: 2026-08-17
- **Autores**: Serra, Zarlenga

---

## Contexto

El registro organización-establecimiento de ADR-003 es la única fuente de verdad que vincula el `mspId` de una organización Fabric con su identidad de dominio (GLN/CUFE, `agentType`, `active`). Pero ese registro solo cataloga establecimientos custodiales: los 6 `agentType` de la matriz DES-3. Las dos organizaciones no custodiales del diseño — `AnmatMSP` (autoridad regulatoria) y `FinanciadorMSP` (organismo financiador) — quedan fuera del registro (DES-6: "las organizaciones no custodiales no deben resolverse como custodios de assets"), y ningún documento define **cómo el chaincode reconoce** que un invocador es la autoridad regulatoria o un financiador.

Sin esa regla, no son implementables de forma determinística:

- las operaciones `REGULATORY_ONLY` del contrato DES-5 (`ProhibitProduct`, `RegisterOrganization`, `SetOrganizationActive`), que el propio contrato define como "solo `AnmatMSP` con `snt.role=regulatory-admin`" sin decir cómo se verifica esa condición sin comparar contra un literal;
- el coendoso regulatorio de DES-6 (retiros iniciados por laboratorio no custodio, eventos regulatorios);
- las consultas de verificación de trazabilidad del financiador (ADR-005 / CC-8), cuya autorización DES-6 asocia al rol `financier-auditor` de `FinanciadorMSP`.

La tensión de fondo la fijó ADR-003: el chaincode no debe acoplar identidad de dominio al nombre interno de la MSP, porque el nombre de la MSP es un detalle de configuración de red y el registro en el ledger es la única fuente de verdad de identidad. Hardcodear `"AnmatMSP"` en el chaincode funciona en el prototipo, pero contradice ese principio y no responde a la segunda pregunta abierta: el relevamiento del proyecto habla de PAMI, obras sociales y prepagas **en plural**, mientras DES-6 modela una sola `FinanciadorMSP`. El hallazgo E5 de la revisión de congruencia (`docs/consistency-review.md`) documenta esta brecha y la issue #85 (DES-16) la deriva a esta decisión, que debe quedar escrita antes de CC-1.

Extender el registro a las organizaciones no custodiales introduce, además, un problema de arranque que esta ADR debe resolver: si registrar una organización (`RegisterOrganization`) exige ser la autoridad regulatoria, y ser la autoridad regulatoria se acredita con una entrada del registro, ¿quién registra la primera entrada?

## Alternativas

**A. MSP ID por convención o hardcode en el chaincode**

- El chaincode compara `cid.GetMSPID()` contra el literal `"AnmatMSP"` (y `"FinanciadorMSP"`) fijado en el código o en una constante de despliegue.
- Es la solución más simple de implementar y no toca el modelo de datos del registro.
- Contradice el principio de ADR-003: acopla la identidad de dominio (quién es la autoridad de aplicación) a un nombre de configuración de red que puede renombrarse o migrarse, y convierte al código del chaincode en fuente de verdad de identidad, duplicando la que el registro ya provee para los custodiales.
- No escala a múltiples financiadores: cada financiador nuevo exigiría recompilar o reconfigurar el chaincode en lugar de una operación de alta gobernada.
- Se descarta porque contradice el principio de ADR-003 de no acoplar identidad de dominio a nombres de configuración de red y no escala a múltiples financiadores.

**B. Registro organización-establecimiento extendido con `agentType` no custodiales**

- El registro de ADR-003 se extiende con dos `agentType` nuevos (`REGULATOR`, `FINANCIER`) y un `idType` propio (`REG`), de modo que **todas** las organizaciones de la red — custodiales y no custodiales — se resuelven por el mismo camino: `cid.GetMSPID()` → entrada del registro → `agentType`.
- Mantiene el registro como única fuente de verdad de identidad y reutiliza la maquinaria existente (`RegisterOrganization`, `SetOrganizationActive`, `active`) para el ciclo de vida de los no custodiales.
- Responde naturalmente a múltiples financiadores: cada uno es una entrada más del registro.
- Requiere resolver el bootstrap de la primera entrada regulatoria (ver Decisión, punto 4) y validar estructuralmente que los `agentType` no custodiales nunca operen como custodios.
- Se adopta.

**C. Parámetro de instanciación del chaincode para cada organización no custodial**

- Los `mspId` de la autoridad regulatoria y de cada financiador se pasan como parámetros de la inicialización del chaincode, aprobados por las organizaciones del canal en el lifecycle.
- Evita el hardcode en código y hace explícita la aprobación multiorganizacional de esas identidades.
- Como mecanismo general, duplica la fuente de verdad que el registro ya provee: el estado de habilitación y la identidad de los financiadores vivirían en la definición del chaincode además del ledger, y cada alta o baja de un financiador exigiría una nueva secuencia de aprobación y commit de definición de chaincode en lugar de una transacción de alta.
- Se descarta como mecanismo general. Se conserva únicamente para el caso que ningún otro mecanismo puede resolver: el bootstrap de la entrada de la autoridad regulatoria (ver Decisión, punto 4).

## Decisión

Se adopta la **alternativa B**: el registro organización-establecimiento se extiende a las organizaciones no custodiales y pasa a ser la única fuente de verdad de identidad para **todas** las organizaciones de la red.

1. **Extensión del catálogo del registro.** Se agregan dos `agentType` nuevos: `REGULATOR` (autoridad de aplicación) y `FINANCIER` (organismo financiador), y un `idType` nuevo: `REG`, cuyo `id` es un slug estable del organismo (`ANMAT`, `INSSJP-PAMI`, etc.). El principio de ADR-003 se conserva intacto: el chaincode nunca compara contra literales de MSP; toda resolución de identidad — custodial o no — sigue el mismo camino `cid.GetMSPID()` → entrada del registro → `agentType`.
2. **Autorización derivada del registro.** Las operaciones `REGULATORY_ONLY` del contrato exigen que el invocador resuelva a una entrada con `agentType=REGULATOR`, `active=true` y atributo `snt.role=regulatory-admin`. Las consultas de verificación del financiador (CC-8) exigen `agentType=FINANCIER`, `active=true` y `snt.role=financier-auditor`. Los `agentType` no custodiales **nunca** son origen ni destino válidos de la matriz DES-3 — que conserva exactamente sus 6 tipos custodiales; la matriz **no** se modifica por esta ADR — y **nunca** pueden persistirse como `CustodioActual`; el chaincode valida ambas restricciones estructuralmente (rechaza cualquier transferencia, despacho, recepción o dispensación cuyo origen o destino resuelva a un `agentType` no custodial).
3. **Múltiples financiadores, soportados de forma nativa.** Cada organismo financiador es una organización Fabric con su propia entrada `FINANCIER` en el registro, dada de alta por la autoridad regulatoria por el mismo camino que un establecimiento: `RegisterOrganization` más la incorporación de la organización a la configuración del canal. El relevamiento del proyecto habla de PAMI, obras sociales y prepagas en plural; `FinanciadorMSP` de DES-6 pasa a ser el **ejemplo del dataset mínimo**, no un singleton del diseño.
4. **Bootstrap de la entrada del regulador.** Extender el registro a los no custodiales crea un problema de huevo y gallina: `RegisterOrganization` exige `agentType=REGULATOR`, pero la primera entrada `REGULATOR` no puede haber sido registrada por nadie. Se resuelve con una **secuencia de despliegue en dos etapas**, más tres invariantes que el chaincode aplica de forma permanente.

   **Precisión necesaria sobre el lifecycle de Fabric** (corregida respecto de una versión anterior de este ADR): la definición de chaincode que las organizaciones aprueban y confirman incluye nombre, versión, secuencia, política de endoso, configuración de colecciones y el indicador `--init-required`, pero **no incluye los argumentos de la invocación de `Init`**. Esa primera invocación es una transacción ordinaria, validada contra la política de endoso vigente del chaincode. Con la política laxa `OR` de ADR-007, una sola organización habilitada podría invocar `Init` sembrando como regulador un `mspId` de su elección. El bootstrap debe gobernarse explícitamente; no alcanza con apoyarse en el lifecycle.

   **Secuencia de despliegue en dos etapas:**
   - **Secuencia 1 — despliegue de bootstrap**: la definición del chaincode se aprueba y confirma con `--init-required` y una **política de endoso estricta**: `AND` de todas las organizaciones fundacionales del canal (o `MAJORITY` sobre ellas, según fije NET-4). Bajo esa política se invoca `Init`, que recibe el `mspId` regulatorio como argumento y siembra la entrada `REGULATOR` de `AnmatMSP`. Como la transacción de `Init` se valida contra esa política estricta, la semilla solo prospera si las organizaciones del canal endosan **esa misma invocación con ese mismo argumento**: es la aprobación multiparte que el lifecycle por sí solo no provee.
   - **Secuencia 2 — definición operativa**: se confirma una definición nueva (secuencia siguiente) con la política de endoso operativa de ADR-007 (`OR` laxa + state-based endorsement por clave). A partir de acá rigen las políticas ordinarias y toda alta posterior — custodial o financiador — pasa por `RegisterOrganization` normal.

   **Invariantes que el chaincode aplica de forma permanente:**
   - **Unicidad del regulador**: el chaincode rechaza el alta de una segunda entrada con `agentType=REGULATOR` mientras exista una activa. `Init` es idempotente y falla si la entrada ya existe, de modo que una reinvocación no puede sustituir al regulador.
   - **No desactivación del último regulador**: `SetOrganizationActive` rechaza desactivar la única entrada `REGULATOR` activa, para que la red no pueda quedar sin autoridad capaz de administrar el registro.
   - **Protección de la entrada regulatoria**: la clave de la entrada `REGULATOR` recibe, en la misma `Init`, una política de endoso por clave (state-based endorsement, ADR-007) que exige a `AnmatMSP`, de modo que ninguna organización pueda modificarla unilateralmente después del bootstrap.

**Por qué esta forma de bootstrap no viola ADR-003**: el principio de ADR-003 prohíbe cablear identidad de dominio como literal en el código del chaincode, porque eso convierte al código en fuente de verdad paralela y opaca. El argumento de `Init` no es un literal del código: es un parámetro de despliegue endosado por todas las organizaciones fundacionales bajo la política estricta de la secuencia 1, verificable en el ledger como cualquier otra transacción, y puede cambiar entre despliegues sin recompilar. La fuente de verdad sigue siendo el ledger — la `Init` solo escribe la primera entrada del registro; después de ese punto el argumento no vuelve a consultarse.

Queda fuera del alcance de esta ADR: la firma concreta de la función de init y el script de despliegue que la invoca (NET-4, #23), los cambios de texto del contrato DES-5 (`docs/api-contract.md`; ver Consecuencias) y la implementación de las validaciones en chaincode (CC-1, CC-7, CC-8).

## Justificación

La alternativa B es la única que preserva las dos propiedades que el diseño ya fijó como principios: una sola fuente de verdad de identidad (el registro, ADR-003) y ninguna comparación contra nombres de MSP en el chaincode. Con el registro extendido, la regla de autorización de las operaciones regulatorias y de las consultas del financiador se vuelve determinística y homogénea con la de los custodios: mismo mecanismo de resolución, mismos campos (`agentType`, `active`), mismo punto de administración (`RegisterOrganization` / `SetOrganizationActive`). Eso simplifica el chaincode (un solo camino de resolución de identidad) y los tests (los casos "organización no registrada / inactiva / `agentType` incompatible" que ADR-003 ya exige cubren también a los no custodiales).

La pluralidad de financiadores deja de ser una limitación del modelo: el paper y el relevamiento de campo describen un ecosistema con PAMI, obras sociales y prepagas, y bajo esta decisión cada uno se incorpora con el mismo onboarding gobernado que un establecimiento, sin tocar código ni definición de chaincode. La autorización sigue siendo doblemente derivada, como en DES-6: el `agentType` de la organización acota **qué puede hacer la organización** y `snt.role` acota **qué puede hacer la identidad** dentro de ella — `REGULATOR` sin `regulatory-admin` no ejecuta operaciones regulatorias, y `FINANCIER` con cualquier rol jamás escribe.

La alternativa A no alcanza porque resuelve el prototipo hipotecando el principio de ADR-003: el día que una MSP se renombre o un segundo financiador se incorpore, la identidad de dominio cableada en el código diverge del estado real de la red. La alternativa C no alcanza como mecanismo general porque convierte cada alta de financiador en una operación de gobernanza de lifecycle de chaincode — mucho más costosa que una transacción — y mantiene dos fuentes de verdad; su único uso legítimo es el punto de arranque, donde todavía no existe registro contra el cual derivar autorización, y ahí es precisamente donde esta ADR la emplea.

## Consecuencias

- **Para el registro (ADR-003 / `modelo-datos.md`)**: el struct del registro organización-establecimiento admite los nuevos valores de `agentType` (`REGULATOR`, `FINANCIER`) y de `idType` (`REG`). Es un agregado compatible: no cambia campos, claves ni semántica de las entradas custodiales existentes.
- **Para el contrato DES-5 (`docs/api-contract.md`)**: `RegisterOrganization` debe documentar los nuevos valores admitidos de `agentType`/`idType` (hoy su validación de `INVALID_REQUEST` enumera solo `GLN`/`CUFE` y el catálogo custodial), y `SetOrganizationActive` el rechazo por desactivación del último regulador. Es un cambio **MINOR** según la política de versionado del contrato (agregado compatible), incorporado en la versión 2.1.0 junto con los agregados de ADR-009 y ADR-011: el diseño del contrato pertenece a DES-5, no a las issues de implementación.
- **Para DES-6 (`docs/organizations-roles-endorsement.md`)**: conserva sus roles sin cambios — `regulatory-admin` y `financier-auditor` siguen siendo los roles ABAC aplicables; lo que cambia es que la organización a la que aplican se reconoce por el registro y no por su nombre de MSP. `FinanciadorMSP` y `AnmatMSP` pasan a leerse como ejemplos del dataset mínimo, igual que `FarmaciaMSP`.
- **Para la matriz DES-3 (`domain/authorized-transfers.json`)**: sin cambios. Conserva exactamente sus 6 `agentType` custodiales; los no custodiales quedan estructuralmente fuera de origen y destino.
- **Issues desbloqueadas**: CC-1 (#14, validaciones de identidad y registro en chaincode), CC-8 (#62, consultas del financiador) y NET-4 (#23, que debe sembrar la entrada `REGULATOR` en el despliegue).
- **Se gana**: una sola fuente de verdad de identidad para toda la red, autorización regulatoria y de financiador determinística sin literales de MSP, soporte nativo de múltiples financiadores con el mismo onboarding gobernado que un establecimiento, y cierre del hallazgo E5.
- **Se pierde / costo**: el despliegue deja de ser trivial — requiere dos secuencias de lifecycle (bootstrap con política estricta y definición operativa), y un error en la semilla obliga a repetir el ciclo; el registro pierde la homogeneidad "una entrada = un establecimiento físico con GLN/CUFE" (las entradas `REG` identifican organismos, no establecimientos).
- **Queda pendiente**: definir en NET-4 (#23) el script de despliegue en dos etapas — política estricta de la secuencia 1, invocación de `Init` con el argumento regulatorio, verificación de que la semilla quedó escrita y confirmación de la secuencia 2 con la política operativa — y elegir entre `AND` de todas las organizaciones fundacionales o `MAJORITY` para esa política de bootstrap.

## Divergencia con el trabajo escrito

No hay divergencia. El trabajo escrito lista a los organismos financiadores en plural (PAMI, obras sociales, prepagas) y presenta a ANMAT como autoridad de aplicación diferenciada del resto de los actores; esta decisión representa ambas cosas con mayor fidelidad que el modelo previo de un `FinanciadorMSP` singleton: cada financiador real puede incorporarse como organización propia y la autoridad regulatoria queda identificada por el registro y no por un nombre de configuración de red.

## Contexto utilizado

- Issue GitHub #85: DES-16 · ADR-010: Identidad de las organizaciones no custodiales (ANMAT y financiadores), consultada el 2026-08-17.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): registro organización-establecimiento, principio de no acoplar identidad de dominio al nombre de MSP, regla de resolución `cid.GetMSPID()` → registro.
- [ADR-005: Rol del organismo financiador en la dispensación](005-rol-organismo-financiador.md): financiador como organización no custodial de solo lectura; verificación claim-driven cuya autorización esta ADR hace derivable del registro.
- [DES-6: Organizaciones, MSP, roles, ABAC y políticas de endoso](../organizations-roles-endorsement.md): `AnmatMSP` y `FinanciadorMSP` como organizaciones no custodiales, roles `regulatory-admin` y `financier-auditor`, catálogo custodial de `agentType`.
- [Contrato DES-5](../api-contract.md): operaciones `RegisterOrganization`, `SetOrganizationActive` y error `REGULATORY_ONLY`; política de versionado (cambio MINOR).
- [Revisión de congruencia](../consistency-review.md), hallazgo E5: brecha de representación de ANMAT y el financiador en el modelo de identidad del chaincode.
- Hyperledger Fabric chaincode lifecycle: https://hyperledger-fabric.readthedocs.io/en/release-2.5/chaincode_lifecycle.html — parámetros que integran la definición aprobada (nombre, versión, secuencia, política de endoso, colecciones, `--init-required`) y, por contraste, el carácter de transacción ordinaria de la invocación de `Init`, validada contra la política de endoso vigente: fundamento de la secuencia de despliegue en dos etapas.
