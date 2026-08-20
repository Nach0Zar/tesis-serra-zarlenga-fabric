# Manual de actualización del trabajo escrito

- **Fecha**: 2026-08-16
- **Fuente de los hallazgos**: [`docs/consistency-review.md`](consistency-review.md) — cada entrada de este manual cita el ID de hallazgo de origen (A1, B4, E2, etc.) para trazabilidad.
- **Destinatarios**: los autores (Serra, Zarlenga), que editan a mano las fuentes del paper de congreso (10 páginas) y del avance de tesis (50 páginas), ambos publicados como PDF en `docs/papers/`.

## Propósito

Los agentes y revisores del repositorio no pueden editar los PDF ni sus fuentes LaTeX/Word. Este archivo es el manual de edición: reúne, **organizado por documento y por sección (de arriba hacia abajo)**, todos los cambios que la revisión de congruencia detectó como necesarios en el trabajo escrito, con redacción sugerida lista para copiar o adaptar. La idea es poder sentarse con el documento abierto y aplicar los cambios en orden, sin tener que reconstruir el análisis.

## Regla general

**El repositorio es la fuente de verdad del diseño.** Las decisiones registradas en ADRs con estado *Aceptado* (`docs/adr/`) y la matriz regulatoria (`domain/authorized-transfers.json`) prevalecen sobre lo que el trabajo escrito afirme. Ante cada discrepancia hay dos salidas válidas, y solo dos:

1. **Actualizar el texto** para reflejar la decisión vigente del repositorio; o
2. **Declarar la divergencia explícitamente** ("el diseño inicial contemplaba X; el análisis posterior condujo a Y, ver ADR-nnn"), cuando el documento ya fue enviado y no puede tocarse (caso típico: el paper de congreso ya presentado).

Lo que no puede quedar es el silencio: dos arquitecturas distintas sin explicación entre el texto publicado y el prototipo.

---

## 1. Paper de congreso

Cambios en orden de aparición dentro del documento.

### 1.1 — §3.1: atribución de los financiadores y ANMAT a la Disposición 3683/2011

- [ ] Aplicado
- **Hallazgo de origen**: A5 (severidad baja).
- **Ubicación exacta**: §3.1, párrafo que presenta los actores de la cadena.
- **Texto actual**: "De acuerdo con la Disposición 3683/2011 de la ANMAT, la cadena de suministro contemplada por el Sistema Nacional de Trazabilidad involucra a los laboratorios..., a los organismos financiadores, como las obras sociales y el PAMI..., y a la propia ANMAT".
- **Problema**: el artículo 2 de la Disposición 3683/2011 define como agentes solo a laboratorios, distribuidoras, operadores logísticos, droguerías, farmacias y establecimientos asistenciales. Los financiadores obtienen acceso al SNT por normativa posterior (Resolución PAMI 1735/2016, Disposición PAMI 1/17), como el propio avance de tesis trata correctamente en §2.1.3.1.
- **Cambio requerido** — reemplazar la frase por:

  > De acuerdo con la Disposición 3683/2011 de la ANMAT (art. 2), la cadena de suministro contemplada por el Sistema Nacional de Trazabilidad involucra a los laboratorios, las distribuidoras, los operadores logísticos, las droguerías, las farmacias y los establecimientos asistenciales. A ellos se suman, por normativa posterior —entre otras, la Resolución PAMI 1735/2016 y la Disposición PAMI 1/17—, los organismos financiadores, como las obras sociales y el PAMI, que acceden al sistema como agentes externos autorizados para verificar y auditar las dispensas realizadas a sus beneficiarios, y la propia ANMAT en su carácter de autoridad de aplicación.

  Antes de cerrar la redacción final, verificar la enumeración contra el texto oficial de la disposición.

### 1.2 — §3.2: enumeración de flujos de transferencia autorizados

- [ ] Aplicado
- **Hallazgo de origen**: A2 (severidad alta).
- **Ubicación exacta**: §3.2, enumeración de los pares origen→destino autorizados.
- **Texto/situación actual**: el paper autoriza solo **5 pares**: laboratorio→droguería, laboratorio→farmacia, laboratorio→centro médico, droguería→farmacia, droguería→centro médico. No aparecen distribuidoras ni operadores logísticos en los flujos (aunque §3.1 los lista como actores).
- **Problema**: la fuente única de verdad del prototipo (`domain/authorized-transfers.json`, DES-3) define **16 pares** autorizados; el paper omite 11. El chaincode va a autorizar operaciones que el texto no describe.
- **Cambio requerido** — elegir una de estas dos opciones:

  **Opción (a) — preferida si el documento admite tablas**: reemplazar la enumeración por la matriz completa. Redacción sugerida:

  > Las transferencias ordinarias de custodia admitidas por el prototipo se rigen por una matriz regulatoria de pares origen→destino, derivada del Decreto 1299/1997 (arts. 2, 4, 5 y 6) y de las Disposiciones ANMAT 7439/1999, 3683/2011 y 963/2015. La matriz define 16 pares autorizados y aplica denegación por defecto a todo par no declarado expresamente.

  Tabla completa de los 16 pares para copiar:

  | Origen | Destino |
  |---|---|
  | Laboratorio | Distribuidora |
  | Laboratorio | Operador logístico |
  | Laboratorio | Droguería |
  | Laboratorio | Farmacia |
  | Laboratorio | Establecimiento asistencial |
  | Distribuidora | Operador logístico |
  | Distribuidora | Droguería |
  | Distribuidora | Farmacia |
  | Distribuidora | Establecimiento asistencial |
  | Operador logístico | Droguería |
  | Operador logístico | Farmacia |
  | Operador logístico | Establecimiento asistencial |
  | Droguería | Droguería |
  | Droguería | Farmacia |
  | Droguería | Establecimiento asistencial |
  | Farmacia | Establecimiento asistencial |

  **Opción (b) — mínima, si el paper ya no puede reestructurarse**: conservar los 5 pares pero declararlos subconjunto. Agregar inmediatamente después de la enumeración:

  > Los flujos enumerados constituyen un subconjunto ilustrativo; la matriz regulatoria completa del prototipo define dieciséis pares origen→destino autorizados —incluyendo los que involucran distribuidoras y operadores logísticos— con denegación por defecto para todo par no declarado.

