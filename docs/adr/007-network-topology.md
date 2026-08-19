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

- La política de chaincode queda como `OR` de las organizaciones custodiales; el chaincode fija políticas de endoso por clave (`SetStateValidationParameter`) sobre las claves públicas de las unidades, ajustándolas en cada transición. Cubre la parte de la tabla de DES-6 que es derivable del estado confirmado; el resto —lo que depende de la operación intentada y lo que debe regir ya en la primera escritura de una clave— se materializa con marcadores en las colecciones implícitas por organización, conforme el punto 6 de la Decisión.
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

6. **Materialización del endoso (para NET-6)**. El diseño combina tres mecanismos nativos de Fabric, cada uno con un alcance distinto: la política de endoso de **chaincode** (laxa, `OR` de las organizaciones custodiales), el **state-based endorsement (SBE)** por clave pública, y la **política de endoso de las colecciones implícitas por organización**.

   **Tres reglas de plataforma condicionan el diseño.**

   - Fabric valida una transacción contra la política de endoso que la clave tenía **antes** de esa transacción. Una política escrita con `SetStateValidationParameter` rige las escrituras **posteriores**, nunca la transacción que la establece. SBE solo puede expresar requisitos derivables del estado ya confirmado, y **no puede aplicarse a la primera escritura de una clave**, que se valida contra la política de chaincode.
   - **La política es de la clave, no de la función.** Fabric no sabe qué función del chaincode produjo una escritura: evalúa la política de cada clave escrita contra el conjunto de endosos de la transacción. Toda organización que aparezca en una rama satisfacible puede endosar cualquier operación que escriba esa clave; una rama disyuntiva agregada para un caso excepcional habilita, con la misma fuerza, todos los casos ordinarios.
   - **Escribir en una colección arrastra su política de endoso.** Cada organización del canal tiene una colección implícita `_implicit_org_<MSPID>` cuya política de endoso es la de su organización dueña, fija y vigente **desde el despliegue** — no depende de que una clave exista ni de una escritura previa. Una transacción que escriba cualquier clave de esa colección debe, por lo tanto, llevar el endoso de esa organización.

   La tercera regla es la pieza que faltaba. ADR-006 la había registrado como **objeción** —escribir el registro de operación en las colecciones implícitas del receptor y de ANMAT convertiría el despacho en una operación multiparte con ANMAT como coendosante obligatorio— y esa objeción sigue siendo válida ahí, donde el coendoso **no** se quiere. Este ADR usa la misma propiedad con el signo opuesto, exactamente donde el coendoso **sí** se quiere y ningún otro mecanismo puede imponerlo.

   **Marcador de participación.** Se define un patrón único, aplicable a toda regla de DES-6 que exija el endoso de una organización que no es la titular de la clave escrita: la transacción escribe, en la colección implícita de esa organización, un **marcador** con clave única por unidad y por evento (`Participacion`+[`gtin`,`numeroSerie`,`txId`]) y contenido determinístico calculado por el chaincode — operación, `mspId` del invocador y timestamp de la transacción. El marcador no transporta información de negocio, no requiere `transient` ni intervención del cliente, y su único propósito es someter la transacción a la política de endoso de esa colección. Propiedades que lo hacen apto:

   - **fuerza el endoso desde la primera escritura**, sin la ventana que afecta a toda clave pública nueva;
   - **no serializa**: la clave es única por unidad y evento, con lo que no hay contención MVCC — a diferencia de un contador o registro compartido por organización;
   - **no altera la clave pública ni su SBE**, que siguen expresando la custodia;
   - **es auditable**: la colección implícita de la organización conserva el marcador, y el ledger común conserva su hash.

   Sobre esa base:

   a. **Política de reposo de la clave de la unidad**: `<org del custodio actual>`, **sin rama alternativa**. La fija `RegisterUnit` (T01) y la restablece toda transición que cambie el custodio registrado o resuelva el tránsito. Es la traducción exacta de las filas de DES-6 sobre operaciones ordinarias y dispensación: el peer del custodio ejecuta y endosa toda escritura sobre su unidad. No incluye a `AnmatMSP`: incluirla habilitaría el endoso unilateral del regulador en las operaciones ordinarias, por la segunda regla.

   b. **Política de tránsito**: el **despacho** (T02/T03) fija sobre la clave de la unidad `AND(<org emisora>, <org receptora declarada>)`. Es el caso en que SBE sí puede expresar un requisito multiparte, porque el despacho deja declarada la contraparte en el estado. Materializa la propiedad central de DES-6 y de ADR-004 —la transacción que resuelve el tránsito exige que ambos peers ejecuten y coincidan— y no admite sustituto: ningún tercero puede reemplazar a una de las partes.

   c. **Restauración en toda salida de `EN_TRANSITO`**, no solo en la recepción:
      - recepción (T04): custodio registrado pasa a ser el receptor → `<org receptora>`;
      - rechazo (T05): la custodia permanece en el emisor (ADR-004) → `<org emisora>`;
      - eventos extraordinarios que cierran el tránsito (T09, T13–T16): la custodia permanece en el emisor → `<org emisora>`.

      Omitirla dejaría la unidad bajo una política que exige al receptor de un despacho ya resuelto: bloqueo permanente de las operaciones del custodio. Es requisito de corrección, y CC-3/CC-5 deben cubrirlo con tests.

   d. **Eventos regulatorios iniciados por ANMAT** (T09, T13–T20, T22/T23, T27, T29/T31/T32). La transacción escribe un marcador de participación en `_implicit_org_<mspId regulatorio>`. Endosantes exigidos por la plataforma: **la organización regulatoria** (política de su colección implícita) **y el custodio actual** (SBE de la clave de la unidad); durante el tránsito, ambas partes en lugar del custodio. Es la fila de DES-6 «`AnmatMSP` y, cuando la operación afecte custodia o datos privados de un establecimiento, la organización custodia involucrada», materializada como **coendoso real de peers** y no como firma de creador. Corrige una versión anterior de este ADR que hacía descansar la participación de ANMAT únicamente en su firma de creador: esa firma acredita identidad e iniciativa —y la lógica del chaincode sigue exigiéndola, resolviendo `cid.GetMSPID()` como `agentType=REGULATOR` con `snt.role=regulatory-admin`—, pero no es un endoso de peer, distinción que este ADR ya aplicaba al laboratorio y que no aplicaba a ANMAT.

   e. **Intervención de un laboratorio no custodio** (retiro, recupero o disposición final sobre una unidad ajena; fila correspondiente de DES-6). Se resuelve en dos transacciones:
      - **Autorización previa**: `AuthorizeLabIntervention` (contrato DES-5), invocada por la organización regulatoria, escribe la clave pública `LabIntervention`+[`gtin`,`numeroSerie`] con SBE **`AnmatMSP`** y un marcador de participación en la colección implícita regulatoria — que fuerza el endoso del regulador ya en esa primera escritura. La SBE de la clave de autorización es de la organización regulatoria **solamente**: así la autoridad puede reemplazarla o corregirla sin depender de nadie (ver punto f).
      - **Ejercicio**: la operación del laboratorio lee la autorización, verifica laboratorio, operación y vigencia contra `GetTxTimestamp()`, la marca como consumida y escribe marcadores de participación en **su propia** colección implícita y en la **regulatoria**. Endosantes exigidos por la plataforma: **laboratorio designado** (su colección), **organización regulatoria** (su colección y la SBE de la clave de autorización) y **custodio actual** (SBE de la clave de la unidad). El par «laboratorio invocante y `AnmatMSP`» de DES-6 queda impuesto como coendoso de peers; el endoso adicional del custodio es un endurecimiento coherente con la justificación de esa misma fila —no mutar un asset bajo custodia ajena sin validación— y se registra como salvedad en DES-6.

   f. **Vigencia y reemplazo de la autorización.** `expiraEn` es un campo que la **lógica** del chaincode evalúa contra `GetTxTimestamp()`: al vencer, la autorización deja de ser ejercible, pero la clave y su SBE siguen existiendo — el vencimiento no borra nada. Por eso la SBE de esa clave **no** puede incluir al laboratorio designado: si lo incluyera, reemplazar una autorización vencida por otra dirigida a un laboratorio distinto exigiría el endoso del laboratorio anterior, y si ese laboratorio se negara, perdiera su peer o saliera del canal, la unidad quedaría **impedida de forma permanente** de recibir cualquier autorización nueva. Corrige una versión anterior de este ADR, que fijaba SBE `AND(laboratorio, AnmatMSP)` sobre esa clave y trataba el vencimiento como si liberase el permiso. Con SBE regulatoria, la autoridad reemplaza o revoca en cualquier momento, y la participación del laboratorio se exige donde corresponde: en la transacción que **ejerce** la autorización, mediante su marcador.

   g. **Registro inicial de unidades (T01).** La clave pública de la unidad no existe todavía, con lo que su primera escritura no puede quedar cubierta por SBE. `RegisterUnit` escribe, además de la clave pública, un marcador de participación en la colección implícita del laboratorio invocante: la política de esa colección **exige el endoso del peer del laboratorio ya en esa primera transacción**. Con eso, la fila de DES-6 «registro inicial de unidades → laboratorio invocante» queda materializada por la plataforma, y no solo por la firma de creador. Precisión que corresponde conservar: la política de chaincode (`OR` de las custodiales) sigue admitiendo endosantes adicionales; lo que la plataforma garantiza es que el laboratorio es **necesario**, no que sea el único. Endosantes de más solo agregan ejecuciones coincidentes de la misma lógica determinística.

      La misma técnica cierra la ventana de creación de las claves de gobernanza que crea la organización regulatoria (`RegisterOrganization`, `SetOrganizationActive`, `AuthorizeLabIntervention`): todas escriben un marcador en la colección implícita regulatoria. `Init` no lo necesita: se ejecuta bajo la política estricta `AND` de todas las organizaciones fundacionales (ADR-010).

      **Alternativa descartada**: forzar el endoso del laboratorio con una clave pública compartida por organización (por ejemplo un contador de altas) protegida con su SBE. Se descarta porque serializaría por conflicto MVCC la operación de mayor volumen del prototipo —el alta de las 50.000 unidades del dataset del protocolo de medición— y distorsionaría la medición que el trabajo existe para producir. El marcador en colección implícita no tiene ese problema: su clave es única por unidad.

      **Costo asumido**: cada alta agrega una escritura privada y su hash en el read-write set, y cada organización almacena sus propios marcadores. Con un peer por organización (punto 2) la diseminación es local y el sobrecosto es acotado, pero es real y debe medirse: NET-3 (#22) lo considera en el dimensionamiento y el protocolo de medición (DES-7) debe reportarlo en lugar de darlo por despreciable.

   h. **Estados terminales** (`DISPENSADO`, `DISPUESTO_FINAL`, `PROHIBIDO`): la política queda en el valor de reposo del último custodio registrado. No se endurece porque ADR-001 no admite transiciones de salida y es la lógica del chaincode la que rechaza el intento con `INVALID_STATE_TRANSITION`.

   i. **Qué no imponen ni SBE ni las colecciones, y se declara.** Las reglas de DES-6 que dependen de **quién invoca** —que quien dispensa sea el custodio con `agentType` habilitado, que quien informa un evento extraordinario sea el custodio y no ANMAT, que el laboratorio que interviene sea el titular de la unidad— se validan en la lógica del chaincode, ejecutada de forma idéntica por todos los peers endosantes sobre una identidad de creador que la plataforma verifica criptográficamente. Los mecanismos de endoso fijan **qué organizaciones deben ejecutar y coincidir**; la lógica restringe **qué puede hacer cada invocador**. La evidencia de NET-6 debe distinguir ambos planos: rechazo por la plataforma (`ENDORSEMENT_POLICY_FAILURE`, transacción inválida en el bloque) para lo primero, error tipificado del contrato para lo segundo.

   Se eligen mecanismos de plataforma y no solo validación en lógica porque hacen que sea **Fabric** quien rechace en fase de validación toda transacción sin los endosos requeridos, en lugar de depender de que la lógica se haya ejecutado en los peers correctos. Ese rechazo verificable es la evidencia empírica que NET-6 (#25) debe producir.

Queda fuera de alcance de este ADR: el contenido de `collections_config.json` (ADR-006), la entrada registral de las organizaciones no custodiales (ADR-010), la modificación de los workflows de CI (NET-4), el ajuste de recursos del host de medición (NET-3) y el esquema exacto del marcador de participación (CC-1).

## Justificación

La decisión de ordenamiento es la única de las seis con riesgo argumental. El trabajo escrito sostiene que distintas organizaciones contribuyen nodos al servicio de ordenamiento, reduciendo la dependencia de un único administrador central, y usa la eliminación del punto único de falla como argumento de la hipótesis. Repartir los 3 orderers entre `AnmatMSP`, `LabMSP` y `DrogueriaMSP` hace que la **estructura** del prototipo se corresponda con esa afirmación, en lugar de simplificarla: el consenso queda repartido entre tres MSP de ordenamiento distintas y ninguna organización controla por sí sola el servicio.

**Qué demuestra y qué no demuestra la evidencia experimental.** La descentralización administrativa así entendida es una propiedad **estructural**, verificable en la configuración del canal y en la distribución de identidades de ordenamiento, no una propiedad que los escenarios de disponibilidad midan. Lo que los escenarios Raft-1 y Raft-2 del protocolo de medición demuestran empíricamente es la **tolerancia a fallas por caída de nodos de ordenamiento lógicamente asignados a organizaciones distintas**, en un entorno de host único con fallos simulados a nivel de contenedor: la red conserva quorum con un nodo caído y deja de ordenar con dos. No demuestran la caída de una organización completa —que implicaría infraestructura, operadores y dominios de falla independientes—, ni la independencia administrativa real, que además queda acotada por la CA única del punto 3. Esta distinción es la que fija la issue #55 (DOC-7 · Limitaciones) para todo el trabajo: entorno single-host, fallos simulados y no de infraestructura física distribuida, valores indicativos del comportamiento relativo y no extrapolables a producción. La tesis debe presentar la descentralización del ordenamiento como propiedad de diseño respaldada por la configuración, y los escenarios Raft como evidencia de tolerancia a fallas bajo esa configuración — no como prueba de resiliencia organizacional.

La alternativa de organización única habría debilitado incluso esa lectura estructural a cambio de un ahorro de configuración modesto; se prefiere pagar el costo de configuración.

LevelDB y la CA única responden al mismo criterio: no cargar el host único de medición con componentes que el contrato congelado y el objetivo de la medición no necesitan. En el caso de la CA, la simplificación toca la raíz de confianza y por eso se declara explícitamente como límite, con el modelo productivo (una CA por organización) documentado. En el caso de LevelDB, la simplificación es reversible por revisión de este ADR y no toca ninguna garantía.

El descarte de cryptogen para el material definitivo es coherente con DES-6: los certificados de cliente deben portar el atributo `snt.role`, que es parte del flujo de registro y enrolamiento de Fabric CA; cryptogen no representa ese proceso.

**Sobre el endoso: tres mecanismos, tres alcances.** El diseño llegó a su forma actual después de corregir tres errores sucesivos, y conviene dejar los tres registrados porque cada uno delimita lo que un mecanismo de Fabric puede y no puede hacer:

- **La política se evalúa contra el estado previo.** Prometer que se fijan coendosos «cuando la operación lo requiere» es imposible: la política que la transacción escribe no rige a esa misma transacción.
- **La política es de la clave, no de la función.** Agregar una rama `AnmatMSP` a la política de reposo para habilitar los eventos regulatorios habilitaba también las operaciones ordinarias: una dispensación endosada solo por el peer del regulador, una recepción sin el par emisor–receptor.
- **La firma de creador no es un endoso.** Resuelto lo anterior quitando esa rama, la participación de ANMAT quedó apoyada únicamente en su firma de cliente, que acredita identidad e iniciativa pero no prueba que ningún peer suyo haya ejecutado la lógica — la misma distinción que el ADR ya aplicaba correctamente al laboratorio.

El tercer error se cierra con la propiedad que faltaba explotar: **la política de endoso de una colección implícita pertenece a su organización dueña y rige desde el despliegue**, sin depender de que ninguna clave exista. Escribir un marcador en la colección implícita de una organización es, por lo tanto, la forma nativa de exigir su endoso — incluso en la primera escritura de una clave, donde SBE no puede llegar. ADR-006 había registrado esa misma propiedad como objeción, porque ahí el coendoso no se quería; acá se usa con el signo opuesto, donde sí se quiere.

Con eso, las reglas de DES-6 se materializan por cuatro caminos delimitados: SBE sobre la clave de la unidad cuando el conjunto de endosantes es derivable del estado (reposo y tránsito); marcador en colección implícita cuando hay que exigir a una organización que no es la titular de la clave (registro inicial, eventos regulatorios, intervención de laboratorio); SBE sobre una clave de gobernanza para proteger su contenido de modificaciones ajenas; y lógica de chaincode sobre la identidad de creador cuando la regla depende de quién invoca.

**Qué propiedad queda demostrable, con precisión.** Sería falso afirmar que ninguna organización puede modificar por sí sola el estado de una unidad: un custodio dispensa, pone en cuarentena o informa un evento sobre su propia unidad con su cliente y su propio peer, y eso es deliberado — DES-6 prohíbe expresamente convertir a la autoridad de aplicación en coendosante de toda escritura ordinaria. La propiedad verificable es más estrecha y sigue siendo fuerte:

> Ninguna organización puede, por sí sola, cambiar el custodio registrado de una unidad, intervenir sobre una unidad que no está bajo su custodia, ni sustituir a una contraparte que el diseño exige.

Las operaciones unilaterales que existen son exactamente las del custodio sobre lo que tiene bajo custodia, y quedan acotadas por la máquina de estados de ADR-001 y por la matriz de DES-3. Es esa formulación —y no la versión amplia— la que debe llevarse al trabajo escrito y la que NET-6 debe respaldar con evidencia.

## Consecuencias

- **NET-1 (#20)**: resuelto el interrogante cryptogen vs. Fabric CA — Fabric CA para el material definitivo; cryptogen solo para pruebas locales descartables.
- **NET-2 (#21)**: el `configtx.yaml` debe definir 3 organizaciones de ordenamiento (`AnmatMSP` + `LabMSP` + `DrogueriaMSP`, cada una con MSP de orderer propia), los consenters Raft correspondientes y las organizaciones de peers del dataset mínimo, con canal `snt-channel`.
- **NET-3 (#22)**: el dimensionamiento del host debe presupuestar 3 orderers, los peers del punto 2 y una CA; LevelDB evita contenedores CouchDB adicionales.
- **NET-4 (#23)**: los scripts de despliegue implementan la secuencia de bootstrap del punto 5; los workflows de CI migran de `mychannel` a `snt-channel` cuando la red propia reemplace a `test-network`.
- **NET-6 (#25)**: la demo de endoso debe evidenciar el rechazo por la plataforma de: (i) una recepción o un rechazo endosados por una sola de las dos partes del tránsito; (ii) una dispensación endosada por un peer que no sea el del custodio; (iii) un evento regulatorio sin el endoso del peer regulatorio, o sin el del custodio; (iv) una operación de laboratorio no custodio sin el endoso del laboratorio, del regulador o del custodio; (v) un `RegisterUnit` sin el endoso del peer del laboratorio invocante. Debe además demostrar la restauración de la política de reposo en las tres salidas de `EN_TRANSITO` (punto 6.c) y separar el rechazo por política (`ENDORSEMENT_POLICY_FAILURE`) del rechazo por lógica (error tipificado del contrato). La afirmación que la evidencia debe sostener es la **acotada** de la Justificación, no la versión amplia.
- **NET-5 (#24)**: además de generar `collections_config.json` para las colecciones explícitas por par, debe verificar el comportamiento de los marcadores en colecciones implícitas — que existen sin declararse, que su política exige a la organización dueña y que su contenido no es legible por terceros.
- **NET-3 (#22) y protocolo de medición (DES-7)**: el marcador de participación agrega una escritura privada por alta y por evento regulatorio; el dimensionamiento y el reporte de resultados deben considerarlo en lugar de darlo por despreciable (punto 6.g).
- **CC-3 (#16) y CC-5**: implementan y testean la gestión de la política de clave — fijación en `RegisterUnit`, armado en el despacho y **restauración en las tres salidas de `EN_TRANSITO`** — y la escritura de marcadores de participación donde el punto 6 los exige.
- **Para el contrato DES-5**: el punto 6 fija la superficie de `AuthorizeLabIntervention` (SBE regulatoria sobre su clave, vigencia evaluada por lógica) y obliga a documentar, por operación, qué organizaciones deben endosar.
- **Protocolo de medición (§8.1)**: los escenarios Raft-1/Raft-2 conservan validez; cada caída afecta al nodo de una organización de ordenamiento distinta, con el alcance interpretativo acotado en la Justificación y por la issue #55 (host único, fallos simulados).
- **Se gana**: correspondencia estructural con la afirmación de descentralización del ordenamiento del trabajo escrito (verificable en la configuración del canal, no en la medición); parámetros concretos que desbloquean NET-1/2/3/4/6; endoso multiparte exigido por la plataforma en los cuatro casos que DES-6 lo pide, incluido el registro inicial que ninguna versión anterior lograba cubrir; y la propiedad acotada que la Justificación enuncia: **ninguna organización puede, por sí sola, cambiar la custodia de una unidad, intervenir sobre una unidad ajena ni sustituir a una contraparte exigida**; menor huella de recursos en el host de medición (LevelDB, CA única).
- **Se pierde / costo**: la configuración de ordenamiento multi-organización es más compleja que el default de `test-network` (tres MSP de orderer, material criptográfico y consenters por organización); la CA única concentra la raíz de confianza y debe listarse como limitación del prototipo; LevelDB cierra la puerta a rich queries hasta una revisión de este ADR; toda escritura sobre una unidad ajena queda subordinada a la disponibilidad del peer del custodio actual —o de ambas partes durante el tránsito—, y los eventos regulatorios agregan la del peer regulatorio; y cada alta y cada evento regulatorio pagan una escritura privada adicional (punto 6.g).
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
- [Contrato público del chaincode](../api-contract.md) (v2.4.0): `QueryUnitsByGTIN` por clave compuesta parcial (sin rich queries), `RegisterOrganization` para el seed del registro, `Init` para el bootstrap regulatorio y `AuthorizeLabIntervention` para el punto 6.e.
- Hyperledger Fabric, The Ordering Service (Raft): https://hyperledger-fabric.readthedocs.io/en/release-2.5/orderer/ordering_service.html — cluster Raft, quorum y contribución de orderers por múltiples organizaciones.
- Hyperledger Fabric, Endorsement policies (state-based endorsement): https://hyperledger-fabric.readthedocs.io/en/release-2.5/endorsement-policies.html — políticas por clave fijadas desde chaincode, su precedencia sobre la política de chaincode y sobre la política de colección, y la regla que fundamenta el punto 6: la validación de una transacción usa la política que la clave tenía **antes** de esa transacción, de modo que una política escrita con `SetStateValidationParameter` solo rige las escrituras posteriores.
- Hyperledger Fabric, Transaction Flow: https://hyperledger-fabric.readthedocs.io/en/release-2.5/txflow.html — separación entre firma del creador de la transacción y endosos recogidos: la firma acredita al invocador, no la ejecución de un peer suyo. Fundamento del punto 6.i y de la corrección del punto 6.d.
- Hyperledger Fabric, Private data: https://hyperledger-fabric.readthedocs.io/en/release-2.5/private-data/private-data.html — colecciones implícitas por organización (`_implicit_org_<MSPID>`), su política de endoso propia vigente desde el despliegue y su uso documentado para registrar la aprobación o el consentimiento de una organización. Fundamento del marcador de participación del punto 6.
- Hyperledger Fabric CA, Users Guide: https://hyperledger-fabric-ca.readthedocs.io/en/latest/users-guide.html — registro, enrolamiento y emisión de atributos; base del descarte de cryptogen.
