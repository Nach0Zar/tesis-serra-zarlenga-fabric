# ADR-004: Modelado de la transferencia — despacho/recepción como dos transacciones

- **Estado**: Aceptado
- **Fecha**: 2026-08-13
- **Autores**: Serra, Zarlenga

---

## Contexto

El Sistema Nacional de Trazabilidad modela la transferencia de custodia como dos eventos con agentes detonantes distintos. La Disposición ANMAT 3683/2011, artículo 8, los enumera por separado: "distribución del producto a un eslabón posterior" (acción del emisor) y "recepción del producto en el establecimiento" (acción del receptor). La Resolución MS 435/2011 exige que cada agente registre los movimientos propios de su eslabón; no es un solo actor el que da fe de que entregó y recibió.

ADR-001 ya incorporó esta distinción en la máquina de estados: el estado `EN_TRANSITO` existe, las transiciones T02 y T03 lo originan (despacho), T04 lo resuelve (recepción), y T05 lo resuelve por rechazo (devolución). ADR-001 dejó explícitamente abierta la pregunta de si `EN_TRANSITO` se implementa como dos transacciones en cadena o como una operación atómica, derivándola a esta ADR.

El modelo de datos de DES-2 tiene una dependencia abierta hacia esta decisión: si se elige el modelo de dos fases, la semántica de `CustodioActual` durante el tránsito queda sin definir sin un campo adicional que distinga "quién tiene la custodia legal actual" de "quién fue declarado como destino pendiente de confirmar".

## Alternativas

**A. Dos transacciones de chaincode: despacho y recepción**

- El emisor invoca `DispatchTransfer` (o equivalente según DES-5), que transiciona la unidad de `EN_LABORATORIO` o `EN_CUSTODIA` a `EN_TRANSITO`. En ese momento el chaincode registra el identificador canónico del destinatario declarado en un campo separado del asset.
- El receptor invoca `ReceiveTransfer`, que transiciona de `EN_TRANSITO` a `EN_CUSTODIA` y actualiza `CustodioActual` al identificador del receptor. Solo puede invocar esta transacción la organización que figura como destinatario declarado.
- Si el receptor rechaza, invoca `ReturnProduct` (T05 de ADR-001), que transiciona a `DEVUELTO`.
- Dos actores distintos firman y endosan dos transacciones distintas, lo que mantiene la auditabilidad del hecho "el emisor declaró enviar" separada del hecho "el receptor aceptó recibir".
- Introduce complejidad: la ventana de tiempo entre despacho y recepción expone un estado `EN_TRANSITO` que el chaincode debe manejar correctamente ante eventos extraordinarios (T09, T13, T14, T15, T16 de ADR-001).
- Se adopta.

**B. Transacción atómica única**

- Una sola transacción de chaincode representa el despacho y la recepción simultáneamente. El asset pasa directamente de `EN_LABORATORIO`/`EN_CUSTODIA` a `EN_CUSTODIA` del nuevo custodio, sin pasar por `EN_TRANSITO`.
- Más simple de implementar.
- Requiere modificar ADR-001: `EN_TRANSITO`, T02, T03, T04 y T05 pierden razón de ser y deben eliminarse.
- No representa fielmente el proceso del SNT: supone que emisor y receptor firman el mismo evento en el mismo instante, lo cual no ocurre en operaciones reales de distribución farmacéutica con entregas posteriores a la facturación.
- No permite modelar el rechazo en recepción como evento separado; el rechazo quedaría fuera del alcance del prototipo o requeriría un mecanismo fuera de la cadena.
- Se descarta.

## Decisión

Se adopta la **alternativa A**: la transferencia se modela como dos transacciones de chaincode separadas, una invocada por el emisor (despacho, genera `EN_TRANSITO`) y otra invocada por el receptor (recepción, genera `EN_CUSTODIA` o `DEVUELTO`).

`CustodioActual` permanece como el emisor durante el tránsito. Se agrega el campo `DestinatarioPendiente` al asset para registrar el identificador canónico del receptor declarado, con significado solo cuando `Estado == EN_TRANSITO`.

## Justificación

La Disposición ANMAT 3683/2011, artículo 8, lista distribución y recepción como movimientos logísticos distinguibles, cada uno con su propio agente detonante. Modelarlos como una única transacción elimina del ledger la evidencia de cuándo ocurrió el despacho y cuándo ocurrió la recepción — información que la normativa exige registrar separadamente y que el rol de auditoría de ANMAT necesita para reconstruir la trazabilidad completa de cada unidad.