### 1.3 — §3.2, Figura 2: momento de la validación del financiador

- [ ] Aplicado
- **Hallazgo de origen**: A4 (severidad alta).
- **Ubicación exacta**: Figura 2 (§3.2), flujo de dispensación.
- **Situación actual**: la figura muestra "Solicitud de dispensación" → "Validación de cobertura" (Organismo Financiador) → "Registrar dispensación" (SNT) → confirmaciones. La validación del financiador aparece **antes** del registro de la dispensa y dentro de su camino.
- **Problema**: ADR-005 (Aceptado) decide lo contrario y descarta explícitamente la validación previa: el financiador **no** endosa ni autoriza la dispensación; es un verificador de solo lectura **posterior**, como condición para liberar el pago.
- **Cambio requerido** — rehacer la Figura 2 con **dos carriles separados**:
  1. **Carril de dispensación** (operación core, sin el financiador en el camino): farmacia / centro médico → registro de la dispensación en el SNT → confirmación. Ningún nodo del financiador aparece entre la solicitud y el registro.
  2. **Carril de verificación posterior** (dirigido por reclamo): a partir del serial presentado en la facturación off-ledger, el organismo financiador consulta (solo lectura) el estado público y el historial de la unidad dispensada → si la traza es legítima (unidad existente, estado DISPENSADO, agente dispensador habilitado) → liberación del pago, que ocurre fuera del ledger.

  Rotular el segundo carril como posterior en el tiempo (por ejemplo, con una línea divisoria "post-dispensa" o numerando las fases). **Alternativa mínima** si no se puede rehacer el diagrama: mantener la figura pero rotularla explícitamente como "circuito administrativo de autorización de cobertura" y agregar al pie: "La autorización de cobertura es un circuito administrativo separado del SNT; la validación de *trazabilidad* por parte del financiador es posterior a la dispensa y opera como condición para la liberación del pago."

### 1.4 — §3.3: organización por categoría → organización por establecimiento

- [ ] Aplicado
- **Hallazgo de origen**: A1 (severidad alta).
- **Ubicación exacta**: §3.3, descripción del modelo de organizaciones y MSP.
- **Texto actual**: "cada categoría de actor reconocida por el SNT se representa como una organización con identidad gestionada a través de un Proveedor de Servicios de Membresía (MSP)".
- **Problema**: ADR-003 (Aceptado, rev. 2) decide exactamente lo contrario: cada **establecimiento** (GLN/CUFE) es su propia organización Fabric, y descarta explícitamente la MSP compartida por categoría, porque la unidad mínima de confidencialidad que Fabric aplica de forma nativa (canales y colecciones privadas) es la organización. DES-6 y ADR-002 asumen ese modelo.
- **Cambio requerido** — reemplazar el párrafo por:

  > Cada establecimiento habilitado por el SNT —laboratorio, distribuidora, operador logístico, droguería, farmacia o establecimiento asistencial—, identificado por su GLN o CUFE, se representa como una organización Fabric independiente, con identidad gestionada a través de su propio Proveedor de Servicios de Membresía (MSP). Las categorías de actor se conservan como clasificación normativa: un registro liviano en el ledger traduce el identificador de cada organización a su identificador canónico de establecimiento (GLN o CUFE), su categoría de agente y su estado de habilitación. Esta granularidad responde a que la unidad mínima de confidencialidad que Fabric aplica de forma nativa, tanto en la membresía de un canal como en la de una colección de datos privados, es la organización: agrupar establecimientos de una misma categoría bajo una MSP compartida impediría garantizar que un establecimiento no acceda a información de operaciones de las que no forma parte.

  **Si el paper ya fue enviado y no puede modificarse**: aplicar en su lugar la declaración de divergencia en el avance de tesis (ver entrada 2.12 de este manual, "Capítulo de diseño"), con la fórmula: "el diseño inicial contemplaba una MSP por categoría de actor; el análisis de confidencialidad condujo a una organización por establecimiento (ver ADR-003)".

### 1.5 — §3.3 y §3.4: referencia [15] citada pero inexistente

- [ ] Aplicado
- **Hallazgo de origen**: B1 (severidad crítica).
- **Ubicación exacta**: §3.3 ("...manteniendo en el ledger compartido únicamente el hash de la información privada para su validación [15]"), §3.4 ("...condición que se sostiene en la arquitectura de canales y colecciones de datos privados de la plataforma [13][15]") y la lista de referencias, que termina en [14].
- **Problema**: [15] se cita dos veces pero no existe en la lista. Es lo primero que salta en una revisión editorial.
- **Cambio requerido**: agregar la entrada [15] a la lista de referencias. Por el contexto de ambas citas, corresponde a la documentación de Hyperledger Fabric sobre *Private Data*. Entrada sugerida (adaptar tipografía y puntuación al formato exacto de las referencias [12]–[14] del paper, que citan la documentación de Fabric como "(s. f.)"):

  > [15] Hyperledger. (s. f.). *Private data*. Hyperledger Fabric Documentation, release-2.5. https://hyperledger-fabric.readthedocs.io/en/release-2.5/private-data/private-data.html

  Alternativa: si la intención original era citar una referencia ya existente (por ejemplo, la [13] u otra entrada de documentación de Fabric), renumerar las citas [15] hacia esa entrada y no agregar nada. Elegir una de las dos salidas; no dejar la cita colgante.

### 1.6 — §3.4: alcance de la garantía de aislamiento de información

