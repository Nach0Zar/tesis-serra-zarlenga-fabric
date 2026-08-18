# ADR-007: Topología física de la red del prototipo

- **Estado**: Propuesto
- **Fecha**: 2026-08-17
- **Autores**: Serra, Zarlenga

---

## Contexto

Las decisiones de diseño previas dejan definido el modelo lógico de la red — canal único con estado mínimo de trazabilidad público y PDC para información comercial (ADR-002), una organización Fabric por establecimiento con registro organización-establecimiento en el ledger (ADR-003), transferencia en dos fases con endoso conjunto de emisor y receptor en la recepción (ADR-004), y la tabla de endoso por clase de operación de DES-6 — pero ninguna fija la topología física: cuántos nodos orderer y de qué organizaciones, cuántos peers y con qué state database, cuántas CA, los nombres definitivos de canal y chaincode, el proceso de bootstrap del dataset mínimo, ni el mecanismo concreto con el que la plataforma exige los endosos que DES-6 diseña. Este ADR es la última pieza entre el diseño y cualquier `configtx.yaml` (decisión D2 de `docs/adr-roadmap.md`, issue #82).

Tres restricciones condicionan la decisión:

1. **Fidelidad al trabajo escrito**: el paper afirma que el esquema "habilita que distintas organizaciones contribuyan nodos al servicio de ordenamiento, reduciendo la dependencia de un único administrador central", y la eliminación del punto único de falla administrativo es uno de los argumentos de la hipótesis. La issue #82 identifica este punto como el de mayor riesgo de divergencia entre prototipo y trabajo escrito.
2. **Protocolo de medición**: la sección 8.1 de `docs/measurement-protocol.md` asume un cluster Raft de 3 orderers, con caída de 1 (Raft-1: la red conserva quorum) y de 2 (Raft-2: la red pierde quorum y deja de ordenar). La topología debe hacer que esos escenarios midan algo con significado, dentro de los límites que la issue #55 (DOC-7) fija para todo el trabajo: host único y fallos simulados, con valores indicativos del comportamiento relativo.
3. **Recursos del host de medición**: el prototipo corre en un único host (WSL2), por lo que cada componente adicional (peers, CouchDB, CA extra) compite por los recursos que el propio protocolo de medición necesita mantener estables.

Además, DES-6 delega explícitamente la materialización de su tabla de endoso ("NET-2, NET-5 y NET-6 deben decidir cómo expresarla en configuración Fabric"), ADR-003 deja pendiente la coordinación entre el alta en la configuración del canal y el alta en el registro, y NET-1 (#20) mantiene abierta la pregunta cryptogen vs. Fabric CA para el material criptográfico.

## Alternativas

### Servicio de ordenamiento

**A. Cluster Raft de 3 orderers repartidos entre 3 organizaciones distintas**

- `AnmatMSP` y las organizaciones de dos establecimientos del dataset mínimo aportan un nodo orderer cada una; cada una figura como organización de ordenamiento en la configuración del canal con su propia MSP de orderer.
- Materializa la afirmación del trabajo escrito: distintas organizaciones contribuyen nodos al servicio de ordenamiento y ninguna administra el ordenamiento en solitario.
- Da significado a los escenarios Raft-1/Raft-2 del protocolo de medición: cada nodo que cae pertenece a una organización de ordenamiento distinta, en lugar de ser un contenedor intercambiable del mismo administrador (sobre el alcance probatorio de esos escenarios, ver Justificación).
- Cuesta configuración: tres MSP de orderer, tres juegos de material criptográfico de ordenamiento y una sección de consenters multi-organización en `configtx.yaml`, en lugar del perfil de organización única de `test-network`.
- Se adopta.

**B. Cluster Raft de 3 orderers bajo una única organización de ordenamiento (default de `test-network`)**

- Más simple de levantar y de mantener; es el camino que la tooling de Fabric trae preconfigurado.
- La prueba de disponibilidad Raft seguiría siendo válida como demostración de tolerancia a fallas (CFT), pero la afirmación de descentralización del ordenamiento del trabajo escrito no tendría siquiera correspondencia estructural: los tres nodos pertenecerían a una única organización.
- Obligaría a incluir una sección "Divergencia con el trabajo escrito" que declare la simplificación y acote la conclusión correspondiente de la tesis.
- Se descarta porque el costo de configuración de la alternativa A es acotado y pagable, mientras que el costo de B es argumental y permanente: dejaría sin correspondencia estructural una de las afirmaciones de la hipótesis en el punto donde el prototipo sí puede representarla.

### State database de los peers

**A. LevelDB**

- El contrato congelado (`docs/api-contract.md`) no requiere rich queries: la única consulta por criterio, `QueryUnitsByGTIN`, usa `GetStateByPartialCompositeKey`, soportada nativamente por LevelDB.
- Corre embebida en el proceso del peer: sin contenedores adicionales, menor consumo de recursos en el host único de medición.
- Se adopta.

**B. CouchDB**

- Habilita rich queries JSON e índices, que ninguna operación del contrato vigente necesita.
- Agrega un contenedor por peer, con costo de memoria y de latencia de lectura/escritura que contaminaría la medición sin aportar funcionalidad usada.
- Se descarta porque no hay requisito que lo justifique. Si una iteración futura necesita consultas por custodio u otros selectores no expresables como clave compuesta, el cambio a CouchDB es una revisión de este ADR.

### Autoridad de certificación

**A. Una única instancia de Fabric CA para todas las organizaciones del prototipo**

- ADR-003 ya fundamenta que una instancia de Fabric CA puede emitir identidades para múltiples organizaciones.
- Reduce el número de contenedores y la complejidad de bootstrap en el host único.
- Concentra la raíz de confianza: quien opera la CA puede emitir identidades de cualquier organización. Es una simplificación consciente del prototipo, no un modelo productivo.
- Se adopta, con el límite de confianza declarado explícitamente (ver Decisión, punto 3).

**B. Una CA (o jerarquía de CA intermedias) por organización**

- Es el modelo productivo correcto: cada organización controla su propia raíz de confianza y ninguna puede emitir identidades ajenas.
- Multiplica contenedores y pasos de enrolamiento en un prototipo cuyo objetivo de medición (latencia, throughput, disponibilidad, confidencialidad) no depende de esa separación.
- Se descarta para el prototipo; queda documentado como el diseño esperado en producción.

### Materialización del endoso de DES-6

**A. Política de endoso estática de chaincode que enumere las combinaciones multiparte**

- Las políticas a nivel chaincode son fijas por despliegue: no pueden expresar "el emisor y el receptor de *esta* transferencia", que varían por unidad y por operación. Expresar la tabla de DES-6 exigiría políticas por par de organizaciones o un `AND` global que convertiría a todas en coendosantes de todo.
- Se descarta porque el endoso que DES-6 y ADR-004 exigen es dinámico por participantes, y una política estática no puede representarlo sin violar las propiedades que DES-6 fija (no exigir `AnmatMSP` en toda escritura, no permitir cambio de custodia con endoso solo del origen).

**B. Validación exclusivamente en lógica de chaincode, con política base laxa**

- El chaincode puede verificar que el invocador sea quien corresponde, pero no puede exigir por sí solo que *otra* organización haya endosado la transacción: con una política `OR`, una transacción endosada por un único peer sería válida para la plataforma aunque la garantía de diseño requiera dos firmas.
- Se descarta como mecanismo único porque no produce la evidencia empírica que NET-6 (#25) debe generar: una transacción sin los endosos requeridos debe ser *rechazada por la plataforma*, no solo desaconsejada por la lógica.

**C. Política base laxa + state-based endorsement (SBE) fijado por el chaincode**

- La política de chaincode queda como `OR` de las organizaciones custodiales; el chaincode fija políticas de endoso por clave (`SetStateValidationParameter`) sobre las claves públicas de las unidades, ajustándolas en cada transición. Cubre la parte de la tabla de DES-6 que es derivable del estado confirmado; el resto exige los mecanismos complementarios que fija el punto 6 de la Decisión.
- La plataforma valida los endosos por clave en la fase de validación: una transacción que modifique la clave sin las firmas requeridas se marca inválida (`ENDORSEMENT_POLICY_FAILURE`) y no se aplica al world state. Eso es exactamente la evidencia que NET-6 necesita.
- Se adopta.

## Decisión

1. **Servicio de ordenamiento**: cluster Raft de **3 nodos orderer aportados por 3 organizaciones distintas**: `AnmatMSP` (1 nodo) y las organizaciones de dos establecimientos del dataset mínimo — los ejemplos `LabMSP` y `DrogueriaMSP` de DES-6 — con 1 nodo cada una. Cada una figura como organización de ordenamiento en la configuración del canal, con su propia MSP de orderer. Ninguna organización administra el servicio de ordenamiento en solitario.

2. **Peers y state database**: 1 peer por organización custodial del dataset mínimo, más 1 peer para `AnmatMSP` y 1 peer para `FinanciadorMSP`. State database: **LevelDB** en todos los peers. El contrato no requiere rich queries (`QueryUnitsByGTIN` opera por clave compuesta parcial, soportada por LevelDB), y LevelDB reduce el consumo de recursos del host único de medición. Si una iteración futura necesita consultas por custodio u otros selectores, el cambio a CouchDB es una revisión de este ADR.

3. **Autoridad de certificación**: una **única instancia de Fabric CA** emite las identidades de todas las organizaciones del prototipo (peers, orderers, clientes, con los atributos `snt.role` de DES-6). **Límite de confianza declarado**: en producción cada organización operaría su propia CA (o CA intermedias bajo su control), porque una CA compartida concentra la raíz de confianza y su operador podría emitir identidades de cualquier organización; para el prototipo, el objetivo de la medición no depende de esa separación, y la simplificación debe listarse entre las limitaciones del trabajo. Se descarta **cryptogen** para el material criptográfico definitivo de la red: no representa el proceso de enrolamiento contra una CA y no ejercita la emisión de atributos; puede usarse únicamente para pruebas locales descartables. Esto resuelve la pregunta abierta de NET-1 (#20).

4. **Nombres definitivos**: el canal se llama **`snt-channel`** y el chaincode **`snt`**. Consecuencia: los workflows de CI que hoy usan `mychannel` (`.github/workflows/chaincode-integration.yml`) deben actualizarse cuando exista la red propia del prototipo. Esa actualización pertenece a NET-4/CI; este ADR no modifica los workflows.

5. **Bootstrap del dataset mínimo**: secuencia ordenada, ejecutada por los scripts de red (NET-2/NET-4):
   - (a) generación del material criptográfico de todas las organizaciones (custodiales, `AnmatMSP`, `FinanciadorMSP` y las tres MSP de orderer) vía Fabric CA;
   - (b) génesis y creación de `snt-channel` con las 3 organizaciones de ordenamiento y todas las organizaciones de peers;
   - (c) **secuencia 1 de lifecycle — despliegue de bootstrap**: instalación del paquete del chaincode `snt` (que embebe la matriz de ADR-008 y el manifiesto fundacional de ADR-010), `approveformyorg` de cada organización sobre ese `packageID`, y `commit` de la definición con el `collections_config.json` generado conforme ADR-006, `--init-required` y **política de endoso `AND` de todas las organizaciones fundacionales** (ADR-010, punto 4);
   - (d) invocación de `Init` por una identidad de la organización regulatoria con `snt.role=regulatory-admin`, que siembra la entrada `REGULATOR` del registro y fija su protección por SBE. Es la única transacción que se ejecuta bajo la política estricta;
   - (e) **secuencia 2 de lifecycle — definición operativa**: nueva secuencia con la misma versión de paquete y la **política de endoso operativa** (`OR` laxa de las organizaciones custodiales, punto 6);
   - (f) seed del resto del registro organización-establecimiento: una invocación de `RegisterOrganization` por cada organización custodial y por cada organización no custodial restante (financiadores), ejecutada por una identidad de la organización regulatoria con `snt.role=regulatory-admin`.

   La secuencia materializa la coordinación que ADR-003 exige: ninguna organización queda activa en el registro sin estar incorporada al canal, ni viceversa. Los pasos (c) a (e) materializan el bootstrap en dos etapas que decide ADR-010: la semilla regulatoria se confirma bajo unanimidad de las organizaciones fundacionales y anclada al `packageID` que todas aprobaron, y recién después la red pasa a la política laxa que el punto 6 necesita.

6. **Materialización del endoso (para NET-6)**: política de endoso de chaincode base **laxa** (`OR` de las organizaciones custodiales del dataset mínimo) combinada con **state-based endorsement (SBE)** aplicado por el chaincode sobre la clave pública de cada unidad y sobre las claves de gobernanza.

   **Dos reglas de plataforma condicionan todo el diseño.**

   - Fabric valida una transacción contra la política de endoso que la clave tenía **antes** de esa transacción. Una política escrita con `SetStateValidationParameter` rige las escrituras **posteriores**, nunca la transacción que la establece. SBE solo puede expresar, por lo tanto, requisitos derivables del estado ya confirmado.
   - **La política es de la clave, no de la función.** Fabric no sabe qué función del chaincode produjo una escritura: evalúa la política de cada clave escrita contra el conjunto de endosos de la transacción. En consecuencia, **toda organización que aparezca en una rama satisfacible de la política de una clave puede endosar cualquier operación que escriba esa clave**. Una rama disyuntiva agregada para habilitar un caso excepcional habilita, con la misma fuerza, todos los casos ordinarios.

   La segunda regla corrige un error de una versión anterior de este ADR, que fijaba una política de reposo `OR(<org del custodio>, AnmatMSP)` para dejar espacio a los eventos regulatorios que ADR-001 habilita a ANMAT. Esa rama disyuntiva permitía que una `Dispense` de una farmacia quedara endosada únicamente por el peer de ANMAT, y que una `ReceiveTransfer` satisficiera la política de tránsito sin el par emisor–receptor: exactamente lo contrario de lo que DES-6 exige y de lo que NET-6 debe demostrar. **Ninguna política de clave de este diseño admite a `AnmatMSP` como rama alternativa a las organizaciones de la operación.**

   a. **Política de reposo de la clave de la unidad**: `<org del custodio actual>`. La fija `RegisterUnit` (T01) al crear la clave y la restablece toda transición que cambie el custodio registrado o que resuelva el tránsito. Es la traducción exacta de la fila de DES-6 sobre eventos ordinarios y dispensación: el peer del custodio ejecuta y endosa toda escritura sobre su unidad.

   b. **Política de tránsito**: en el **despacho** (T02/T03) el chaincode fija sobre la clave de la unidad `AND(<org emisora>, <org receptora declarada>)`. Es el caso en que SBE sí puede expresar un requisito multiparte, porque el despacho **deja declarado en el estado** quién es la contraparte. Materializa la propiedad central de DES-6 y de ADR-004: la transacción que resuelve el tránsito —recepción, rechazo o evento extraordinario— exige que ambos peers ejecuten la misma validación y coincidan. Sin rama alternativa: no hay tercero que pueda sustituir a una de las partes.

   c. **Restauración en toda salida de `EN_TRANSITO`**, no solo en la recepción. Cada transición que resuelve el tránsito restablece la política de reposo del punto (a) sobre el custodio que corresponda:
      - recepción (T04): custodio registrado pasa a ser el receptor → `<org receptora>`;
      - rechazo (T05): la custodia permanece en el emisor (ADR-004) → `<org emisora>`;
      - eventos extraordinarios que cierran el tránsito (T09, T13–T16): la custodia registrada permanece en el emisor → `<org emisora>`.

      Omitir esta restauración dejaría la unidad indefinidamente bajo una política que exige al receptor de un despacho ya resuelto: un bloqueo permanente de las operaciones ordinarias del custodio. Es un requisito de corrección del chaincode, no una optimización, y CC-3/CC-5 deben cubrirlo con tests.

   d. **Cómo ejerce ANMAT sus facultades regulatorias sin rama propia.** ADR-001 habilita a ANMAT a ejecutar eventos extraordinarios y regulatorios sobre unidades en custodia ajena (T09, T13–T20, T22/T23, T27, T29/T31/T32). Con la política de reposo del punto (a), esas transacciones son válidas cuando ANMAT es el **creador** de la transacción y el peer del custodio actual la **endosa**:
      - la identidad del iniciador está garantizada por la plataforma: la firma del creador se verifica en la validación de la transacción, y la lógica del chaincode resuelve `cid.GetMSPID()` contra el registro y exige `agentType=REGULATOR` con `snt.role=regulatory-admin`;
      - la participación del custodio está garantizada por la política de la clave.

      Esto **es** la fila de DES-6 «evento regulatorio iniciado por ANMAT → `AnmatMSP` y, cuando la operación afecte custodia o datos privados de un establecimiento, la organización custodia involucrada», materializada de la forma más directa. Y no exige del custodio ninguna decisión: un peer no "consiente", ejecuta la lógica determinística y endosa si el resultado es válido. La cooperación necesaria es la de una máquina en funcionamiento, no la de un administrador.

      **Consecuencia de disponibilidad, asumida y declarada**: si el peer del custodio está caído o la organización fue removida del canal, ANMAT no puede ejecutar el evento regulatorio sobre esa unidad; durante el tránsito la exigencia es más fuerte todavía, porque la política del punto (b) requiere a las dos partes. La contrapartida es la propiedad que el trabajo persigue: **ninguna organización, tampoco la autoridad de aplicación, puede modificar por sí sola el estado registrado de una unidad**. Se elige la garantía por sobre la disponibilidad, y el costo se lista entre las limitaciones del prototipo (`docs/alcance-prototipo.md`).

   e. **Estados terminales** (`DISPENSADO`, `DISPUESTO_FINAL`, `PROHIBIDO`): la política queda en el valor de reposo del último custodio registrado. No se endurece porque la máquina de estados de ADR-001 no admite ninguna transición de salida y es la lógica del chaincode —no la política de clave— la que rechaza el intento con `INVALID_STATE_TRANSITION`.

   f. **Operaciones cuyo endoso no es derivable del estado: clave de autorización acompañante.** Cuando una regla de DES-6 exige un conjunto de endosantes que depende de la operación intentada y no del estado previo, la exigencia se traslada a una **clave de autorización separada**, creada por una transacción previa y protegida con la política que la regla pide. La operación que consume la autorización debe **escribir** (borrar) esa clave, con lo que su transacción queda obligada a satisfacer esa política.

      La regla de DES-6 «retiro, recupero o disposición final iniciado por laboratorio no custodio → laboratorio invocante y `AnmatMSP`» se materializa así: ANMAT emite `AuthorizeLabIntervention` (contrato DES-5), que crea la clave `LabIntervention`+[`gtin`,`numeroSerie`] y le fija SBE **`AND(<org del laboratorio designado>, AnmatMSP)`**. La operación posterior del laboratorio lee esa clave y la consume borrándola, de modo que la plataforma exige los dos endosos que la fila pide. La política de la clave de autorización **no es `AnmatMSP` a secas**: eso probaría la participación de ANMAT y dejaría la del laboratorio apoyada solo en la firma de creador, que acredita identidad e iniciativa pero no es un endoso de peer — corrección respecto de una versión anterior de este ADR.

      La transacción del laboratorio escribe además la clave de la unidad, sujeta a la política de reposo del punto (a). El resultado es un endoso de **tres** organizaciones: laboratorio designado, `AnmatMSP` y custodio actual. Es un endurecimiento respecto de la letra de la fila de DES-6, coherente con su propia justificación —"validar la legitimidad del reclamo antes de mutar un asset bajo custodia ajena"—, y queda incorporado como salvedad en `docs/organizations-roles-endorsement.md`.

      Se descarta armar la clave de la **unidad** con `AND(<org laboratorio>, AnmatMSP)` mientras la autorización esté pendiente: bloquearía las operaciones ordinarias del custodio sobre su propia unidad durante toda la vigencia de la autorización. La clave acompañante deja la política de reposo intacta y solo grava la transacción que ejerce la autorización.

      La autorización lleva vencimiento (`expiraEn`, contrato DES-5) para que una autorización no ejercida no quede como permiso permanente. v1 **no** incorpora revocación anticipada; además, como la clave queda protegida por `AND(laboratorio, AnmatMSP)`, reemplazarla antes de su vencimiento por otra dirigida a un laboratorio distinto exige el endoso del laboratorio designado originalmente. Ambas fricciones se acotan fijando ventanas de vigencia cortas y se declaran en el alcance.

   g. **La ventana de creación de una clave, y el caso `RegisterUnit`.** Una clave que todavía no existe no tiene política propia: su primera escritura se valida contra la **política de chaincode** (`OR` de las organizaciones custodiales). La política que el chaincode fija en esa misma transacción rige recién a partir de la siguiente. Por lo tanto:
      - la exclusividad que DES-6 pide para el registro inicial de unidades («endoso del laboratorio invocante») **no la impone la plataforma**: cualquier peer custodial puede endosar un T01. Lo que sí garantiza la plataforma es que el **creador** de la transacción es una identidad de un laboratorio registrado —la firma de creador se verifica siempre— y que un alta duplicada es rechazada por la validación MVCC, porque su conjunto de lectura declara la clave inexistente;
      - el riesgo residual es acotado y debe declararse: un endosante malicioso podría hacer pasar un alta que viola reglas de formato o de habilitación (dígito verificador, formato de serie, laboratorio inactivo), siempre en connivencia con un laboratorio real que firma la propuesta. No puede suplantar a un laboratorio ni duplicar una unidad;
      - **alternativa evaluada y descartada**: obligar a que cada T01 escriba además una clave por laboratorio protegida con SBE del laboratorio (por ejemplo un contador de altas), lo que forzaría su endoso. Se descarta porque serializaría por conflicto MVCC la operación de mayor volumen del prototipo —el alta de las 50.000 unidades del dataset del protocolo de medición— y distorsionaría precisamente la medición que el trabajo existe para producir. Endurecer la política de chaincode a `MAJORITY` se descarta por la misma razón y porque tampoco garantizaría la presencia del laboratorio;
      - la misma ventana existe para las claves de gobernanza que ANMAT crea (entradas del registro, autorizaciones de intervención), con el mismo mitigante: el creador debe ser una identidad `REGULATOR` y la clave queda protegida por SBE desde esa misma transacción. **No** existe ventana equivalente para los datos privados: la política de endoso de la colección se define en el despliegue y rige desde la primera escritura.

   h. **Qué no queda cubierto por SBE, y se declara.** Las reglas de DES-6 que dependen del invocador y no del estado —que quien dispensa sea el custodio con `agentType` habilitado, que quien informa un evento extraordinario sea el custodio y no ANMAT, que el laboratorio que interviene sea el titular de la unidad, que el alta la inicie un laboratorio— se validan en la **lógica del chaincode**, ejecutada de forma idéntica por todos los peers endosantes, sobre una identidad de creador que la plataforma verifica criptográficamente. SBE fija el conjunto de organizaciones que deben ejecutar y coincidir; la lógica restringe qué puede hacer cada invocador. La evidencia que NET-6 debe producir distingue ambos planos: rechazo por la plataforma (`ENDORSEMENT_POLICY_FAILURE`, transacción inválida en el bloque) para lo materializado por política, y error tipificado del contrato para lo materializado por lógica.

   Se elige SBE y no solo validación en lógica de chaincode para las reglas del primer plano porque SBE hace que la **plataforma** rechace en fase de validación toda transacción que carezca de los endosos requeridos sobre la clave, en lugar de depender de que la lógica se haya ejecutado en los peers correctos. Ese rechazo verificable es la evidencia empírica que NET-6 (#25) debe producir.

Queda fuera de alcance de este ADR: el contenido de `collections_config.json` (ADR-006), la entrada registral de las organizaciones no custodiales (ADR-010), la modificación de los workflows de CI (NET-4) y el ajuste de recursos del host de medición (NET-3).

## Justificación

La decisión de ordenamiento es la única de las seis con riesgo argumental. El trabajo escrito sostiene que distintas organizaciones contribuyen nodos al servicio de ordenamiento, reduciendo la dependencia de un único administrador central, y usa la eliminación del punto único de falla como argumento de la hipótesis. Repartir los 3 orderers entre `AnmatMSP`, `LabMSP` y `DrogueriaMSP` hace que la **estructura** del prototipo se corresponda con esa afirmación, en lugar de simplificarla: el consenso queda repartido entre tres MSP de ordenamiento distintas y ninguna organización controla por sí sola el servicio.

**Qué demuestra y qué no demuestra la evidencia experimental.** La descentralización administrativa así entendida es una propiedad **estructural**, verificable en la configuración del canal y en la distribución de identidades de ordenamiento, no una propiedad que los escenarios de disponibilidad midan. Lo que los escenarios Raft-1 y Raft-2 del protocolo de medición demuestran empíricamente es la **tolerancia a fallas por caída de nodos de ordenamiento lógicamente asignados a organizaciones distintas**, en un entorno de host único con fallos simulados a nivel de contenedor: la red conserva quorum con un nodo caído y deja de ordenar con dos. No demuestran la caída de una organización completa —que implicaría infraestructura, operadores y dominios de falla independientes—, ni la independencia administrativa real, que además queda acotada por la CA única del punto 3. Esta distinción es la que fija la issue #55 (DOC-7 · Limitaciones) para todo el trabajo: entorno single-host, fallos simulados y no de infraestructura física distribuida, valores indicativos del comportamiento relativo y no extrapolables a producción. La tesis debe presentar la descentralización del ordenamiento como propiedad de diseño respaldada por la configuración, y los escenarios Raft como evidencia de tolerancia a fallas bajo esa configuración — no como prueba de resiliencia organizacional.

La alternativa de organización única habría debilitado incluso esa lectura estructural a cambio de un ahorro de configuración modesto; se prefiere pagar el costo de configuración.

LevelDB y la CA única responden al mismo criterio: no cargar el host único de medición con componentes que el contrato congelado y el objetivo de la medición no necesitan. En el caso de la CA, la simplificación toca la raíz de confianza y por eso se declara explícitamente como límite, con el modelo productivo (una CA por organización) documentado. En el caso de LevelDB, la simplificación es reversible por revisión de este ADR y no toca ninguna garantía.

El descarte de cryptogen para el material definitivo es coherente con DES-6: los certificados de cliente deben portar el atributo `snt.role`, que es parte del flujo de registro y enrolamiento de Fabric CA; cryptogen no representa ese proceso.

**Sobre el endoso: qué expresa SBE y qué no.** SBE es el único mecanismo nativo de Fabric que expresa políticas por participante y por unidad, que es la forma que tiene la tabla de DES-6. Pero lo hace con dos restricciones que este ADR tuvo que incorporar tras sucesivas correcciones, porque cada una invalidaba un tramo del diseño anterior:

- **La política se evalúa contra el estado previo.** Una versión anterior prometía fijar coendosos regulatorios «cuando la operación lo requiere»; es imposible, porque la política que la transacción escribe no rige a esa misma transacción. De ahí el patrón de la clave de autorización acompañante (punto 6.f).
- **La política es de la clave, no de la función.** Una versión posterior agregó una rama `AnmatMSP` a la política de reposo para dejar espacio a los eventos regulatorios. Como Fabric no sabe qué función produjo una escritura, esa rama habilitaba también las operaciones ordinarias: una dispensación endosada solo por el peer del regulador, una recepción que satisfacía la política de tránsito sin el par emisor–receptor. La rama desaparece (punto 6.a/6.b).

La consecuencia de la segunda corrección es la que más aporta al argumento del trabajo, y conviene enunciarla como propiedad y no como restricción: **ninguna organización, tampoco la autoridad de aplicación, puede modificar por sí sola el estado registrado de una unidad.** El custodio no puede transferir sin que el receptor ejecute; el receptor no puede recibir sin que el emisor ejecute; ANMAT no puede intervenir sin que ejecute el peer del custodio; un laboratorio no custodio no puede retirar del mercado una unidad ajena sin que ejecuten el regulador y el custodio. El precio es de disponibilidad —una intervención regulatoria depende de que el peer del custodio esté en línea— y se asume explícitamente en el punto 6.d.

Las reglas de DES-6 se materializan, entonces, por tres caminos distintos y bien delimitados: SBE sobre la clave de la unidad cuando el conjunto de endosantes es derivable del estado (reposo y tránsito); SBE sobre una clave de autorización acompañante cuando depende de la operación intentada (intervención de laboratorio no custodio); y lógica de chaincode sobre la identidad de creador verificada por la plataforma cuando depende de quién invoca. Las tres preservan las propiedades que DES-6 obliga a conservar: no convierten a `AnmatMSP` en coendosante universal, no permiten que una transferencia cambie custodia con endoso solo del origen, y no requieren una MSP por categoría. Lo que ninguna de las tres alcanza —la ventana de creación de una clave, con el registro inicial de unidades como caso principal— se declara en el punto 6.g en lugar de presentarse como resuelto.

## Consecuencias

- **NET-1 (#20)**: resuelto el interrogante cryptogen vs. Fabric CA — Fabric CA para el material definitivo; cryptogen solo para pruebas locales descartables.
- **NET-2 (#21)**: el `configtx.yaml` debe definir 3 organizaciones de ordenamiento (`AnmatMSP` + `LabMSP` + `DrogueriaMSP`, cada una con MSP de orderer propia), los consenters Raft correspondientes y las organizaciones de peers del dataset mínimo, con canal `snt-channel`.
- **NET-3 (#22)**: el dimensionamiento del host debe presupuestar 3 orderers, los peers del punto 2 y una CA; LevelDB evita contenedores CouchDB adicionales.
- **NET-4 (#23)**: los scripts de despliegue implementan la secuencia de bootstrap del punto 5; los workflows de CI migran de `mychannel` a `snt-channel` cuando la red propia reemplace a `test-network`.
- **NET-6 (#25)**: la demo de endoso se implementa con política base `OR` + SBE conforme el punto 6, y debe evidenciar el rechazo por la plataforma de: (i) una recepción o un rechazo endosados por una sola de las dos partes del tránsito; (ii) una dispensación endosada por un peer que no sea el del custodio; (iii) un evento regulatorio de ANMAT sin el endoso del peer del custodio; (iv) una operación de laboratorio no custodio sin el endoso conjunto del laboratorio y del regulador, que no puede borrar la clave de autorización. Debe además demostrar la restauración de la política de reposo en las tres salidas de `EN_TRANSITO` (punto 6.c), documentar la ventana de creación del punto 6.g como límite conocido, y separar en su evidencia el rechazo por política (`ENDORSEMENT_POLICY_FAILURE`) del rechazo por lógica (error tipificado del contrato).
- **CC-3 (#16) y CC-5**: además de las transiciones, deben implementar y testear la gestión de la política de clave — fijación en `RegisterUnit`, armado en el despacho y **restauración en las tres salidas de `EN_TRANSITO`** (punto 6.c). Una salida sin restauración deja la unidad bloqueada de forma permanente.
- **Para el contrato DES-5**: el punto 6.f introduce la operación `AuthorizeLabIntervention`, sin la cual la fila de DES-6 sobre intervención de laboratorio no custodio no es materializable, y fija la política de su clave en `AND(laboratorio, AnmatMSP)`. El contrato debe además reflejar que un evento regulatorio iniciado por ANMAT exige el endoso del peer del custodio actual, y ambas partes durante el tránsito.
- **Protocolo de medición (§8.1)**: los escenarios Raft-1/Raft-2 conservan validez; cada caída afecta al nodo de una organización de ordenamiento distinta, con el alcance interpretativo acotado en la Justificación y por la issue #55 (host único, fallos simulados).
- **Se gana**: correspondencia estructural con la afirmación de descentralización del ordenamiento del trabajo escrito (verificable en la configuración del canal, no en la medición); parámetros concretos que desbloquean NET-1/2/3/4/6; endoso multiparte exigido por la plataforma (evidencia para NET-6); y la propiedad que el punto 6 hace verificable: **ninguna organización, tampoco la autoridad de aplicación, puede modificar por sí sola el estado registrado de una unidad**; menor huella de recursos en el host de medición (LevelDB, CA única).
- **Se pierde / costo**: la configuración de ordenamiento multi-organización es más compleja que el default de `test-network` (tres MSP de orderer, material criptográfico y consenters por organización); la CA única concentra la raíz de confianza y debe listarse como limitación del prototipo; LevelDB cierra la puerta a rich queries hasta una revisión de este ADR; y toda escritura sobre una unidad —incluida la intervención regulatoria— queda subordinada a la disponibilidad del peer del custodio actual, o de ambas partes durante el tránsito (punto 6.d).
- **Queda pendiente**: el ajuste fino de recursos del host de medición (`.wslconfig`) se resuelve en NET-3; la entrada registral de las organizaciones no custodiales (incluida la eventual entrada de `AnmatMSP`) se resuelve en ADR-010.

## Divergencia con el trabajo escrito

- **Servicio de ordenamiento**: **no hay divergencia estructural**. Se eligió deliberadamente la opción fiel a la afirmación del trabajo escrito ("distintas organizaciones contribuyen nodos al servicio de ordenamiento, reduciendo la dependencia de un único administrador central") y la topología adoptada se corresponde con ella. Sí debe acotarse el **alcance probatorio**: los escenarios Raft evidencian tolerancia a fallas de nodos en un host único con fallos simulados, no independencia administrativa ni resiliencia organizacional (issue #55). La tesis debe presentar la descentralización como propiedad de diseño y los escenarios como evidencia de tolerancia a fallas, sin extrapolar.
- **CA única**: es una **simplificación** respecto del modelo de confianza que una red productiva multi-organización requeriría (cada organización con su propia CA). No contradice una afirmación puntual del trabajo escrito, pero debe listarse explícitamente entre las limitaciones del prototipo en la próxima iteración del documento, junto con la nota de que en producción la raíz de confianza estaría distribuida.

## Contexto utilizado

- Issue GitHub #82: DES-13 · ADR-007: Topología física de la red del prototipo, consultada el 2026-08-17.
- [ADR-002: Topología de canales en la red Hyperledger Fabric](002-topologia-canales.md): canal único, estado mínimo público y PDC; marco que esta topología física implementa.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): una organización por establecimiento, registro organización-establecimiento, viabilidad de una CA para múltiples organizaciones y coordinación alta-en-canal/alta-en-registro.
- [ADR-004: Transferencia como despacho y recepción en dos transacciones](004-transfer-dispatch-reception.md), sección "Endoso": la recepción requiere endoso conjunto de emisor y receptor; base del SBE del punto 6.
- [ADR-006: Mecanismo de colecciones privadas para la información comercial y el registro de operación](006-private-data-collections.md) (D1, issue #81): fuente del `collections_config.json` que consume el bootstrap.
- [DES-6: Organizaciones, MSP, roles, ABAC y políticas de endoso](../organizations-roles-endorsement.md): dataset mínimo de MSP de ejemplo, tabla de endoso por clase de operación y propiedades que la materialización debe preservar.
- [Protocolo de medición](../measurement-protocol.md), sección 8.1: escenarios Raft-1/Raft-2/Peer-1 sobre cluster de 3 orderers.
- [Roadmap de ADRs](../adr-roadmap.md), decisión D2: alcance de esta decisión y riesgo de divergencia con el trabajo escrito.
- [Contrato público del chaincode](../api-contract.md) (v2.2.0): `QueryUnitsByGTIN` por clave compuesta parcial (sin rich queries), `RegisterOrganization` para el seed del registro, `Init` para el bootstrap regulatorio y `AuthorizeLabIntervention` para el punto 6.e.
- Hyperledger Fabric, The Ordering Service (Raft): https://hyperledger-fabric.readthedocs.io/en/release-2.5/orderer/ordering_service.html — cluster Raft, quorum y contribución de orderers por múltiples organizaciones.
- Hyperledger Fabric, Endorsement policies (state-based endorsement): https://hyperledger-fabric.readthedocs.io/en/release-2.5/endorsement-policies.html — políticas por clave fijadas desde chaincode, su precedencia sobre la política de chaincode y sobre la política de colección, y la regla que fundamenta el punto 6: la validación de una transacción usa la política que la clave tenía **antes** de esa transacción, de modo que una política escrita con `SetStateValidationParameter` solo rige las escrituras posteriores.
- Hyperledger Fabric, Transaction Flow: https://hyperledger-fabric.readthedocs.io/en/release-2.5/txflow.html — separación entre firma del creador de la transacción y endosos recogidos; fundamento del punto 6.g (SBE no distingue invocador de endosante).
- Hyperledger Fabric CA, Users Guide: https://hyperledger-fabric-ca.readthedocs.io/en/latest/users-guide.html — registro, enrolamiento y emisión de atributos; base del descarte de cryptogen.