La alternativa B requeriría modificar ADR-001, que ya fue aceptada por el equipo como base del diseño del chaincode y el baseline. Revertir `EN_TRANSITO` a esta altura del diseño tiene un costo mayor que resolver la semántica de `CustodioActual` durante el tránsito con un campo adicional.

La alternativa A también preserva el valor de la evaluación comparativa: la baseline debe implementar el mismo modelo de dos fases para que la comparación de latencia y throughput entre chaincode y baseline corresponda a los mismos procesos del SNT.

## Semántica de CustodioActual y DestinatarioPendiente durante el tránsito

| Campo | Significado durante EN_TRANSITO | Significado fuera de EN_TRANSITO |
|---|---|---|
| `CustodioActual` | Identificador canónico del emisor (quien despachó y sigue siendo responsable legal). | Custodio actual de la unidad. |
| `DestinatarioPendiente` | Identificador canónico del receptor declarado por el emisor. El chaincode valida que quien invoca `ReceiveTransfer` coincida con este valor. | Campo vacío (`""`). |

Reglas de modificación de estos campos:
- **T02 / T03 (despacho)**: chaincode escribe `DestinatarioPendiente` con el destino declarado por el emisor; `CustodioActual` no cambia.
- **T04 (recepción)**: chaincode mueve el valor de `DestinatarioPendiente` a `CustodioActual`; `DestinatarioPendiente` se limpia a `""`.
- **T05 (rechazo/devolución en tránsito)**: chaincode transiciona a `DEVUELTO`; `CustodioActual` se mantiene en el emisor (la unidad rechazada vuelve al remitente); `DestinatarioPendiente` se limpia a `""`.

El campo `DestinatarioPendiente` es de solo-lectura para el cliente en todas las operaciones excepto el despacho, donde el cliente lo proporciona como parámetro. El chaincode valida que el destinatario declarado exista en el registro de organización-establecimiento (ADR-003), esté activo y tenga un `agentType` compatible con el par de transferencia según la matriz de DES-3, antes de aceptar el despacho.

## Visibilidad de DestinatarioPendiente

ADR-002 enumeró un conjunto cerrado de campos de estado público del canal (identificador, lote, vencimiento, custodio actual, estado del producto) y clasificó como privado "cualquier dato que permita inferir relaciones o volúmenes comerciales entre partes específicas". `DestinatarioPendiente` es un campo nuevo respecto de esa enumeración y expone, durante la ventana de tránsito, que el emisor despacha hacia un receptor determinado. Por eso su ubicación en estado público —en lugar de una colección privada compartida por emisor, receptor y `AnmatMSP`— requiere justificación explícita y no debe leerse como una extensión silenciosa de la clasificación de ADR-002.

Se decide mantener `DestinatarioPendiente` en el estado público del canal por dos razones:

1. **No agrega información comercial que no sea ya pública.** ADR-002 ya fijó que `CustodioActual`, y por lo tanto la secuencia histórica de custodios (consultable con `GetHistoryForKey`), es pública. Cuando la recepción se confirma, el receptor pasa a ser `CustodioActual` público de todos modos. `DestinatarioPendiente` solo adelanta durante el tránsito una relación custodio → siguiente-custodio que el historial público revela igual una vez completada la recepción. No expone precio, cantidades ni condiciones comerciales: esos datos van a PDC.
2. **La validación de recepción lo necesita legible por el peer endosante del receptor.** La transición T04 exige que el chaincode compruebe que el invocador coincide con el destinatario declarado. Mantener el campo en el estado público evita hacer depender de una colección privada una comprobación que es de control de acceso, no de confidencialidad comercial.

Lo que sí permanece privado es el contexto documental y comercial de la operación (factura, remito, cantidades), conforme a la clasificación de ADR-002 (ver la sección siguiente). Esta decisión debe documentarse en la tesis como una interpretación consciente de la frontera de confidencialidad, coherente con la advertencia que ADR-002 ya hace sobre la lectura del artículo 9 de la Disposición 3683/2011.

## Referencias documentales de la operación

La Disposición ANMAT 3683/2011 exige que la documentación comercial respaldatoria de toda transferencia incluya número de factura, remito y datos de identificación del origen y destino. Estos datos constituyen información documental en el sentido de ADR-002.

ADR-002 los clasifica explícitamente como privados: "número de factura o remito" forma parte de la "información comercial y documental" que se almacena en Private Data Collections, no en el estado público del canal.

Esta ADR confirma esa clasificación para ambas operaciones:
- **Despacho**: el emisor escribe en la PDC de la operación el número de remito/factura, cantidades despachadas y demás datos documentales exigidos. La membresía de la colección incluye al emisor, al receptor declarado y a `AnmatMSP`.
- **Recepción**: el receptor puede agregar al mismo contexto privado la confirmación de los datos documentales, o crear una entrada diferenciada si la normativa exige registro separado de la recepción.