- [ ] Aplicado
- **Hallazgo de origen**: A3 (severidad alta).
- **Ubicación exacta**: §3.4, frase sobre aislamiento.
- **Texto actual**: "El aislamiento de la información entre actores garantiza que ningún establecimiento acceda a información correspondiente a transacciones de las que no forma parte, con excepción de la autoridad de aplicación".
- **Problema**: leída literalmente, la frase promete una propiedad que el diseño deliberadamente no cumple: ADR-002 (Aceptado) decide que el **estado mínimo de trazabilidad** (GTIN, serie, lote, vencimiento, custodio actual, estado del producto) es público para todos los miembros del canal, y que solo la información comercial/documental va a colecciones de datos privados. La justificación es funcional: cualquier destinatario legítimo debe poder validar de forma independiente el estado de una unidad antes de aceptar su custodia.
- **Cambio requerido** — reemplazar la frase por:

  > El aislamiento de la información comercial y documental entre actores garantiza que ningún establecimiento acceda a los datos comerciales —precios, condiciones, cantidades negociadas, documentación de respaldo— de transacciones de las que no forma parte, con excepción de la autoridad de aplicación. El estado mínimo de trazabilidad de cada unidad (identificador del producto, número de serie, lote, fecha de vencimiento, custodio actual y estado) mantiene, en cambio, visibilidad para todas las organizaciones del canal: es el dato regulatorio mínimo que cualquier eslabón autorizado de la cadena necesita para verificar de forma independiente la legitimidad de lo que recibe antes de aceptar la custodia. Esta distinción entre dato regulatorio mínimo e información comercial constituye una interpretación de diseño del artículo 9 de la Disposición 3683/2011, adoptada de forma consciente: la disposición no distingue explícitamente ambas categorías de datos.

### 1.7 — §3.4: excepción del endoso mono-organización en el registro inicial

- [ ] Aplicado
- **Hallazgo de origen**: E7 (severidad baja).
- **Ubicación exacta**: §3.4, afirmación de que las comprobaciones "son ejecutadas y endosadas por los nodos de las distintas organizaciones".
- **Problema**: DES-6 define para el registro inicial de una unidad (`RegisterUnit`) el endoso del laboratorio invocante **solamente**, porque en ese momento no existe contraparte previa. La afirmación general del paper deja esa excepción sin explicar, y conviene que la explicación llegue antes de que el jurado encuentre la excepción.
- **Cambio requerido** — agregar a continuación de esa afirmación:

  > La única excepción a este esquema de endoso multiorganizacional es el registro inicial de cada unidad, dado que en ese momento no existe una contraparte previa en la cadena capaz de verificar la custodia: la plataforma exige la validación del nodo del laboratorio que la origina, pero no la de ninguna otra organización. La integridad multiorganizacional del alta se ejerce, por lo tanto, de forma retrospectiva en la primera transferencia, cuando la organización receptora valida de forma independiente el estado registrado antes de aceptar la custodia.

- **Precisión sobre la redacción**: la palabra correcta es **"necesariamente"**, no "únicamente". El diseño obliga al nodo del laboratorio a validar el alta —lo consigue haciendo que la operación escriba también un dato en el espacio privado de esa organización, cuya política de validación le pertenece y rige desde el despliegue—, pero no impide que otros nodos validen además. Endosantes de más no debilitan nada: solo agregan ejecuciones coincidentes de la misma lógica determinística. El mecanismo está en ADR-007, punto 6.g.

### 1.8 — §3.2, "Caso reingreso a stock": condición de custodia con la negación invertida

- [ ] Aplicado
- **Hallazgo de origen**: detectado en la review del PR #88 al redactar ADR-009.
- **Ubicación exacta**: §3.2, párrafo "Caso reingreso a stock".
- **Texto actual** (literal): "Ejemplos de esto puede ser que el medicamento no se encuentre vencido, que el actor que intenta reingresar a stock el medicamento **no sea** el actual custodio de este o que el medicamento no se haya registrado como destruido o finalmente dispuesto."
- **Problema**: la enumeración mezcla dos formas. El primer y el tercer ejemplo describen condiciones cuya verificación protege la operación (que no esté vencido, que no esté destruido), pero el segundo, leído literalmente, exigiría validar que quien reingresa **no** sea el custodio actual — lo contrario de lo razonable: quien reincorpora una unidad al stock es precisamente quien la tiene bajo su custodia registrada. El prototipo adopta la lectura corregida (ADR-009 resuelve el actor de recupero como el custodio actual registrado) y la declara como interpretación, no como cita literal. Mientras la fuente no se corrija, el ADR y el trabajo escrito dicen cosas distintas.
- **Cambio requerido** — reformular la enumeración para que las tres condiciones tengan la misma orientación. Redacción sugerida:

  > Ejemplos de estas validaciones son que el medicamento no se encuentre vencido, que el actor que intenta reingresarlo a stock sea su actual custodio, y que el medicamento no haya sido registrado como destruido o finalmente dispuesto.

  Si el equipo considera que la redacción original era intencional y que efectivamente debe validarse lo contrario, entonces el cambio va del otro lado: hay que revisar ADR-009, porque el prototipo estaría implementando una regla distinta de la relevada.

### 1.9 — §3.4: el aislamiento no cubre los metadatos de relación entre organizaciones

- [ ] Aplicado
- **Hallazgo de origen**: detectado en la review del PR #88 sobre ADR-006.
- **Ubicación exacta**: §3.4, a continuación del párrafo de aislamiento que corrige la entrada 1.6. Aplica también al capítulo de limitaciones de la tesis (entrada 2.12).
- **Problema**: la corrección de la entrada 1.6 acota el aislamiento al **contenido** comercial y documental, pero sigue sin decir qué pasa con el **hecho** de que dos organizaciones operaron entre sí. ADR-006 materializa las colecciones privadas como una colección por par de organizaciones, con nombre determinístico derivado de ambos `mspId`. Ese nombre viaja **en claro** en el `CollectionHashedReadWriteSet` de cada transacción, y la membresía de cada colección es pública porque forma parte de la definición del chaincode que todas las organizaciones aprueban en el lifecycle. En consecuencia, cualquier miembro del canal que observe los bloques puede inferir que las organizaciones A y B registraron una operación en un momento dado, aunque no pueda leer ni el contenido ni sobre qué unidad recae. El mecanismo **reduce** la exposición de relaciones comerciales a metadatos observables por análisis de bloques; no la elimina.
- **Cambio requerido** — agregar a continuación del párrafo de aislamiento:

  > Esta protección alcanza al contenido de cada operación, no al hecho de que dos organizaciones hayan operado entre sí. La materialización elegida —una colección de datos privados por par de organizaciones autorizadas— hace que el identificador de la colección utilizada quede registrado en claro en el conjunto de lectura-escritura de cada transacción, de modo que un observador con acceso al canal puede inferir que dos establecimientos registraron una operación, sin poder determinar su contenido, su valor ni la unidad involucrada. Cerrar también ese canal de metadatos exigiría una colección única con contenido cifrado extremo a extremo, alternativa descartada para el prototipo porque traslada la garantía de confidencialidad desde la plataforma hacia la gestión de claves y compromete la visibilidad regulatoria.

