# ADR-006: Mecanismo de colecciones privadas para la información comercial y el registro de operación

- **Estado**: Propuesto
- **Fecha**: 2026-08-17
- **Autores**: Serra, Zarlenga

---

## Contexto

ADR-002 dividió el modelo de datos en estado público del canal (estado mínimo de trazabilidad) e información comercial y documental que se almacena en Private Data Collections con membresía "organizaciones participantes de la operación + `AnmatMSP`", y delegó explícitamente a NET-5 la granularidad de las colecciones, sus nombres y el mecanismo para generar la membresía de forma programática. ADR-004 hizo depender de esa capa privada una pieza central del flujo de transferencia: el **registro de operación** (destinatario declarado `DestinatarioPendiente` + factura/remito), con ciclo de vida activo/cerrado, que la recepción (T04) y el rechazo (T05) validan contra la operación activa, y que se conserva como historial auditable tras el cierre. ADR-004 aclaró que "PDC de la operación" es una regla lógica de visibilidad por operación, no una colección Fabric creada por transacción, y dejó la elección del mecanismo concreto para esta decisión.

La restricción técnica que obliga a decidir ahora es que las membresías de Fabric no son por operación: las colecciones explícitas se definen **por chaincode** en el momento del deploy (`collections_config.json` forma parte de la definición aprobada por el lifecycle), y las colecciones implícitas existen una por organización con políticas fijas. Con una organización por establecimiento (ADR-003), la cantidad de pares posibles crece con el cuadrado de las organizaciones, por lo que la elección del mecanismo determina tanto la viabilidad del flujo de ADR-004 como el costo de gobernanza del onboarding.

El contrato del chaincode (v2.0.2) ya fija el transporte: la información comercial viaja por `transient` bajo la clave `commercial` y el destinatario declarado bajo la clave `destinatario`; ninguno viaja como argumento público. Lo que el contrato no fija — y esta ADR sí — es dónde aterrizan esos datos y bajo qué claves. La issue #81 (DES-12) formaliza esta decisión, que bloquea la implementación de `DispatchTransfer`/`ReceiveTransfer` (CC-3, #16) y la configuración de red (NET-5, #24).

## Alternativas

**A. Colecciones implícitas por organización (`_implicit_org_<MSPID>`)**

- Fabric crea automáticamente una colección implícita por organización del canal, sin `collections_config.json` que mantener: el alta de una organización no requiere redefinir el chaincode. Era la candidata a validar según la issue #81, precisamente por ese costo de gobernanza nulo.
- El patrón sería: en el despacho, escribir el registro de operación en la colección implícita del emisor, la del receptor declarado y la de `AnmatMSP`, replicando el dato hacia cada parte.
- Las colecciones implícitas tienen propiedades **fijas** que no se pueden reconfigurar: `memberOnlyWrite: true` y política de endoso de colección fijada a la organización dueña. Eso rompe el flujo de ADR-004 en dos puntos:
  1. **En el despacho**, el cliente del emisor no puede escribir el registro de operación en la colección implícita del receptor ni en la de ANMAT: `memberOnlyWrite` bloquea toda escritura propuesta por un cliente de una organización distinta de la dueña. El patrón alternativo que Fabric documenta para estos casos — cada parte escribe su propia copia del dato en su propia colección implícita, como en el sample "secured asset transfer" — no aplica aquí, porque el receptor **no conoce la operación hasta el despacho**: no hay un intercambio previo fuera de banda que le permita escribir su copia, y exigirlo introduciría exactamente el canal lateral que el modelo de dos transacciones de ADR-004 evita.
  2. **En la recepción**, ADR-004 exige endoso conjunto emisor + receptor (T04), y ambos peers endosantes deben poder leer **el mismo** registro de operación para producir read-sets consistentes sobre los mismos hashes. Con colecciones implícitas cada peer solo puede leer la colección de su propia organización: no existe una colección que ambos endosantes puedan leer, por lo que el endoso conjunto sobre el contenido privado de la operación es irrealizable con este mecanismo.
- Se descarta porque sus políticas fijas (`memberOnlyWrite: true`, endoso por organización dueña) son incompatibles con el despacho escrito por el emisor y con la lectura conjunta que exige el endoso emisor + receptor de la recepción.