El diseño concreto de las colecciones (nombres, política de membresía, persistencia del hash en el ledger compartido) queda para NET-5, tal como establece ADR-002.

## Rechazo en recepción y relación con EXT-4

El rechazo en recepción es la transición T05 de ADR-001: `EN_TRANSITO → DEVUELTO`, invocada por `DESTINATION_AGENT` o `CURRENT_CUSTODIAN`. El chaincode valida que quien invoca sea la organización declarada como destinatario o el emisor, y que la causa de rechazo esté documentada.

`DEVUELTO` es un estado no terminal de ADR-001 con las siguientes transiciones disponibles: `REINGRESAR_STOCK` (T25), `RETIRAR_MERCADO` (T19), `PROHIBIR_PRODUCTO` (T20), `DISPONER_FINAL` (T33), `INFORMAR_ROBO` (T14), `INFORMAR_EXTRAVIO` (T15), `INFORMAR_DETERIORO` (T16). La issue EXT-4 (devolución entre actores ya en custodia confirmada) cubre el mismo estado destino pero a partir de T21/T22/T23/T24 — los estados `DEVUELTO` generados por T05 y por EXT-4 son semánticamente equivalentes y comparten las mismas transiciones de resolución.

## Consecuencias

- **Para ADR-001**: no se requieren cambios. La alternativa A confirma la existencia de `EN_TRANSITO` y las transiciones T02–T05, que ADR-001 ya define. Esta ADR resuelve la única pregunta que ADR-001 dejó abierta (dos transacciones vs. atómica); el cambio de estado de ADR-001 queda a criterio del proceso de aprobación del equipo.
- **Para DES-2 / modelo-datos.md**: el struct `MedicationUnit` debe agregar el campo `DestinatarioPendiente string` con valor `""` fuera de `EN_TRANSITO`. La sección 5 de modelo-datos.md puede cerrar la dependencia abierta hacia DES-9.
- **Para DES-5 (issue #11)**: el contrato público del chaincode debe exponer dos operaciones distintas para la transferencia: una de despacho (invocable por el emisor) y otra de recepción (invocable solo por el destinatario declarado). Las firmas exactas, parámetros, errores y payloads quedan para DES-5.
- **Para CC-3 y BASE-2**: la implementación del chaincode y del baseline debe respetar el modelo de dos transacciones para que la comparación cuantitativa de DES-7 mida los mismos procesos en ambos prototipos.
- **Para NET-5 / ADR-002**: las colecciones privadas deben cubrir tanto el evento de despacho como el de recepción, con el mismo esquema de membresía (partes de la operación + `AnmatMSP`).
- **Se gana**: fidelidad normativa, auditabilidad del hecho separado de cada parte, soporte de rechazo en recepción sin mecanismos externos.
- **Se pierde / costo**: mayor complejidad de implementación; la ventana de tiempo en `EN_TRANSITO` debe manejarse correctamente ante eventos extraordinarios (T09, T13–T16 de ADR-001).
- **Queda pendiente**: el mecanismo de idempotencia para el caso en que el receptor invoque `ReceiveTransfer` más de una vez, y el comportamiento esperado si el despacho queda en `EN_TRANSITO` de forma indefinida sin que el receptor confirme ni rechace (queda fuera del alcance del prototipo salvo que una issue específica lo incorpore).

## Contexto utilizado

- Issue GitHub #57: DES-9 · ADR-004, consultada el 2026-08-13.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): fuente de los estados `EN_TRANSITO`, `DEVUELTO` y las transiciones T02–T05, T09, T13–T16.
- [ADR-002: Topología de canales](002-topologia-canales.md): clasificación de factura/remito como datos privados de PDC.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): base para validar existencia y habilitación del destinatario declarado.
- [DES-2 / docs/modelo-datos.md](../modelo-datos.md): dependencia abierta resuelta por esta ADR mediante el campo `DestinatarioPendiente`.
- Disposición ANMAT 3683/2011, artículo 8: enumera distribución al siguiente eslabón y recepción en establecimiento como movimientos logísticos separados con agentes detonantes distintos. URL: https://www.argentina.gob.ar/normativa/nacional/disposici%C3%B3n-3683-2011-182665/actualizacion
- Resolución MS 435/2011, artículo 1: exige que cada agente implemente un sistema de trazabilidad que permita el control y seguimiento de las unidades. URL: https://www.argentina.gob.ar/normativa/nacional/resoluci%C3%B3n-435-2011-180934/texto