- **Nota**: si el equipo prefiere no extender §3.4 en el paper de congreso por límite de espacio, la salvedad **debe** aparecer al menos en el capítulo de limitaciones de la tesis. Lo que no puede quedar es la afirmación de aislamiento sin la acotación.

### 1.10 — §3.4 y conclusiones: enunciar la propiedad acotada de no unilateralidad

- [ ] Aplicado
- **Hallazgo de origen**: detectado en la cuarta ronda de review del PR #88 sobre ADR-007. **Esta entrada agrega una afirmación, no corrige un error**: es un resultado del diseño que el trabajo escrito todavía no aprovecha.
- **Ubicación exacta**: §3.4, en el desarrollo sobre políticas de endoso.
- **Contexto**: al materializar las políticas de endoso se detectó que Hyperledger Fabric evalúa la política de validación de cada dato escrito **sin saber qué operación la produjo**. Una excepción concedida a la autoridad de aplicación para habilitar sus facultades regulatorias habilitaba, con la misma fuerza, que esa autoridad avalara en solitario cualquier operación ordinaria — una dispensación, una recepción de mercadería. El diseño eliminó esa excepción y obtuvo la participación regulatoria por otra vía: la operación escribe también un dato en el espacio privado de la autoridad, cuya política de validación le pertenece, de modo que su nodo debe validar junto con el del custodio.
- **Cuidado con la versión amplia**: no debe afirmarse que «ninguna organización puede modificar por sí sola el estado de una unidad». Es falso, y un jurado puede refutarlo con un ejemplo simple: una farmacia dispensa su propia unidad con su sola organización. La afirmación defendible es la acotada que sigue.
- **Cambio requerido** — agregar al desarrollo de políticas de endoso:

  > De este esquema se desprende una propiedad que conviene enunciar de forma explícita: ninguna organización de la red puede, por sí sola, cambiar el custodio registrado de una unidad, intervenir sobre una unidad que no está bajo su custodia, ni sustituir a una contraparte que el proceso exige. El custodio no puede transferir sin que el destinatario valide; el destinatario no puede aceptar sin que el remitente valide; la autoridad regulatoria no puede intervenir sobre una unidad ajena sin que validen su propio nodo y el del custodio; y un laboratorio no puede retirar del mercado una unidad bajo custodia de un tercero sin la validación conjunta de la autoridad regulatoria y de ese custodio. Las operaciones que un custodio realiza sobre lo que efectivamente tiene bajo su custodia —dispensar, poner en cuarentena, informar un evento— sí se resuelven con su sola organización, y de forma deliberada: exigir la participación de la autoridad de aplicación en toda escritura la convertiría en un cuello de botella y reintroduciría la dependencia de un administrador central que el diseño busca evitar. El precio de la garantía es de disponibilidad —una intervención regulatoria depende de que el nodo del custodio esté operativo— y se asume de forma consciente.

- **Nota**: es probablemente el resultado más citable del capítulo de diseño; conviene que aparezca también en las conclusiones, ligado a la hipótesis sobre integridad y ausencia de un administrador único.

---

## 2. Avance de tesis

Cambios en orden de aparición dentro del documento (por página).

### 2.1 — §2.1.2.1 (p. 5) y bibliografía (p. 29): Resolución 435/2011 mal atribuida

- [ ] Aplicado
- **Hallazgo de origen**: B3 (severidad media).
- **Ubicación exacta**: texto en §2.1.2.1 (p. 5): "La resolución 435/2011 del Ministerio de salud argentino establece..."; entrada bibliográfica (p. 29): "ADMINISTRACIÓN NACIONAL DE MEDICAMENTOS, ALIMENTOS Y TECNOLOGÍA MÉDICA (ANMAT), 2011. *Resolución 435/2011*...".
- **Problema**: la Resolución 435/2011 la dicta el Ministerio de Salud, no ANMAT (la Disposición 3683/2011 sí es de ANMAT). El texto y la bibliografía se contradicen entre sí, y la entrada bibliográfica atribuye la norma al organismo equivocado. El repo la cita correctamente como "Resolución MS 435/2011" (ADR-001).
- **Cambio requerido**:
  1. Corregir el autor institucional de la entrada bibliográfica. Entrada corregida (mantener el resto de los campos y el formato del documento):

     > MINISTERIO DE SALUD DE LA NACIÓN ARGENTINA, 2011. *Resolución 435/2011*. [resto de la entrada según el formato de la bibliografía]

  2. Ajustar la cita en el texto para que el sistema autor-año siga resolviendo (por ejemplo: "La Resolución 435/2011 del Ministerio de Salud de la Nación (Ministerio de Salud de la Nación Argentina, 2011) establece...").
  3. Verificar de paso que la URL de la entrada apunte efectivamente a la Resolución 435/2011 y no a otra norma.

### 2.2 — §2.1.3 (p. 6): incorporar la Disposición ANMAT 7439/1999 y desarrollar el Decreto 1299/1997

