# EVAL-8: análisis cualitativo y estructural con resultados finales

Fecha de corte: 22/10/2026. Issue [EVAL-8 / #48](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/48).

## 1. Resultado y alcance de la interpretación

La campaña final de rendimiento muestra que ambos prototipos sostuvieron aproximadamente las cargas ofrecidas ensayadas, con latencias menores en la baseline. Fabric aporta controles verificables entre organizaciones, un registro compartido de transacciones y aislamiento del contenido comercial mediante colecciones privadas. Esas propiedades tienen costos observados de latencia y reintentos, y costos funcionales de coordinación, disponibilidad y granularidad. La evidencia permite argumentar una elección condicionada por el modelo de confianza; no demuestra superioridad absoluta ni capacidad máxima.

Este análisis usa los **20 escenarios finales** de EVAL-6/7. Sustituye como base de interpretación el [corte parcial de 13 escenarios](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/48#issuecomment-6333125883), que se conserva como antecedente. El [primer aporte final](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/48#issuecomment-6339628252) incorporó las mediciones faltantes; aquí se desarrolla la dimensión estructural completa. No se ejecutan nuevos benchmarks ni se alteran ADRs, reglas del dominio, políticas de endoso o PDC.

Se distinguen tres clases de afirmación: **observación** de las campañas; **propiedad del diseño e implementación** apoyada por NET/CC; e **interpretación** de cuándo su costo se justificaría. Una observación de rendimiento no demuestra por sí sola integridad o confidencialidad; una prueba de endoso tampoco mide rendimiento o disponibilidad productivos.

## 2. Fuentes, identidad y trazabilidad

| Fuente | Identidad y uso | Límite |
|---|---|---|
| EVAL-6/7, rendimiento final | Campaña `eval67-389ca16-20261021e-full`; commit medido `389ca160b2a490cd66bb0194f012e2b75b3598e8`, etiqueta `eval67-20261021T0020Z`; contrato 2.11.2; [entrega y reproducción][R67]. | Un host y tasas fijas; sin presupuesto agregado equivalente demostrado. |
| Evidencia versionada de EVAL-6/7 | Commit `18d0d950817e38451953e056b418d49751b384ce`; [CSV finales][CSV67], crudos, metadata, campaña, índices, manifiestos, auditorías y SVG. Publicación mediante [#156](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/pull/156) y [#157](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/pull/157). | El commit de evidencia y los HEAD de integración son posteriores al artefacto medido. |
| EVAL-4/5, disponibilidad | [Release y complemento del 22/10][AV]; JSON y reporte del [análisis complementado][AVFILE]. PRs #153/#154 integradas. | Otra campaña/host respecto de rendimiento. `raft-1` sigue incomplete; se conserva una restauración fallida con evidencia reconstruida. #44/#45 siguen abiertas al elaborar este análisis. |
| Integridad, auditabilidad y privacidad | [NET-5](../network/evidence/NET-5.md), [NET-6](../network/evidence/NET-6.md), [NET-9](../network/evidence/NET-9.md); CC-5/9/10, ADR-006/007/012 y código enlazado en las secciones siguientes. | Pruebas estructurales previas con sus propios paquetes y fechas, no nuevas pruebas adversariales del commit de rendimiento. |

Dataset compartido de rendimiento: 50.000 identidades, seed `20260727`, SHA-256 `de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d`. El estado golden contiene 46.920 unidades presentes y 3.080 ausentes; 22.103 preparadas y 24.817 de relleno. El mismo plan lógico acreditado rige ambos SUT; cada intento restaura y verifica, y los SUT alternan sin solaparse. Ryzen 7 7800X3D, WSL2 Ubuntu 24.04, 6 CPU visibles, memoria efectiva 8.327.917.568 bytes y swap 4 GiB; recursos individuales y versiones en [R67].

El procesamiento estricto incluyó 224 mediciones (112 por SUT), sin pares faltantes, y excluyó sólo 264 smoke y 40 warm-ups. La campaña terminó con 264 intentos completed y cero failed. Son 16 escenarios con cinco pares y cuatro con ocho: registro 10/20, unidad 50 e historial 25. Se conservan originales 1–5 y extensiones 1–8. El procesamiento con el código integrado volvió a reproducir sus ocho salidas normalizando únicamente prefijos de rutas; SHA-256 de comparison.csv: `d233ef6d3d1c820b3ca082a2e50bba274c06fbc1b11641a158d75c59dd4a5dd0`.

La reproducción estricta depende de los bundles originales de snapshots fuera de Git; un manifiesto solo no los sustituye. No se publican claves, dumps, volúmenes ni planes privados. El análisis complementado de disponibilidad se cotejó con su SHA-256 publicado: `87c1b99c054bc6997198866ff2164835005fdd9ff21a7185ffedd62c854b5be7`. No se presenta ese cotejo como una nueva ejecución de disponibilidad.

## 3. Costo cuantitativo final, separado por perfil

### 3.1 Caminos exitosos

Media entre repeticiones; p95 nearest-rank calculado por ronda y luego promediado. La tabla usa 1–8 cuando hubo extensión y 1–5 en los demás casos. Redondeo a tres decimales; desvíos, CV, p50/p99 y ambas series completas permanecen en [CSV67] y [el reporte final][REPORT67]. No se calcula un promedio entre workloads.

| Perfil | Tasa ofrecida | n por SUT | Throughput Fabric | Throughput baseline | p95 Fabric (ms) | p95 baseline (ms) |
|---|---:|---:|---:|---:|---:|---:|
| Registro | 5 | 5 | 4,944 | 5,000 | 2108,200 | 8,000 |
| Registro | 10 | 8 | 9,968 | 10,000 | 2098,125 | 7,125 |
| Registro | 20 | 8 | 19,809 | 20,000 | 2254,250 | 6,500 |
| Transferencia | 5 | 5 | 4,881 | 5,000 | 4378,200 | 15,400 |
| Transferencia | 10 | 5 | 9,719 | 10,000 | 4555,400 | 14,000 |
| Transferencia | 20 | 5 | 19,456 | 20,000 | 4767,600 | 12,400 |
| Dispensación | 5 | 5 | 4,991 | 5,000 | 2079,400 | 6,400 |
| Dispensación | 10 | 5 | 9,941 | 10,000 | 2100,600 | 6,600 |
| Dispensación | 20 | 5 | 19,846 | 20,000 | 2141,000 | 5,800 |
| Unidad | 10 | 5 | 10,032 | 10,000 | 8,600 | 3,000 |
| Unidad | 25 | 5 | 25,031 | 25,000 | 5,200 | 2,000 |
| Unidad | 50 | 8 | 50,029 | 50,000 | 5,500 | 2,000 |
| Historial | 10 | 5 | 10,032 | 10,000 | 9,200 | 3,000 |
| Historial | 25 | 8 | 25,031 | 25,000 | 7,250 | 2,250 |
| Historial | 50 | 5 | 50,029 | 50,000 | 6,800 | 2,000 |
| Mixto | 20 | 5 | 19,460 | 20,000 | 4336,000 | 11,200 |

Throughput en operaciones conceptuales/s; transferencia en **pares completos/s**, cada uno con dos transacciones confirmadas. En mixto, las transferencias siguen contando como un par dentro del ciclo exacto 2 registros / 11 transferencias / 2 dispensaciones / 5 consultas. Restauración, verificación y calentamiento no integran las latencias del workload.

### 3.2 Rechazos y variabilidad

| Variante de rechazo | n por SUT | Casos/s Fabric | Casos/s baseline | p95 Fabric (ms) | p95 baseline (ms) |
|---|---:|---:|---:|---:|---:|
| BLOCKING_STATE/dispense | 5 | 5,033 | 5,000 | 7,000 | 3,400 |
| BLOCKING_STATE/transfer | 5 | 5,033 | 5,000 | 8,600 | 3,800 |
| DUPLICATE_IDENTITY/register | 5 | 5,033 | 5,000 | 7,000 | 4,000 |
| UNAUTHORIZED_TRANSFER/transfer | 5 | 5,033 | 5,000 | 9,600 | 3,800 |

Se acreditaron 6.040 rechazos esperados Fabric y 6.000 baseline con `TRANSFER_NOT_AUTHORIZED`, `UNIT_ALREADY_EXISTS` e `INVALID_STATE_TRANSITION`. Las diferencias de borde del generador no acreditan capacidad superior. Estas rondas prueban decisiones contractuales concretas; no se suman al throughput exitoso, no son fallos inesperados ni prueban por sí solas las políticas de endoso.

La extensión predefinida a ocho no garantizó estabilidad. Persisten estos CV poblacionales de p95:

| SUT / perfil | CV p95 en 1–8 |
|---|---:|
| baseline / Historial 25 | 19,25 % |
| fabric / Historial 25 | 16,54 % |
| fabric / Unidad 50 | 18,18 % |
| baseline / Registro 10 | 16,36 % |
| baseline / Registro 20 | 15,38 % |

Se conservan las rondas; no se eligen las mejores cinco ni se prolonga el experimento hasta obtener un CV favorable. Las tasas son **ofrecidas y fijas**: sostenerlas no identifica saturación ni throughput máximo. Compartir host, dataset y perfiles no acredita igualdad del presupuesto agregado: se mantuvieron las cuotas propias de cada SUT. Las diferencias observadas no aíslan causalmente CPU, batching, red, PDC o marcadores.

## 4. Integridad: controles verificables y confianza requerida

[NET-6](../network/evidence/NET-6.md), [NET-9](../network/evidence/NET-9.md) y [ADR-007](adr/007-network-topology.md) separan autorización del cliente de endoso de peers. En reposo, la SBE de la unidad exige al custodio; durante el tránsito exige `AND(emisor,receptor)`. La recepción restituye la política al receptor y el rechazo al emisor. La política pública es la que exige ambas partes al cerrar el tránsito; el `OR(A,B)` de la colección privada no la sustituye. El alta exige al laboratorio mediante su colección implícita porque la SBE de una clave nueva aún no puede proteger esa primera escritura.

La evidencia Core correlaciona txID, bloque, posición exacta y `TRANSACTIONS_FILTER`: los endosos insuficientes aparecen como `ENDORSEMENT_POLICY_FAILURE` y no actualizan el estado. La referencia persistente es el [run 33815572025](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/actions/runs/33815572025), con revisión y paquete identificados en NET-6. No se interpreta un error devuelto al cliente durante simulación como esa misma evidencia de plataforma.

NET-9 extiende ese control a salidas extraordinarias de tránsito, cierre del registro privado, restauración de SBE y participación del regulador o del laboratorio no custodio cuando corresponde. El reporte local declara que su working tree no era una referencia inmutable; para el respaldo de integración se utiliza además la [PR #125](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/pull/125), head `4ea1fbc9a0bf128211f11206548420a0aa992bd0`, cuyo [job de red pasó](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/actions/runs/35918879396/job/107377398963). Las ejecuciones NET tienen identidades y paquetes propios; no se reetiquetan como pruebas realizadas sobre el paquete medido de EVAL-6.

La conclusión es acotada: una organización no puede sustituir por sí sola las contrapartes exigidas ni cambiar unilateralmente la custodia. **No toda operación requiere varias organizaciones**: el custodio puede dispensar su propia unidad. Entre despacho y recepción existe además una ventana en la que sólo el emisor validó el par; la recepción reevalúa la matriz y exige ambas partes. La persistencia compartida y sus verificaciones dificultan alterar la historia unilateralmente dentro del modelo de confianza; no vuelven verdadero un dato físico falso ingresado por un actor autorizado ni eliminan compromiso de claves, colusión o fallas de gobierno.

En la baseline, la API y la base bajo un operador aplican las reglas comunes y transacciones SQL; no ofrecen endosos entre administraciones independientes. Esa concentración simplifica la ejecución, pero la confianza en la administración central es una premisa, no una garantía equivalente al coendoso. Esta comparación se refiere a la baseline implementada; no prueba que todo diseño centralizado carezca de auditoría externa o alta disponibilidad.

### 4.1 Marcadores y transferencia: costos observados

Se acreditaron **33.042 marcadores**, 31.842 en registro y 1.200 en mixto, esperados y observados sin discrepancias. Corresponden a las altas y a la colección implícita del **laboratorio invocante**, como muestra [RegisterUnit](../chaincode/internal/snt/units.go). La campaña no ofreció eventos regulatorios y no acredita marcadores de esos eventos. Esto precisa la interpretación del parcial que asociaba las altas con participación implícita de ANMAT. Los marcadores exigidos en otros flujos regulatorios pertenecen a sus respectivas pruebas NET.

El conteo atribuye volumen de trabajo adicional; no hay A/B sin marcadores que aísle su costo marginal de latencia. Tampoco se puede separar ese costo restándolo de los resultados de una política diferente.

La transferencia añade dos confirmaciones por operación conceptual. Fabric registró **1.863 reintentos transitorios**, baseline cero; permanecen dentro de la latencia end-to-end y de los intentos de transacción. En mixto, p95 del par fue **4.436 ms frente a 12,2 ms**, mientras p95 global de mezcla fue **4.336 ms frente a 11,2 ms**. Son métricas distintas. Las barreras de visibilidad incluyen estado público todavía no observado por el receptor y, según el protocolo, disponibilidad del contenido privado. Los agregados no autorizan atribuir todos los reintentos exclusivamente a PDC, gossip o CPU. Un par que finalmente confirma no es un fallo funcional final, pero sus intentos fallidos y esperas sí son costo operativo real. Véase [DES-7 §3.4](measurement-protocol.md).

## 5. Auditabilidad: bloque, historial y telemetría son capas distintas

| Capa observable | Qué conserva o expone | Qué no debe inferirse |
|---|---|---|
| Ledger de bloques | Transacciones ordenadas con su código de validación, válidas o inválidas. Las inválidas quedan en bloque pero no actualizan el world state. | No contiene toda solicitud presentada: una propuesta rechazada y no ordenada no llega al bloque. |
| `GetUnitHistory` (CC-5) | Modificaciones válidas y confirmadas de una clave, con txID, timestamp, isDelete y snapshot de valor en orden cronológico normalizado. | No lista transacciones inválidas ni propuestas rechazadas; no es el log de todos los intentos. |
| `QueryUnitsByState` (CC-9) | Enumeración del estado actual mediante índice compuesto actualizado con cada transición. | No es un historial ni demuestra riqueza de consultas arbitrarias; el mantenimiento del índice agrega trabajo de escritura. |
| `GetLabInterventionHistory` (CC-10) | Emisiones, reemplazos, consumos y revocaciones confirmados de la clave de autorización. | El vencimiento es derivado del timestamp y no produce una escritura ni una entrada histórica sintética. |
| Cliente/Gateway y listener NET-8 | El cliente/Gateway registra rechazos de propuesta; el listener separa eventos de negocio válidos y avisos de transacciones inválidas en sus streams. | No hay un único stream on-ledger que garantice observar todos los intentos. |

La distinción está implementada en [queries.go](../chaincode/internal/snt/queries.go), documentada en [NET-6](../network/evidence/NET-6.md) y [el listener](anmat-event-listener.md). Los rechazos esperados de §3.2 validan reglas del dominio; no se convierten automáticamente en transacciones inválidas persistidas en bloques.

### 5.1 Historial nativo y costo de la baseline

[ADR-012 revisión 3](adr/012-baseline-design.md) exige `unit_events` y `lab_intervention_events` para reproducir las consultas de historial de las claves Fabric. La API inserta el evento junto a la modificación de estado; no lo actualiza ni borra por sus operaciones normales. Esa política append-only es una convención de aplicación bajo un administrador con facultades sobre PostgreSQL. Un administrador puede alterar filas; el prototipo no añade un testigo externo independiente para detectar o impedir toda modificación administrativa.

En Fabric, `GetHistoryForKey` aprovecha la historia que mantiene el ledger. **Nativo no significa gratis:** bloques, almacenamiento, réplica, índices y consultas tienen costo. La diferencia es que el chaincode no implementa otra tabla por cada clave para reconstruir esa historia. En particular, las escrituras de autorización pagan un INSERT adicional en `lab_intervention_events` del lado relacional; esas operaciones **no están** en los perfiles de EVAL-6/7. No se usa ese INSERT para explicar la diferencia medida de registro, transferencia o dispensación.

El historial por unidad sí se midió a 10/25/50 consultas/s (§3). El historial de autorizaciones, la enumeración por estado, el listener y las operaciones extraordinarias no tienen nuevos perfiles de rendimiento en esta campaña. Su análisis es estructural y funcional, con sus propias fuentes; no se extrapolan latencias desde otros workloads.

## 6. Confidencialidad: aislamiento del contenido y costo funcional

[NET-5](../network/evidence/NET-5.md) documenta diez colecciones explícitas por par autorizado: miembros emisor, receptor y ANMAT; lectura/escritura de miembros y política privada `OR(A,B)`, sin ANMAT como sustituto del endoso de una parte. El estado mínimo de trazabilidad sigue público dentro del canal. El [extracto sanitizado de bloque](../network/evidence/NET-5-block-40.json) muestra el nombre de colección y hashes sin el contenido privado; el probe y su paquete son una prueba de mecanismo de la red, no una medición del workload core de EVAL-6.

La colección oculta a terceros el remito, la factura y el contenido comercial; **no oculta toda la relación**. El nombre `transfer_<MSP A>_<MSP B>` circula en claro en el hashed RW set, por lo que un observador del canal puede inferir qué par de organizaciones operó. Los hashes y la actividad tampoco prueban anonimato. En la baseline las filas comerciales conviven en una base bajo una administración: aplicar permisos en la API no crea aislamiento entre administradores independientes de esa misma base. Se comparan estas implementaciones, sin afirmar que una base relacional no pueda incorporar otros mecanismos de aislamiento.

`requiredPeerCount: 1` garantiza diseminación adicional a un miembro elegible, no específicamente al receptor. La lectura privada que necesita la recepción puede depender de pull/reconciliación. Subir el umbral haría al despacho depender de más peers disponibles; cambiarlo después de medir alteraría el experimento. El [ADR-006](adr/006-private-data-collections.md) y DES-7 explican este compromiso; los reintentos medidos de §4.1 se conservan sin atribuirles una causa única.

### 6.1 DES-19: seis colecciones sin el laboratorio

[DES-19 / #116](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/116) identifica una incompatibilidad concreta: un laboratorio titular ajeno al par no pertenece a seis de las diez colecciones. No puede simular el cierre del registro privado exigido para salir de `EN_TRANSITO`, aunque exista una autorización de intervención. Ampliar la membresía le expondría documentación y contrapartes comerciales de transferencias ajenas.

La decisión incorporada en [ADR-001](adr/001-maquina-estados-medicamento.md), [ADR-006](adr/006-private-data-collections.md), el [contrato](api-contract.md) y [el alcance](alcance-prototipo.md) acota ese retiro durante el tránsito; no amplía la membresía. ANMAT es miembro de los pares, pero también debe reunir los endosos aplicables. El laboratorio conserva las vías admitidas cuando es parte o cuando la unidad ya salió del tránsito. No se trata de menor throughput: es una **restricción funcional concreta** que se aceptó para preservar la confidencialidad. No se deduce de ella cumplimiento normativo integral del prototipo ni se resuelve otra política desde este análisis.

## 7. Disponibilidad y puntos únicos de falla

La tabla usa el JSON del complemento publicado de EVAL-4/5 [AVFILE], cuyo hash se verificó (§2). Son medias entre rondas. En peers se separan clase **objetivo** (operaciones que requieren el peer caído) y **control**. La recuperación DES-7 es desde la inyección hasta el inicio del primer tramo estable de 30 s que recupera al menos 95% de la tasa previa; no significa reparación completa ni reingreso del nodo.

| Escenario | n | Éxito en falla | Recuperación del servicio desde inyección |
|---|---:|---|---|
| baseline / api-1 | 5 | 0,0 % | 60,8 s |
| baseline / db-1 | 5 | 0,0 % | 62,6 s |
| fabric / raft-1 | 8 (una restauración fallida) | 97,1 % | 0,2 s |
| fabric / raft-2 | 5 | 24,5 % | No alcanzada en la ronda |
| fabric / peer-1a | 5 | 0,0 % / 100,0 % (objetivo/control) | 75,1 s / 0,2 s |
| fabric / peer-1b | 8 | 0,4 % / 100,0 % (objetivo/control) | 78,9 s / 0,1 s |
| fabric / peer-1c | 5 | 0,0 % / 97,3 % (objetivo/control) | 80,1 s / 0,2 s |
| fabric / peer-1d | 5 | 2,2 % / 100,0 % (objetivo/control) | 66,4 s / 0,8 s |

API o BD detenidas cortaron globalmente el servicio de la baseline ensayada. Esos componentes únicos constituyen puntos de falla en **esa topología**, no una propiedad inevitable de cualquier arquitectura centralizada. No se implementó ni midió una alternativa relacional con réplica, failover o múltiples instancias de API; no se afirma haberla descartado.

Un orderer caído permitió continuidad con quórum 2/3; perder dos dejó sólo lecturas (24,5% global) y bloqueó escrituras. La continuidad de lecturas no equivale a disponibilidad de todas las operaciones. Raft no elimina la dependencia del quórum ni prueba aquí tolerancia a comportamiento malicioso.

**`raft-1` sigue incomplete.** Su repetición 2 conserva estado failed: el líder no reparó el WAL (`failed to read WAL and cannot repair`). La evidencia de ese intento fue reconstruida e identificada; los otros dos orderers sostuvieron servicio durante la falla. El balance de ocho incluye esa restauración fallida y no es una serie de ocho completed. La continuidad promedio de 97,1% y recuperación de servicio de 0,2 s no describen reincorporación exitosa del líder ni cero fallas. El CV p95 final de raft-1 permaneció en 23,4% según el complemento; no se sustituye esa ronda por otra.

Los peers muestran el precio de la política: el control puede seguir mientras las operaciones que exigen al peer detenido pierden servicio. En tránsito no basta que otros peers estén vivos si no satisfacen `AND(emisor,receptor)`. Una indisponibilidad permanente de una contraparte puede dejar el tránsito sin una resolución confirmable bajo la política vigente, incluso para el regulador. No se añade una recuperación unilateral desde EVAL-8.

Los fallos fueron de **contenedor en host único**. Aunque los orderers pertenecen lógicamente a organizaciones distintas, esto no ensaya pérdida física del host, infraestructura administrativa independiente ni recuperación ante desastre. Los componentes se restituyeron normalmente a los 60 s, salvo raft-2; las recuperaciones de peers de 66,4–80,1 s incluyen ese intervalo. Peer-1c añade una sonda regulatoria declarada: no se presenta como la mezcla core sin modificaciones. Los descartes iniciales y sus reintentos autorizados permanecen en la evidencia publicada.

EVAL-4/5 usó un host distinto de EVAL-6/7. Se comparan los comportamientos de cada campaña en su contexto; no se mezclan operaciones, latencias o tasas de las dos en una estadística común. Las issues #44/#45 permanecen abiertas: usar su evidencia publicada con límites no equivale a declarar cumplida toda su aceptación.

## 8. Cuatro asimetrías estructurales y sus consecuencias

| Asimetría | Evidencia | Beneficio o costo que debe conservarse |
|---|---|---|
| Historial nativo frente a tablas de aplicación | CC-5/10 y ADR-012 (§5.1). | Fabric reutiliza historia de claves; baseline mantiene eventos explícitos bajo su administrador. No es almacenamiento gratis ni el INSERT de autorizaciones una explicación de workloads donde no se ejecuta. |
| Push frente a pull | [Listener NET-8](anmat-event-listener.md), [#64](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/64). | Fabric entrega eventos confirmados por streaming; baseline expone consultas, sin transporte push equivalente. Un observador relacional necesita polling o un mecanismo adicional que no fue implementado ni medido aquí. |
| Confidencialidad que restringe una operación | DES-19 y seis de diez pares sin el laboratorio (§6.1). | Se preserva la privacidad comercial acotando el retiro del laboratorio ajeno al par durante tránsito. No es una ventaja funcional gratuita. |
| SBE por clave frente a retiro por lote | [CLI-4 / #114](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/114), [alcance](alcance-prototipo.md). | Una transacción sobre todas las unidades exigiría satisfacer las políticas de sus custodios; el cliente implementa una transacción por unidad y puede terminar con retiro parcial. |

El listener distingue eventos de negocio válidos de transacciones inválidas en bloques; una propuesta rechazada antes de ordenar queda fuera de ambos streams. Además, no persiste checkpoints y el replay puede duplicar avisos identificables por transactionId. Su existencia no demuestra entrega productiva exactamente una vez ni latencia regulatoria máxima: EVAL-6/7 no midió este servicio.

El retiro/prohibición por lote no redefine una operación unitaria del contrato ni garantiza atomicidad global. El cliente informa resultado por serial; una unidad ya retirada y las que fallaron no autorizan anunciar éxito del lote completo. Es una consecuencia del diseño y la granularidad de endoso adoptados, no una imposibilidad general de agrupar operaciones en toda plataforma distribuida.

## 9. Matriz de afirmación, evidencia, costo y límite

| Afirmación defendible | Evidencia concreta | Costo asociado | Alcance de la conclusión |
|---|---|---|---|
| La transferencia exige a las contrapartes previstas. | SBE y casos inválidos NET-6/9, políticas y correlación bloque/txID. | Dos confirmaciones, endosos y dependencia de los peers participantes; reintentos medidos. | No todo write requiere varias organizaciones; no prueba verdad física del medicamento. |
| La traza de cambios confirmados puede reconstruirse y contrastarse con el bloque. | CC-5/10, NET-6/9 y consultas de historial. | Persistencia y réplica; en baseline, tablas/eventos de aplicación. | Historial de clave, bloque con inválidas y propuestas rechazadas son fuentes distintas. |
| El contenido comercial se aísla de organizaciones no miembros. | Probe NET-5, extracto sanitizado y ADR-006. | PDC, diseminación y la restricción funcional DES-19. | Nombres de colección y metadatos revelan relaciones; no hay anonimato total. |
| La caída de un orderer no impide toda operación mientras queda quórum. | Balance raft-1 del release, con la restauración fallida declarada. | Replicación, elección de líder y recuperación operativa. | No ocho completed; perder dos orderers bloquea writes; no caída física del host. |
| La baseline de una API/BD concentra el servicio. | api-1/db-1: 0% global en falla. | Simplicidad y latencia menor, dependencia de esa administración/topología. | No se evaluaron otras topologías relacionales de alta disponibilidad. |
| La baseline respondió con menor latencia bajo las tasas ensayadas. | 20 escenarios finales, mismos perfiles/host y pares completos (§3). | Fabric ejecuta más mecanismos y transacciones por transferencia. | No se aisló cada causa ni se igualó presupuesto agregado; no es capacidad máxima. |

## 10. Cuándo se justifica el costo en este sistema de trazabilidad

La interpretación del trade-off depende de quién puede administrar y validar los registros y qué fallas se aceptan. La finalidad regulatoria no decide por sí sola la arquitectura ni transforma estas pruebas en una certificación de cumplimiento.

| Condición del problema | Lectura de los resultados |
|---|---|
| Actores independientes necesitan controlar conjuntamente cambios de custodia y verificar una traza compartida sin confiar sólo en un administrador. | El coendoso, la historia replicada y la visibilidad pública mínima aportan propiedades relevantes. Debe aceptarse la latencia medida, la operación de identidades/peers/PDC y la dependencia de las partes exigidas. |
| Existe una autoridad central aceptada por todos, con gobierno y auditoría suficientes para el caso. | La baseline es defendible por su menor latencia y menor coordinación distribuida; las pruebas no demuestran que blockchain sea necesaria. La confianza administrativa y los puntos de falla de la topología se hacen explícitos. |
| Se requiere intervención urgente aun si una contraparte está fuera de línea o el titular no integra el par privado. | El diseño actual exhibe un costo funcional/de disponibilidad que puede ser incompatible con ese requisito. No se lo oculta detrás del porcentaje global de éxito ni se inventa una excepción de endoso. |
| Se exige anonimato de relaciones, retiro por lote atómico o continuidad ante pérdida de host. | El prototipo no demuestra esas propiedades: metadatos visibles, retiros parciales y host único permanecen como límites. |

La evidencia final respalda la factibilidad funcional del prototipo bajo la matriz ensayada y una mejora estructural en control compartido respecto de la baseline implementada. No respalda una mejora global de rendimiento: la baseline tuvo menor latencia. La continuidad observada de Fabric fue selectiva y estuvo condicionada por quórum y endoso. La elección de Fabric se justifica sólo cuando esas propiedades de control y verificación independientes son necesarias y sus costos resultan aceptables para el proceso concreto.

Para extrapolar haría falta evidencia adicional sobre presupuesto de recursos comparable, saturación, infraestructura realmente distribuida, operación de claves y recuperación gobernada. No se ejecutó esa evaluación ni se amplió el prototipo desde EVAL-8. No se mezclan distintos hosts, se conservan variabilidad y fallas, y no se atribuye causalidad a un componente sin un experimento que la aísle.

## 11. Cobertura de los criterios de EVAL-8

| Criterio de #48 | Desarrollo y evidencia |
|---|---|
| Integridad y endoso multi-org | §4, NET-6/9, con límites de unilateralidad y paquetes de cada ejecución. |
| Auditabilidad, válidas e inválidas | §5, distinción ledger/historial/propuestas; CC-5/9/10. |
| Punto único de falla, EVAL-4 vs EVAL-5 | §7, complemento publicado, clases objetivo/control y restauración fallida. |
| Aislamiento de NET-5 y costo funcional DES-19 | §6, metadata visible y restricción de seis pares. |
| Trade-off y condiciones regulatorias | §§9–10, beneficios, costos, modelo de confianza y límites sin superioridad absoluta. |
| Cuatro asimetrías estructurales | §8 y su desarrollo en §§4–6. |
| Separación por perfil, transferencia de dos pasos y reintentos | §§3–4.1, valores finales y costo end-to-end sin inferencia causal exclusiva. |
| Rechazos separados y disponibilidad no agregada a rendimiento | §§3.2 y 7, con fuentes y campañas identificadas. |

## Fuentes de las tablas finales

[R67]: https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/blob/18d0d950817e38451953e056b418d49751b384ce/benchmarks/results/eval67/eval67-389ca16-20261021e-full/README.md
[CSV67]: https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/blob/18d0d950817e38451953e056b418d49751b384ce/benchmarks/results/eval67/eval67-389ca16-20261021e-full/analysis/aggregate.csv
[REPORT67]: https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/blob/18d0d950817e38451953e056b418d49751b384ce/benchmarks/results/eval67/eval67-389ca16-20261021e-full/analysis/report.md
[AV]: https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/releases/tag/evidence-eval4-eval5-citable
[AVFILE]: https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/releases/download/evidence-eval4-eval5-citable/analysis-eval4-eval5-citable-complement.tar.gz
