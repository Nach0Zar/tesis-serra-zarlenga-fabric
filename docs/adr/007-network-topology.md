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

   **Regla de plataforma que condiciona todo el diseño.** Fabric valida una transacción contra la política de endoso que la clave tenía **antes** de esa transacción. Una política escrita con `SetStateValidationParameter` rige las escrituras **posteriores**, nunca la transacción que la establece. De ahí se siguen dos límites que este ADR asume explícitamente:
   - SBE solo puede expresar requisitos de endoso **derivables del estado ya confirmado**. No puede condicionar el conjunto de endosantes a la operación que la transacción intenta ejecutar, porque en el momento de la validación esa operación aún no forma parte del estado.
   - SBE no distingue **invocador** de **endosante**: la firma del creador de la transacción se verifica siempre, pero la política de la clave se evalúa sobre el conjunto de endosos, no sobre quién propuso la transacción.

   Toda regla de DES-6 cuyo conjunto de endosantes dependa de *qué* se intenta hacer debe, por lo tanto, o bien materializarse por otro mecanismo, o bien **convertirse en estado previo** mediante una transacción de armado. El diseño que sigue aplica ese criterio operación por operación.

   a. **Política de reposo de la clave de la unidad**: `OR(<org del custodio actual>, AnmatMSP)`. La fija `RegisterUnit` (T01) al crear la clave y la restablece toda transición que cambie el custodio registrado o que resuelva el tránsito. Delimita el conjunto máximo de organizaciones capaces de escribir la unidad: su custodio y la autoridad regulatoria. La rama `AnmatMSP` es **necesaria**: ADR-001 habilita a ANMAT a ejecutar eventos extraordinarios y regulatorios sobre unidades en custodia ajena (T09, T13–T20, T22/T23, T27, T29/T31/T32); sin ella, la plataforma marcaría inválidas esas transacciones por más que la lógica del chaincode las autorice. Esa rama **no** convierte a `AnmatMSP` en coendosante obligatorio de nada — es disyuntiva —, con lo que se preserva la propiedad de DES-6 que lo prohíbe.

   b. **Política de tránsito**: en el **despacho** (T02/T03) el chaincode fija sobre la clave de la unidad `OR(AND(<org emisora>, <org receptora declarada>), AnmatMSP)`. Es el caso en que SBE sí puede expresar un requisito multiparte, porque el despacho **deja declarado en el estado** quién es la contraparte: la política de la recepción se deriva del estado confirmado por la transacción anterior. La rama `AND` materializa la propiedad central de DES-6 —una transferencia ordinaria no cambia custodia con el endoso del origen solamente— y la rama `AnmatMSP` mantiene disponibles los eventos extraordinarios que ADR-001 habilita durante el tránsito.

   c. **Restauración en toda salida de `EN_TRANSITO`**, no solo en la recepción. Cada transición que resuelve el tránsito restablece la política de reposo del punto (a) sobre el custodio que corresponda:
      - recepción (T04): custodio registrado pasa a ser el receptor → `OR(<org receptora>, AnmatMSP)`;
      - rechazo (T05): la custodia permanece en el emisor (ADR-004) → `OR(<org emisora>, AnmatMSP)`;
      - eventos extraordinarios que cierran el tránsito (T09, T13–T16): la custodia registrada permanece en el emisor → `OR(<org emisora>, AnmatMSP)`.

      Omitir esta restauración en (T05) o en los eventos extraordinarios dejaría la unidad indefinidamente bajo una política que exige al receptor de un despacho ya resuelto: un bloqueo permanente de las operaciones ordinarias del custodio. Es un requisito de corrección del chaincode, no una optimización, y CC-3/CC-5 deben cubrirlo con tests.

   d. **Estados terminales** (`DISPENSADO`, `DISPUESTO_FINAL`, `PROHIBIDO`): la política queda en el valor de reposo del último custodio registrado. No se endurece porque la máquina de estados de ADR-001 no admite ninguna transición de salida y es la lógica del chaincode —no la política de clave— la que rechaza el intento con `INVALID_STATE_TRANSITION`.

   e. **Operaciones cuyo endoso no es derivable del estado: patrón de armado.** Cuando una regla de DES-6 exige un conjunto de endosantes que depende de la operación intentada, se convierte esa exigencia en estado previo. El diseño usa dos formas del mismo patrón:
      - **Forma 1 — armar la propia clave.** Una transacción previa, válida bajo la política vigente, deja en el estado el dato del que se deriva la política siguiente y la fija. El par despacho → recepción del punto (b) es exactamente esta forma, leída ahora como caso particular de una regla general.
      - **Forma 2 — clave acompañante protegida.** Cuando armar la clave de la unidad tendría efectos colaterales inaceptables, la exigencia se materializa sobre una **clave de autorización separada** cuyo SBE fija a la organización que debe participar. La operación que consume la autorización debe **escribir** (borrar) esa clave, y por lo tanto su transacción debe satisfacer la política de esa clave. El endoso queda exigido por la plataforma sin haber tenido que anticipar, en la política de la unidad, qué operación se intentaría.

      La regla de DES-6 «retiro, recupero o disposición final iniciado por laboratorio no custodio → laboratorio invocante y `AnmatMSP`» se materializa con la **forma 2**: ANMAT emite previamente una autorización de intervención (`AuthorizeLabIntervention`, contrato DES-5) sobre la unidad, que crea la clave `LabIntervention`+[`gtin`,`numeroSerie`] y le fija SBE `AnmatMSP`; la operación posterior invocada por el laboratorio lee esa clave, la consume borrándola y escribe la unidad. Resultado: la transacción lleva la firma de creador del laboratorio (que la lógica del chaincode valida como titular habilitado) y **debe** llevar el endoso de `AnmatMSP` para poder borrar la clave de autorización — el par exigido por DES-6, impuesto por la fase de validación de Fabric.

      Se elige la forma 2 y no la forma 1 para este caso porque armar la clave de la unidad con `AND(<org laboratorio>, AnmatMSP)` mientras la autorización esté pendiente **bloquearía las operaciones ordinarias del custodio sobre su propia unidad** durante toda la vigencia de la autorización. La forma 2 deja la política de reposo intacta y solo grava la transacción que efectivamente ejerce la autorización.

      La autorización lleva vencimiento (`expiraEn`, contrato DES-5) para que una autorización no ejercida no quede como permiso permanente. v1 **no** incorpora revocación anticipada: el riesgo se acota fijando ventanas cortas, y agregar una operación de revocación sería un cambio compatible del contrato si un requisito lo pidiera.

   f. **Eventos regulatorios de ANMAT que tocan datos privados.** La fila de DES-6 «evento regulatorio iniciado por ANMAT → `AnmatMSP` y, cuando la operación afecte custodia o datos privados de un establecimiento, la organización custodia involucrada» no se materializa sobre la clave pública: la rama `AnmatMSP` de la política de reposo permitiría a ANMAT escribirla en solitario. Se materializa sobre la **política de endoso de la colección privada** del par (`OR(<org A>, <org B>)`, ADR-006): una transacción que cierre el registro de operación —escritura del histórico y `DelPrivateData`— debe estar endosada por al menos una de las dos organizaciones del par, porque `AnmatMSP` no figura como endosante de la colección. Ver ADR-006, punto 4, donde queda registrada la consecuencia operativa.

   g. **Qué no queda cubierto por SBE, y se declara.** Las reglas de DES-6 que dependen del invocador y no del estado —que quien dispensa sea el custodio con `agentType` habilitado, que quien informa un evento extraordinario sea el custodio y no ANMAT, que el laboratorio que interviene sea el titular de la unidad— se validan en la **lógica del chaincode**, ejecutada de forma idéntica por todos los peers endosantes. SBE fija el conjunto máximo de escritores por clave; la lógica lo restringe por operación. La evidencia que NET-6 debe producir distingue ambos planos: rechazo por la plataforma (`ENDORSEMENT_POLICY_FAILURE`, transacción inválida en el bloque) para las reglas materializadas por política, y error tipificado del contrato para las materializadas por lógica.

   Se elige SBE y no solo validación en lógica de chaincode para las reglas del primer plano porque SBE hace que la **plataforma** rechace en fase de validación toda transacción que carezca de los endosos requeridos sobre la clave, en lugar de depender de que la lógica se haya ejecutado en los peers correctos. Ese rechazo verificable es la evidencia empírica que NET-6 (#25) debe producir.

Queda fuera de alcance de este ADR: el contenido de `collections_config.json` (ADR-006), la entrada registral de las organizaciones no custodiales (ADR-010), la modificación de los workflows de CI (NET-4) y el ajuste de recursos del host de medición (NET-3).

## Justificación

La decisión de ordenamiento es la única de las seis con riesgo argumental. El trabajo escrito sostiene que distintas organizaciones contribuyen nodos al servicio de ordenamiento, reduciendo la dependencia de un único administrador central, y usa la eliminación del punto único de falla como argumento de la hipótesis. Repartir los 3 orderers entre `AnmatMSP`, `LabMSP` y `DrogueriaMSP` hace que la **estructura** del prototipo se corresponda con esa afirmación, en lugar de simplificarla: el consenso queda repartido entre tres MSP de ordenamiento distintas y ninguna organización controla por sí sola el servicio.

**Qué demuestra y qué no demuestra la evidencia experimental.** La descentralización administrativa así entendida es una propiedad **estructural**, verificable en la configuración del canal y en la distribución de identidades de ordenamiento, no una propiedad que los escenarios de disponibilidad midan. Lo que los escenarios Raft-1 y Raft-2 del protocolo de medición demuestran empíricamente es la **tolerancia a fallas por caída de nodos de ordenamiento lógicamente asignados a organizaciones distintas**, en un entorno de host único con fallos simulados a nivel de contenedor: la red conserva quorum con un nodo caído y deja de ordenar con dos. No demuestran la caída de una organización completa —que implicaría infraestructura, operadores y dominios de falla independientes—, ni la independencia administrativa real, que además queda acotada por la CA única del punto 3. Esta distinción es la que fija la issue #55 (DOC-7 · Limitaciones) para todo el trabajo: entorno single-host, fallos simulados y no de infraestructura física distribuida, valores indicativos del comportamiento relativo y no extrapolables a producción. La tesis debe presentar la descentralización del ordenamiento como propiedad de diseño respaldada por la configuración, y los escenarios Raft como evidencia de tolerancia a fallas bajo esa configuración — no como prueba de resiliencia organizacional.

La alternativa de organización única habría debilitado incluso esa lectura estructural a cambio de un ahorro de configuración modesto; se prefiere pagar el costo de configuración.

LevelDB y la CA única responden al mismo criterio: no cargar el host único de medición con componentes que el contrato congelado y el objetivo de la medición no necesitan. En el caso de la CA, la simplificación toca la raíz de confianza y por eso se declara explícitamente como límite, con el modelo productivo (una CA por organización) documentado. En el caso de LevelDB, la simplificación es reversible por revisión de este ADR y no toca ninguna garantía.

El descarte de cryptogen para el material definitivo es coherente con DES-6: los certificados de cliente deben portar el atributo `snt.role`, que es parte del flujo de registro y enrolamiento de Fabric CA; cryptogen no representa ese proceso.

**Sobre el endoso: qué expresa SBE y qué no.** SBE es el único mecanismo nativo de Fabric que expresa políticas por participante y por unidad, que es la forma que tiene la tabla de DES-6. Pero expresa esas políticas **en función del estado confirmado**, no de la operación intentada, y una versión anterior de este ADR sobreestimó su alcance al prometer que los coendosos regulatorios «se fijan de la misma forma sobre la clave cuando la operación lo requiere»: eso es imposible, porque la política que la transacción escribe no rige a esa misma transacción. La corrección no es cosmética — cambia la materialización de tres reglas de DES-6:

- la que exige que una transferencia ordinaria no cambie custodia con el endoso del origen solamente se materializa con SBE, porque el despacho deja en el estado el dato que la política de la recepción necesita (punto 6.b);
- la que exige el coendoso de `AnmatMSP` cuando un laboratorio no custodio interviene sobre una unidad ajena se materializa con una **clave de autorización protegida** cuyo borrado arrastra el endoso regulatorio (punto 6.e), porque el requisito depende de quién invoca y no del estado previo;
- la que exige a la organización custodia involucrada cuando ANMAT toca datos privados se materializa con la **política de endoso de la colección** (punto 6.f), que sí es una propiedad del recurso escrito y no de la operación.

Las tres preservan las propiedades que DES-6 obliga a conservar: no convierten a `AnmatMSP` en coendosante universal, no permiten que una transferencia cambie custodia con endoso solo del origen, y no requieren una MSP por categoría. Y las tres producen evidencia verificable a nivel de plataforma: transacciones marcadas inválidas por la propia fase de validación de Fabric, no errores de aplicación. Lo que queda fuera de ese plano (punto 6.g) se declara como validado por lógica de chaincode, para que la tesis no atribuya a la plataforma garantías que no impone.

## Consecuencias

- **NET-1 (#20)**: resuelto el interrogante cryptogen vs. Fabric CA — Fabric CA para el material definitivo; cryptogen solo para pruebas locales descartables.
- **NET-2 (#21)**: el `configtx.yaml` debe definir 3 organizaciones de ordenamiento (`AnmatMSP` + `LabMSP` + `DrogueriaMSP`, cada una con MSP de orderer propia), los consenters Raft correspondientes y las organizaciones de peers del dataset mínimo, con canal `snt-channel`.
- **NET-3 (#22)**: el dimensionamiento del host debe presupuestar 3 orderers, los peers del punto 2 y una CA; LevelDB evita contenedores CouchDB adicionales.
- **NET-4 (#23)**: los scripts de despliegue implementan la secuencia de bootstrap del punto 5; los workflows de CI migran de `mychannel` a `snt-channel` cuando la red propia reemplace a `test-network`.
- **NET-6 (#25)**: la demo de endoso se implementa con política base `OR` + SBE conforme el punto 6, y debe evidenciar el rechazo por la plataforma en los **tres** planos que el punto 6 materializa: (i) recepción sin el endoso del emisor o del receptor durante el tránsito; (ii) operación de laboratorio no custodio sin el endoso de `AnmatMSP`, que no puede borrar la clave de autorización; (iii) cierre regulatorio de una transferencia endosado solo por `AnmatMSP`, que no satisface la política de la colección del par. Debe además demostrar la restauración de la política de reposo en las tres salidas de `EN_TRANSITO` del punto 6.c, y separar en su evidencia el rechazo por política (`ENDORSEMENT_POLICY_FAILURE`) del rechazo por lógica (error tipificado del contrato).
- **CC-3 (#16) y CC-5**: además de las transiciones, deben implementar y testear la gestión de la política de clave — fijación en `RegisterUnit`, armado en el despacho y **restauración en las tres salidas de `EN_TRANSITO`** (punto 6.c). Una salida sin restauración deja la unidad bloqueada de forma permanente.
- **Para el contrato DES-5**: el punto 6.e introduce una operación nueva, `AuthorizeLabIntervention`, sin la cual la fila de DES-6 sobre intervención de laboratorio no custodio no es materializable. Es un agregado compatible (MINOR) del contrato.
- **Protocolo de medición (§8.1)**: los escenarios Raft-1/Raft-2 conservan validez; cada caída afecta al nodo de una organización de ordenamiento distinta, con el alcance interpretativo acotado en la Justificación y por la issue #55 (host único, fallos simulados).
- **Se gana**: correspondencia estructural con la afirmación de descentralización del ordenamiento del trabajo escrito (verificable en la configuración del canal, no en la medición); parámetros concretos que desbloquean NET-1/2/3/4/6; endoso multiparte exigido por la plataforma (evidencia para NET-6); menor huella de recursos en el host de medición (LevelDB, CA única).
- **Se pierde / costo**: la configuración de ordenamiento multi-organización es más compleja que el default de `test-network` (tres MSP de orderer, material criptográfico y consenters por organización); la CA única concentra la raíz de confianza y debe listarse como limitación del prototipo; LevelDB cierra la puerta a rich queries hasta una revisión de este ADR.
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