- [ ] Aplicado
- **Hallazgo de origen**: E2 (severidad media).
- **Ubicación exacta**: lista cronológica de normas del marco regulatorio (§2.1.3, p. 6) y desarrollo del Decreto 1299/1997 en esa misma sección; párrafo adicional en §2.1.3.1 sobre distribuidoras y operadores logísticos.
- **Problema**: la matriz regulatoria del prototipo (`domain/authorized-transfers.json`) fundamenta 5 pares autorizados en la Disposición ANMAT 7439/1999, que la lista cronológica de la tesis no incluye; y usa los artículos 2, 4, 5 y 6 del Decreto 1299/1997 como base de casi todos los pares, pero la tesis lista el decreto sin desarrollar esos artículos. En la defensa, cada regla del chaincode debería poder rastrearse al marco teórico.
- **Cambio requerido**:
  1. **Agregar a la lista cronológica** (entre 1997 y 2011), entrada sugerida:

     > **1999 — Disposición ANMAT 7439/1999**: regula la habilitación de las distribuidoras de medicamentos y define la figura del operador logístico, que actúa por cuenta y orden de laboratorios o distribuidoras y entrega los productos a los destinos indicados por el titular (arts. 2, 5 y 7).

  2. **Desarrollar el Decreto 1299/1997**, párrafo sugerido para agregar donde el marco regulatorio lo presenta:

     > El Decreto 1299/1997, que reglamenta la comercialización de especialidades medicinales, define los circuitos comerciales lícitos de la cadena: el artículo 2 establece que los laboratorios, directamente o mediante distribuidoras que actúan por su cuenta y orden, comercializan con droguerías, farmacias y establecimientos asistenciales habilitados; el artículo 4 dispone que las farmacias adquieren especialidades medicinales a laboratorios, distribuidoras o droguerías, y pueden venderlas a establecimientos asistenciales; el artículo 5 habilita a los establecimientos asistenciales a adquirir a laboratorios, distribuidoras, droguerías y farmacias habilitadas; y el artículo 6 reconoce las transacciones comerciales documentadas entre droguerías. Estos cuatro artículos, junto con la Disposición ANMAT 7439/1999, constituyen el fundamento normativo de la matriz de transferencias autorizadas que el prototipo formaliza y aplica de manera determinística.

  3. **Agregar un párrafo en §2.1.3.1** sobre distribuidoras y operadores logísticos, texto sugerido:

     > Las distribuidoras y los operadores logísticos participan de la cadena sin ser propietarios comerciales de los productos: la distribuidora comercializa por cuenta y orden del laboratorio (Decreto 1299/1997, art. 2) y el operador logístico, habilitado por la Disposición ANMAT 7439/1999, actúa por cuenta y orden de laboratorios o distribuidoras, ejerciendo la custodia física durante la distribución y entregando los productos a los destinos indicados por el titular. Por esta capacidad de custodia física, ambos se mantienen como agentes diferenciados del sistema de trazabilidad.

### 2.3 — §2.1.3.1, "Organismos financiadores": acceso de auditoría de PAMI presentado como hecho normativo

- [ ] Aplicado
- **Hallazgo de origen**: B7 (severidad baja).
- **Ubicación exacta**: §2.1.3.1, apartado "Organismos financiadores".
- **Texto actual**: "Para este fin, el instituto posee acceso de auditoría a la base de datos central."
- **Problema**: la Resolución PAMI 1735/2016 y la Disposición PAMI 1/17 no establecen expresamente ese acceso en su texto (lo que establecen es la convalidación de dispensaciones ya informadas como condición del pago). La afirmación queda sin respaldo normativo si se la presenta como hecho.
- **Cambio requerido** — reemplazar la frase por:

  > Para este fin, el instituto accede al SNT como agente externo autorizado para verificar y auditar las dispensas realizadas a sus beneficiarios (ver "Agentes externos con acceso al SNT"). La Resolución PAMI 1735/2016 y la Disposición PAMI 1/17 condicionan el pago de las liquidaciones a la industria a la convalidación de las dispensaciones ya informadas al sistema.

  Si se identifica una norma que establezca expresamente el acceso de auditoría a la base central, citarla; en ese caso puede mantenerse la afirmación original con su cita.

### 2.4 — §2.1.3.1, "Droguerías": inicio de trazabilidad por droguería excluido del prototipo

- [ ] Aplicado
- **Hallazgo de origen**: E1 (severidad alta).
- **Ubicación exacta**: §2.1.3.1, apartado "Droguerías", donde se afirma que "tienen la posibilidad de ser el eslabón de origen de la trazabilidad del medicamento, como los laboratorios".
- **Problema**: el prototipo restringe el registro inicial de unidades exclusivamente al laboratorio (ADR-001, T01). La tesis documenta una capacidad normativa que el prototipo no soporta, y sin una nota el lector concluye que el prototipo está incompleto sin saber si fue a propósito.
- **Cambio requerido** — mantener el texto normativo (es correcto) y agregar a continuación:

  > Cabe señalar que el prototipo desarrollado en este trabajo excluye de manera consciente el inicio de trazabilidad por droguería: solo los laboratorios ejecutan el registro inicial de unidades. El caso existe en la normativa relevada, pero es minoritario y no altera la evaluación comparativa del prototipo, por lo que se registra como limitación de alcance (ver el capítulo de alcance del prototipo).

  Nota de sincronización: esta exclusión debe quedar también registrada como fila en `docs/alcance-prototipo.md` (mismo hallazgo E1); verificar que ambos lados digan lo mismo.

### 2.5 — §2.1.3.2, "Registro de agentes": nota sobre el alcance del CUFE

- [ ] Aplicado
- **Hallazgo de origen**: E6 (severidad baja).
- **Ubicación exacta**: §2.1.3.2, donde se indica que el GLN es obligatorio para laboratorio/distribuidora/operador logístico/droguería y que el CUFE aplica a laboratorios de producción pública.
- **Problema**: el texto normativo de la tesis es correcto, pero el prototipo (ADR-003) acepta `CUFE` como identificador canónico para cualquier tipo de agente, como simplificación consciente. Sin una nota, la tesis y el diseño se contradicen en silencio.
- **Cambio requerido** — agregar al final del apartado:

  > El prototipo desarrollado en este trabajo acepta GLN o CUFE como identificador canónico de cualquier tipo de agente, como simplificación consciente del modelo de identidad; la normativa acota el CUFE a los laboratorios de producción pública, mientras que los restantes agentes se registran con GLN de GS1 Argentina. Una implementación productiva debería validar que el CUFE solo acompañe a establecimientos habilitados para utilizarlo. Esta simplificación se lista entre las limitaciones del prototipo.