**B. Colecciones explícitas por par autorizado de organizaciones, generadas programáticamente**

- Para cada par de organizaciones entre las que la matriz de DES-3 autorice una transferencia en alguna dirección, se define una colección explícita con membresía {org A, org B, `AnmatMSP`} y nombre determinístico derivado de ambos `mspId` ordenados lexicográficamente.
- Resuelve exactamente el requisito que hunde a la alternativa A: emisor y receptor son miembros de la misma colección, ambos peers pueden leer el mismo registro de operación durante el endoso conjunto de la recepción, y `AnmatMSP` audita por membresía sin mecanismos adicionales.
- La escala es acotada y calculable: las colecciones crecen con los pares de organizaciones cuyo par de tipos esté autorizado por la matriz (16 pares de tipos autorizados sobre `defaultDecision: DENY`), no con todas las combinaciones de organizaciones. Para el dataset mínimo del prototipo (~6 organizaciones custodiales) el resultado es del orden de decenas de colecciones — trivial de generar y desplegar.
- El costo real es de **gobernanza, no de arquitectura**: `collections_config.json` no puede mantenerse a mano (ADR-002 ya lo prohibía como política), e incorporar una organización nueva requiere regenerar el archivo y actualizar la definición del chaincode con una nueva secuencia de lifecycle. Ese costo es consistente con el pipeline de onboarding gobernado que ADR-003 ya asume (alta en configuración de canal + alta en el registro de organización-establecimiento): se agrega un paso al mismo pipeline, no un proceso nuevo. Debe declararse, sin embargo, como limitación de escalabilidad para un despliegue productivo con cientos de establecimientos.
- Se adopta.

**C. Datos transitorios con solo hash persistido (sin colección)**

- El dato viaja por `transient` (como ya exige el contrato), el chaincode lo valida durante el endoso y solo persiste un hash (por ejemplo como estado público), sin escribir contenido en ninguna colección.
- Elimina por completo el problema de membresía: no hay colecciones que definir ni regenerar.
- Se descarta porque ADR-004 exige conservar el registro de operación como **historial auditable** accesible a ANMAT y a las partes, y además la recepción necesita leer el destinatario declarado desde la capa privada para validar `RECEIVER_MISMATCH`. Con transient-only el contenido se pierde al terminar el endoso y solo quedaría el hash: insuficiente para la verificación del destinatario en la recepción e insuficiente como registro histórico de la operación.

## Decisión

Se adopta la **alternativa B**: colecciones explícitas por par autorizado de organizaciones, generadas programáticamente.

1. **Definición de cada colección**: para cada par de organizaciones entre las que exista **alguna** relación de transferencia autorizada por la matriz `domain/authorized-transfers.json` — en cualquiera de las dos direcciones — se define **una** colección, con:
   - nombre determinístico `transfer_<mspIdA>_<mspIdB>`, donde `mspIdA` y `mspIdB` son los identificadores de ambas organizaciones **ordenados lexicográficamente**. El orden lexicográfico (y no el de origen → destino) hace que la colección sea la misma cualquiera sea el sentido del flujo: la transferencia ordinaria en la dirección autorizada y la devolución en sentido inverso (ADR-009) resuelven determinísticamente al mismo nombre desde cualquier peer endosante. Una colección por par de organizaciones, no por par ordenado;
   - membresía (`memberOrgsPolicy`): organización origen, organización destino y `AnmatMSP`;
   - `memberOnlyRead: true` (solo los miembros leen el contenido);
   - `requiredPeerCount: 1` (la escritura privada debe diseminarse al menos a un peer de otra organización miembro antes de completar el endoso);
   - `blockToLive: 0` (retención indefinida: el registro histórico auditable que exige ADR-004 no admite purga automática).