### 2.6 — Figura 2.1 (p. 14): flujos dibujados que la matriz del prototipo no autoriza

- [ ] Aplicado
- **Hallazgo de origen**: A2 (severidad alta) — lado del avance de tesis.
- **Ubicación exacta**: Figura 2.1 (p. 14), diagrama de la cadena de distribución.
- **Situación actual**: la figura muestra laboratorio→droguería, laboratorio→distribuidora, **droguería→distribuidora/operadora logística**, distribuidora→farmacia y distribuidora→hospital.
- **Problema**: la flecha droguería→distribuidora/operador logístico corresponde a un par que la matriz regulatoria del propio proyecto **rechaza** (`defaultDecision: DENY`; el par no figura entre los 16 autorizados). Además la figura omite la mayoría de los pares autorizados.
- **Cambio requerido** — rehacer la figura de modo que **cada flecha corresponda a un par de los 16 autorizados** de `domain/authorized-transfers.json` (tabla completa en la entrada 1.2 de este manual):
  - **Flechas a quitar**: droguería→distribuidora; droguería→operador logístico (ambas denegadas por la matriz).
  - **Flechas a conservar**: laboratorio→droguería; laboratorio→distribuidora; distribuidora→farmacia; distribuidora→hospital (renombrar "hospital" como "establecimiento asistencial", consistente con la terminología normativa).
  - **Flechas a agregar** (para completar los 16 pares): laboratorio→operador logístico; laboratorio→farmacia; laboratorio→establecimiento asistencial; distribuidora→operador logístico; distribuidora→droguería; operador logístico→droguería; operador logístico→farmacia; operador logístico→establecimiento asistencial; droguería→droguería (flecha reflexiva); droguería→farmacia; droguería→establecimiento asistencial; farmacia→establecimiento asistencial.
  - Si dibujar los 16 pares satura el diagrama, es válido dibujar un subconjunto **siempre que ninguna flecha corresponda a un par denegado** y que el epígrafe declare: "Se representan los flujos principales; la matriz completa del prototipo autoriza dieciséis pares origen→destino".
  - **Alternativa** (solo si el equipo decide que droguería→distribuidora/operador logístico es un flujo real que debería permitirse, p. ej. logística tercerizada de una droguería): el fix va al revés — agregar el par a `domain/authorized-transfers.json` con su referencia normativa y subir `schemaVersion` a MINOR. Hay que decidir una dirección; no pueden convivir las dos versiones.

### 2.7 — §2.1.5 (pp. 16–19) y §2.1.6.2 (p. 24): cuatro conceptos técnicos imprecisos del marco teórico

- **Hallazgo de origen**: B4 (severidad media). Cuatro sub-entradas; ninguna afecta el diseño del prototipo, pero el marco teórico fundamenta la elección de Fabric y un jurado técnico puede objetarlas.

#### 2.7.a — Nonce presentado como elemento general (§2.1.5, p. 17, y Figura 2.2, p. 16)

- [ ] Aplicado
- **Problema en una frase**: el nonce es un artefacto específico de Proof of Work, no un componente general de las funciones hash ni de toda cadena de bloques (Fabric con Raft no lo usa), pero el texto lo presenta como elemento general y la Figura 2.2 —rotulada "cadena de bloques aplicada a la trazabilidad de medicamentos"— lo incluye en el encabezado de cada bloque.
- **Redacción de reemplazo sugerida** (mover la mención del nonce al párrafo de PoW):

  > En los protocolos de consenso basados en Prueba de Trabajo, como el de Bitcoin, el encabezado de cada bloque incluye un valor arbitrario denominado *nonce*, que los nodos mineros varían sucesivamente hasta que el hash del bloque satisface la dificultad objetivo de la red. El nonce es, por lo tanto, un artefacto propio de la Prueba de Trabajo y no un componente general de las funciones hash ni de toda cadena de bloques: las plataformas permisionadas con ordenamiento determinístico, como Hyperledger Fabric con Raft, no lo utilizan.

  Además: **quitar el campo "Nonce"** del encabezado de los bloques en la Figura 2.2, o bien re-rotular la figura como "esquema genérico de una cadena de bloques de tipo Bitcoin" en lugar de "aplicada a la trazabilidad de medicamentos".

#### 2.7.b — Firma digital como "encriptar con la clave privada" y párrafo de direcciones (§2.1.5.1, p. 18)

- [ ] Aplicado
- **Problema en una frase**: presentar la firma como "encriptación con la clave privada" es el modelo didáctico de RSA que no aplica en general (ni en ECDSA, que es lo que usa Fabric), y el texto asocia el concepto de *address* a "encriptar con la clave pública" cuando la propia cita de NIST indica que la dirección se deriva de la clave pública mediante una función hash.
- **Redacción de reemplazo sugerida**:

  > La firma digital es una operación criptográfica específica: el emisor genera la firma sobre el mensaje —o sobre su hash— utilizando su clave privada, y cualquier tercero puede verificar esa firma con la clave pública correspondiente, comprobando así la autoría y la integridad del mensaje. En esquemas como ECDSA, empleado por Hyperledger Fabric, firmar no equivale a encriptar el mensaje. Por su parte, las direcciones (*addresses*) utilizadas en redes blockchain no guardan relación con la encriptación hacia un destinatario: se derivan de la clave pública mediante la aplicación de una función hash, tal como lo describe el NIST.

#### 2.7.c — Proof of Work como defensa anti-DoS (§2.1.5.4, p. 19)

- [ ] Aplicado
- **Problema en una frase**: la función principal del costo computacional de PoW es hacer costosa la producción de bloques (resistencia sybil / seguridad del consenso); presentarlo primariamente como mitigación de denegación de servicio es débil.
- **Redacción de reemplazo sugerida**:

  > La función principal del costo computacional en la Prueba de Trabajo es hacer costosa la producción de bloques: proponer un bloque válido exige invertir trabajo verificable, lo que protege al consenso frente a ataques de identidades múltiples (ataques Sybil), en los que un atacante crearía identidades ilimitadas sin costo para dominar la decisión colectiva. La eventual mitigación de ataques de denegación de servicio es un efecto secundario de ese costo, no el propósito primario del mecanismo.

#### 2.7.d — Raft asociado a Prueba de Autoridad/Identidad (§2.1.6.2, Etapa 2, p. 24)

- [ ] Aplicado
- **Problema en una frase**: Raft es un protocolo de replicación de log tolerante a fallas por caída (CFT); su elección de líder no proviene de PoA ni pone en juego reputación o identidad.
- **Redacción de reemplazo sugerida**:

  > Raft es un protocolo de replicación de registro (*log*) tolerante a fallas por caída (*crash fault tolerant*, CFT): los nodos del servicio de ordenamiento eligen un líder por mayoría (quórum) y replican en él el registro de transacciones, de modo que el clúster tolera la caída de una minoría de sus nodos. Su elección de líder es un mecanismo determinístico de consenso entre nodos conocidos, descripto en la documentación del servicio de ordenamiento de Hyperledger Fabric, y no deriva de la Prueba de Autoridad ni involucra reputación o identidad puesta en juego.

  Complementario (opcional, refuerza el argumento a favor de Fabric): agregar en §2.1.5.5 ("resolución de conflictos de cadenas") una frase aclarando que en una red permisionada con ordenamiento determinístico como Fabric/Raft no se producen forks de tipo Nakamoto, por lo que la regla de la cadena más larga no aplica al prototipo.

### 2.8 — §2.1.6.1 (pp. 22–23): peers de anclaje definidos dos veces

- [ ] Aplicado
- **Hallazgo de origen**: B5 (severidad baja).
- **Ubicación exacta**: §2.1.6.1; primero se listan "dos tipos de nodos pares" (de anclaje y de endoso) e inmediatamente después "dos tipos de roles" (líderes y anclas).
- **Problema**: "anclaje" y "anclas" son el mismo concepto (*anchor peers*) repetido en dos listas con redacción distinta.
- **Cambio requerido** — unificar ambas listas en una clasificación única. Redacción sugerida:

  > Todos los nodos pares (*peers*) alojan una copia del ledger y pueden asumir, además, uno o más roles específicos: nodo de endoso (*endorsing peer*), cuando ejecuta el chaincode para firmar propuestas de transacción; nodo líder (*leader peer*), cuando distribuye a los demás peers de su organización los bloques recibidos del servicio de ordenamiento; y nodo de anclaje (*anchor peer*), cuando está declarado en la configuración del canal como punto de descubrimiento para la comunicación entre organizaciones. No se trata de tipos disjuntos de nodos: un mismo peer puede cumplir varios de estos roles simultáneamente.

  (Clasificación tomada de la página "Peers" de la documentación de Fabric, ya citada en la bibliografía.)

### 2.9 — Bibliografía (pp. 30–31): fechado "Hyperledger, 2020a–g"

- [ ] Aplicado
- **Hallazgo de origen**: B6 (severidad baja).
- **Ubicación exacta**: entradas "Hyperledger, 2020a" a "Hyperledger, 2020g" de la bibliografía, y todas sus citas en el texto.
- **Problema**: las URLs apuntan a la documentación de `release-2.5`, publicada en 2023 y actualizada después; el año 2020 no corresponde. Además, el paper de congreso cita las mismas fuentes como "(s. f.)", lo cual es inconsistente entre los dos documentos.
- **Cambio requerido**: reemplazar "2020a"–"2020g" por "(s. f.-a)"–"(s. f.-g)" (sin fecha) en la bibliografía **y** en todas las citas del texto, quedando consistente con el criterio ya usado en el paper. Alternativa igualmente válida: usar el año real de la release en **ambos** documentos; lo que no puede quedar es un documento con "2020" y el otro con "(s. f.)".

### 2.10 — Anexo A (p. 32): texto placeholder de plantilla y cronograma faltante

- [ ] Aplicado
- **Hallazgo de origen**: B2 (severidad crítica).
- **Ubicación exacta**: Anexo A (p. 32).
- **Texto actual**: "No obstante, se presentó una demora en …, motivo por el cual …" — frase con puntos suspensivos sin completar, sin ningún cronograma debajo.
- **Cambio requerido**:
  1. **Borrar textualmente** la frase inconclusa "No obstante, se presentó una demora en …, motivo por el cual …", o completarla con la demora real y su motivo si efectivamente hubo una.
  2. **Insertar el cronograma real del proyecto**: diagrama de Gantt o tabla de tareas con columnas Tarea | Inicio | Fin | Estado (completada / en curso / pendiente), cubriendo al menos: relevamiento normativo, marco teórico, decisiones de diseño (ADRs / stories DES-*), implementación del chaincode y la red, baseline centralizada, benchmarks y medición, redacción final. El repositorio (issues DES-1 a DES-11, `docs/adr-roadmap.md`) sirve como insumo del estado real de cada tarea.

### 2.11 — Anexo B (p. 33): instrucciones de la cátedra sin reemplazar y encuesta sin resultados

- [ ] Aplicado
- **Hallazgo de origen**: B2 (severidad crítica).
- **Ubicación exacta**: Anexo B (p. 33).
- **Texto actual**: "Procurar que las encuestas sean representativas. Al menos 120 encuestados en una muestra homogénea. La encuesta, claramente, debe favorecer el por qué de la tesis. Si se realiza por Google Forms... el scope *enumerate* o el scope *itemize*." — son instrucciones de la cátedra/plantilla, no contenido, y no hay resultados de encuesta.
- **Cambio requerido** — elegir una de estas dos salidas:
  1. **Si la encuesta forma parte del plan**: borrar textualmente todo el bloque de instrucciones citado arriba y reemplazarlo por el contenido real: objetivo de la encuesta, instrumento (preguntas), población y muestra (mínimo 120 respuestas según la propia consigna), y resultados con sus gráficos/tablas y lectura.
  2. **Si la encuesta no forma parte del plan**: eliminar el Anexo B completo, junto con toda referencia a él en el cuerpo del documento, y verificar que el índice no lo liste.