2. **Generación programática**: `collections_config.json` **no se escribe a mano**. Lo produce una herramienta de build versionada en el repositorio (issue NET-5, #24) que toma como entrada el registro de organización-establecimiento (dataset de organizaciones) y la matriz `domain/authorized-transfers.json`, y emite el archivo de colecciones. Esta ADR fija el **contrato** de esa herramienta — entradas, convención de nombres, propiedades de cada colección — no su implementación.
3. **Qué colección usa cada operación**: el despacho (`DispatchTransfer`) y la recepción/rechazo (`ReceiveTransfer`/`RejectTransfer`) usan la colección del par formado por la organización emisora y la del destino declarado. La devolución (`ReturnProduct`), cuando declara un receptor de devolución como dato privado conforme ADR-009, usa **la misma colección** del par formado por el custodio declarante y el receptor declarado: por la convención de nombre ordenado del punto 1, el sentido inverso del flujo no requiere una colección adicional, y la relación existe por definición porque la devolución solo procede entre organizaciones con relación de transferencia autorizada en la dirección original. Los eventos extraordinarios (T09, T13–T16) no escriben datos privados nuevos: su campo `motivo` es un argumento público del canal (contrato v2.0.2).
4. **Claves dentro de la colección**, conforme el ciclo de vida activo/cerrado de ADR-004:
   - Registro de la operación **activa**: composite key `TransferOpActive` + [`gtin`, `numeroSerie`]. Por construcción existe a lo sumo un registro activo por unidad (ADR-004, regla 2).
   - Registro **histórico**: composite key `TransferOp` + [`gtin`, `numeroSerie`, `txIdDespacho`], donde `txIdDespacho` es el identificador de la transacción de despacho que creó la operación (clave nueva por operación, ADR-004, regla 5).
   - Al cerrar la operación por cualquier vía (T04, T05, T09, T13–T16), el chaincode escribe el registro histórico y elimina la clave activa con `DelPrivateData`. La eliminación borra el contenido de la base de datos privada de los peers miembros, pero el hash de la escritura original permanece en el ledger del canal como evidencia inmutable de que la operación existió.
5. **Contenido del registro de operación**: el destinatario declarado (identificador canónico `GLN:`/`CUFE:`), los datos documentales del `transient` `commercial` (remito, factura, cantidad) y — cuando DES-14/ADR-008 lo confirme — el `ruleId` y la versión de la matriz que autorizó el par de la transferencia.
6. **Evidencia pública**: la evidencia en el ledger común se limita a los hashes que Fabric persiste automáticamente por cada escritura privada (hash de clave y valor en el read-write set de la transacción). No se replica contenido comercial al canal. NET-5 debe demostrar que una organización no miembro de la colección ve el hash y no el contenido.

Queda fuera de alcance de esta ADR: la implementación de la herramienta generadora (NET-5, #24), la integración del `collections_config.json` en el script de despliegue (NET-4, #23) y el uso de las APIs de datos privados dentro del chaincode (CC-3, #16).

## Justificación

La decisión se ancla en el requisito más rígido del diseño ya aceptado: el endoso conjunto emisor + receptor de la recepción (ADR-004, "Endoso") exige que ambos peers endosantes ejecuten la misma validación sobre el mismo registro de operación y produzcan read-sets consistentes. Solo una colección de la que ambas organizaciones son miembros satisface esa condición; las colecciones implícitas no pueden, por diseño de plataforma, y los datos transitorios no dejan nada que leer. La alternativa B no es la más barata en gobernanza, pero es la única de las tres que implementa el flujo de ADR-004 sin debilitarlo.

El costo de gobernanza que introduce — regenerar colecciones y redefinir el chaincode en cada onboarding — no es un costo nuevo en especie: ADR-003 ya estableció que el alta de un establecimiento es una operación gobernada de múltiples pasos (configuración de canal + registro) y que las políticas que referencian organizaciones individuales deben generarse por herramientas. Esta decisión agrega un paso más al mismo pipeline y lo hace con la misma herramienta de generación programática que ADR-002 ya exigía evaluar. Para el prototipo, con un dataset acotado de organizaciones, el costo es marginal; para producción queda declarado como limitación explícita (ver "Queda pendiente").

La convención de nombres determinística (`transfer_<mspIdA>_<mspIdB>` con los identificadores ordenados lexicográficamente) hace que el chaincode pueda resolver la colección de una operación sin tablas de mapeo adicionales: con el `mspId` del invocador y el `mspId` de la contraparte declarada (resuelto desde el registro de ADR-003), el nombre de la colección se computa determinísticamente en el endoso — condición necesaria para que todos los peers endosantes escriban en la misma colección. El orden lexicográfico, en lugar del orden origen → destino, evita además duplicar colecciones para el flujo inverso de una devolución (ADR-009) y garantiza que emisor y receptor computen el mismo nombre sin depender de quién invoca.

## Consecuencias

- **Para NET-5 (issue #24)**: materializa esta decisión: implementa la herramienta generadora con el contrato fijado aquí, produce el `collections_config.json` del dataset del prototipo y ejecuta la demo de confidencialidad (organización no miembro ve hash, no contenido; ANMAT lee por membresía).
- **Para CC-3 (issue #16)**: la implementación de `DispatchTransfer`/`ReceiveTransfer`/`RejectTransfer` usa las claves (`TransferOpActive`, `TransferOp`) y la resolución determinística de nombre de colección definidas aquí, incluyendo el cierre con `DelPrivateData`.
- **Para NET-4 (issue #23)**: el script de despliegue del chaincode (`deployCC`) debe incluir el `collections_config.json` generado en la definición aprobada por el lifecycle.
- **Para ADR-003**: el pipeline de onboarding gobernado incorpora un paso adicional: alta en configuración de canal + alta en el registro + **regeneración de `collections_config.json` y nueva secuencia de lifecycle del chaincode**.
- **Se gana**: el flujo completo de ADR-004 es implementable sin canales laterales — lectura conjunta del registro de operación en el endoso de la recepción, auditoría de ANMAT por membresía, historial auditable con retención indefinida y evidencia hash verificable en el ledger común.
- **Se pierde / costo**: cada incorporación de organización exige regenerar colecciones y actualizar la definición del chaincode (nueva secuencia de lifecycle); el número de colecciones, aunque acotado por la matriz, crece con los pares de organizaciones y es una limitación de escalabilidad declarada para producción.
- **Queda pendiente**: evaluar en producción mecanismos que eviten el upgrade de chaincode por onboarding — por ejemplo, revisar esta decisión si una versión futura de Fabric incorpora colecciones dinámicas o membresías actualizables sin redefinición del chaincode.

## Divergencia con el trabajo escrito

No hay divergencia. El trabajo escrito afirma que el ledger común conserva el hash de la información privada como evidencia verificable, y esta decisión lo cumple: cada escritura del registro de operación en la colección del par deja su hash en el read-write set de la transacción del canal, visible para toda organización miembro, incluso después de que el contenido activo se elimine con `DelPrivateData` al cerrar la operación. Se deja constancia explícita de que la afirmación del trabajo escrito queda satisfecha sin necesidad de actualización.

## Contexto utilizado

- Issue GitHub #81: DES-12 · ADR-006: Diseño de colecciones privadas (resuelve NET-5), consultada el 2026-08-17.
- [ADR-002: Topología de canales](002-topologia-canales.md): clasificación estado público / información comercial, política de diseño de PDC para NET-5 y prohibición de mantener membresías a mano.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): una organización por establecimiento, registro organización-establecimiento y pipeline de onboarding gobernado que esta decisión extiende.
- [ADR-004: Modelado de la transferencia — despacho/recepción como dos transacciones](004-transfer-dispatch-reception.md): registro de operación, ciclo de vida activo/cerrado, endoso conjunto de la recepción y membresía lógica emisor + receptor declarado + `AnmatMSP` que esta decisión materializa.
- [docs/api-contract.md](../api-contract.md) (v2.0.2), sección "Datos privados": transporte por `transient` (`commercial`, `destinatario`) y carácter público del campo `motivo` de los eventos extraordinarios.
- [domain/authorized-transfers.json](../../domain/authorized-transfers.json): matriz de pares de `agentType` autorizados (entrada de la herramienta generadora).
- Documentación oficial de Hyperledger Fabric: [Private data](https://hyperledger-fabric.readthedocs.io/en/release-2.5/private-data/private-data.html) y [Private data architecture](https://hyperledger-fabric.readthedocs.io/en/release-2.5/private-data-arch.html): colecciones explícitas e implícitas, propiedades `memberOnlyRead`/`memberOnlyWrite`, `requiredPeerCount`, `blockToLive`, persistencia de hashes en el ledger del canal y `DelPrivateData`.