### 2.12 — Capítulo de diseño (para la próxima entrega): incorporar las decisiones de los ADRs

- [ ] Aplicado
- **Hallazgos de origen**: A1, A3, A4 (lado tesis) y regla general de sincronización.
- **Ubicación**: capítulo de diseño/arquitectura de la próxima iteración del documento de tesis (hoy inexistente o embrionario; §1.2.1 lista las MSP por categoría y debe reescribirse).
- **Cambio requerido** — el capítulo debe incorporar, como mínimo, estas cuatro decisiones ya aceptadas en el repositorio, presentadas como decisiones de diseño con su justificación:
  1. **Interpretación normativa y clasificación de datos (ADR-002)**: canal único; distinción entre *estado mínimo de trazabilidad* (GTIN, serie, lote, vencimiento, custodio actual, estado — público para todos los miembros del canal) e *información comercial y documental* (restringida a las partes de la operación más ANMAT, mediante Private Data Collections). Presentarla explícitamente como interpretación consciente y discutible del artículo 9 de la Disposición 3683/2011, no como lectura unívoca — tal como exige la sección "Consecuencias → Para la tesis" del propio ADR.
  2. **Organización por establecimiento (ADR-003)**: cada establecimiento (GLN/CUFE) es una organización Fabric con su propia MSP; las categorías de actor sobreviven como clasificación normativa (`agentType`) en un registro organización-establecimiento que traduce `mspId → GLN/CUFE`. Si el paper de congreso publicado no puede corregirse, documentar acá la evolución: "el diseño inicial contemplaba una MSP por categoría de actor; el análisis de confidencialidad condujo a una organización por establecimiento (ver ADR-003)".
  3. **Modelo de dos transacciones para la transferencia (ADR-004)**: despacho (emisor, genera EN_TRANSITO) y recepción o rechazo (receptor, genera EN_CUSTODIA o DEVUELTO) como transacciones separadas; el custodio registrado permanece en el emisor durante el tránsito y el destinatario declarado vive en la colección privada de la operación, no en el estado público.
  4. **Rol del organismo financiador (ADR-005)**: organización no custodial de solo lectura; no endosa ni autoriza la dispensación; verifica la traza con posterioridad a la dispensa, de forma dirigida por reclamo, como condición para liberar un pago que ocurre fuera del ledger; ningún dato personal del afiliado se persiste en cadena.
- **Herramienta de control**: al redactar el capítulo, usar el "Checklist de sincronización con el trabajo escrito" de [`docs/adr-roadmap.md`](adr-roadmap.md) para verificar que cada afirmación arquitectónica del texto quede mapeada a un ADR que la implemente o que documente la divergencia.

---

## 3. Tabla resumen y cierre

Prioridad derivada de la severidad del hallazgo en `consistency-review.md`: 🔴/🟠 → **Alta**, 🟡 → **Media**, 🔵 → **Baja**.

| # | Documento — ubicación | Hallazgo | Prioridad |
|---|---|---|---|
| 1.1 | Paper §3.1 — actores de la 3683/2011 | A5 | Baja |
| 1.2 | Paper §3.2 — flujos autorizados (16 pares) | A2 | Alta |
| 1.3 | Paper §3.2, Figura 2 — validación del financiador | A4 | Alta |
| 1.4 | Paper §3.3 — organización por establecimiento | A1 | Alta |
| 1.5 | Paper §3.3/§3.4/Referencias — referencia [15] | B1 | Alta |
| 1.6 | Paper §3.4 — alcance del aislamiento de información | A3 | Alta |
| 1.7 | Paper §3.4 — excepción de endoso en el registro inicial | E7 | Baja |
| 1.8 | Paper §3.2 — condición de custodia en reingreso a stock (negación invertida) | review PR #88 | Alta |
| 1.9 | Paper §3.4 + limitaciones de tesis — metadatos de relación no cubiertos por el aislamiento | review PR #88 | Media |
| 1.10 | Paper §3.4 + conclusiones — enunciar la propiedad acotada de no unilateralidad (custodia, unidad ajena, contraparte exigida) | review PR #88 | Alta |
| 2.1 | Tesis §2.1.2.1 + bibliografía — Resolución 435/2011 | B3 | Media |
| 2.2 | Tesis §2.1.3 — Disp. 7439/1999 y Dec. 1299/1997 | E2 | Media |
| 2.3 | Tesis §2.1.3.1 — acceso de auditoría de PAMI | B7 | Baja |
| 2.4 | Tesis §2.1.3.1 — inicio de trazabilidad por droguería | E1 | Alta |
| 2.5 | Tesis §2.1.3.2 — alcance del CUFE | E6 | Baja |
| 2.6 | Tesis Figura 2.1 (p. 14) — flujos no autorizados | A2 | Alta |
| 2.7.a | Tesis §2.1.5 / Figura 2.2 — nonce | B4 | Media |
| 2.7.b | Tesis §2.1.5.1 — firma digital y direcciones | B4 | Media |
| 2.7.c | Tesis §2.1.5.4 — PoW como anti-DoS | B4 | Media |
| 2.7.d | Tesis §2.1.6.2 — Raft ≠ PoA | B4 | Media |
| 2.8 | Tesis §2.1.6.1 — peers de anclaje duplicados | B5 | Baja |
| 2.9 | Tesis bibliografía — "Hyperledger 2020a–g" | B6 | Baja |
| 2.10 | Tesis Anexo A — placeholder y cronograma | B2 | Alta |
| 2.11 | Tesis Anexo B — instrucciones de plantilla / encuesta | B2 | Alta |
| 2.12 | Tesis capítulo de diseño — incorporar ADR-002/003/004/005 | A1, A3, A4 | Alta |

**Recordatorio**: al aplicar cada cambio, marcar la casilla "Aplicado" de su entrada en este archivo (y, si corresponde, dejar constancia en el hallazgo original de `consistency-review.md`). Una entrada sin marcar se considera pendiente. Si al aplicar un cambio se opta por una redacción distinta de la sugerida, anotarlo en la entrada para que la próxima revisión de congruencia no lo detecte como divergencia nueva.
