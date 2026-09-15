# ADR-013: Criterios de verificación de autenticidad por el adquirente

- **Estado**: Propuesto
- **Fecha**: 2026-08-28
- **Autores**: Serra, Zarlenga

---

## Contexto

La Disposición ANMAT 3683/2011 impone a todo miembro de la cadena que adquiere un medicamento la obligación de poder consultar si las unidades son originales y fueron distribuidas por la cadena legalmente autorizada. CC-7 (#61) materializa ese requisito, pero llega al código sin decisión de diseño previa: a diferencia de CC-8 (#62) —que tiene su semántica cerrada por [ADR-011](011-financier-trace-verification.md) y su firma congelada en el contrato desde la versión 2.1.0—, la operación del adquirente **no existe en la superficie congelada de `docs/api-contract.md`** ni tiene ADR que la defina. Sin esta decisión, CC-7 tendría que diseñar el contrato desde una issue de implementación, que es exactamente lo que la política de versionado del contrato prohíbe.

Las piezas que condicionan la decisión ya están fijadas:

- **[ADR-002](002-topologia-canales.md) ya describe este caso de uso, literalmente, y ya resolvió su regla de visibilidad.** Su Contexto plantea el problema en estos términos: «cuando una Droguería transfiere un medicamento a una Farmacia con la que nunca operó antes, esa Farmacia necesita poder verificar, antes de aceptar la custodia, que el producto no está vencido, robado, en cuarentena o retirado del mercado». Y su Decisión separa el **estado mínimo de trazabilidad** —identificador, lote, vencimiento, custodio actual y estado del producto—, de visibilidad amplia dentro del canal, de la **información comercial y documental**, restringida a las partes y a `AnmatMSP` mediante PDC. La pregunta abierta que enuncia CC-7 («¿booleano + estado, sin exponer la traza completa de terceros? — coordinar con ADR-002/NET-5») está, en su parte de confidencialidad, ya contestada.
- **[ADR-001](001-maquina-estados-medicamento.md)** define la máquina de estados versionada, la tabla de transiciones como fuente de verdad de qué secuencias son posibles, y la distinción entre estados **terminales** y estados **bloqueantes** (`VENCIDO`, `DETERIORADO`, `RETIRADO_MERCADO`, `PROHIBIDO`, `DEVUELTO`, `EN_CUARENTENA`), que bloquean la circulación ordinaria sin ser terminales.
- **[ADR-003](003-establishment-identity-gln-cufe.md)** define el registro organización-establecimiento como única fuente de resolución de identidad, y el patrón «Límites de garantía» que esta ADR replica.
- **[ADR-005](005-rol-organismo-financiador.md)** declara un supuesto de confianza del prototipo que condiciona el alcance de esta decisión: el acceso de **lectura** al estado público del canal no puede restringirse por chaincode.
- **[ADR-008](008-transfer-matrix-distribution.md)** fija la matriz de transferencias embebida y la declaración de matriz única durante toda la evaluación de v1.
- **[ADR-011](011-financier-trace-verification.md)** ya resolvió, para el financiador, la pregunta de fondo «¿veredicto evaluable o historial crudo?» y fijó dos comprobaciones —camino de estados válido contra ADR-001 y pares de transferencia autorizados contra la matriz de ADR-008— que son exactamente la noción de «cadena de custodia legítima» que CC-7 necesita.

El problema de diseño es doble. Primero: **qué distingue esta verificación de la del financiador**, para que no sean la misma operación con dos nombres. Segundo: **qué significa «respuesta acotada»** cuando ADR-005 ya declaró que la lectura del estado público no es restringible — es decir, si el acotamiento es una barrera de confidencialidad o una decisión de forma.

## Alternativas

**A. No agregar operación: el adquirente compone `ReadUnit` + `GetUnitHistory`**

- El contrato ya expone las dos lecturas necesarias; el adquirente las combina y juzga por su cuenta.
- No toca el contrato congelado ni agrega lógica al chaincode.
- Traslada la regla normativa fuera del prototipo, que es exactamente lo que ADR-011 descartó para el financiador por la misma razón: la legitimidad de una cadena de custodia dejaría de ser una regla determinística del sistema para ser la interpretación de cada consumidor, y la baseline (BASE-3) no tendría comportamiento definido que replicar. La hipótesis del trabajo —que la validación normativa se traslada al chaincode— quedaría sin materializar justo en la obligación que la Disposición impone al adquirente.
- Se descarta porque reproduce, para el adquirente, el defecto que ADR-011 ya identificó y descartó para el financiador.

**B. Reutilizar `VerifyTrace` para ambos consultantes**

- Una sola operación de verificación, con la checklist de ADR-011, habilitada además a los `agentType` custodiales.
- Evita duplicar lógica y mantiene una única noción de «traza legítima».
- No sirve: la checklist de ADR-011 **exige que la unidad esté `DISPENSADO`** (comprobación 2) y devuelve `NO_DISPENSADA` en cualquier otro caso. Es correcto para el financiador, cuya condición de pago nace de una dispensa ya ocurrida, y es inservible para el adquirente, cuyo caso de uso ocurre **antes** de aceptar la custodia, cuando la unidad está precisamente en `EN_TRANSITO` o `EN_CUSTODIA`. Aplicada al adquirente, la operación respondería `NO_DISPENSADA` en el 100 % de sus invocaciones legítimas.
- Se descarta porque la comprobación 2 de ADR-011 es incompatible con el momento en que el adquirente consulta.

**C. Operación propia con veredicto estructurado, compartiendo las comprobaciones de cadena con ADR-011**

- Se agrega `VerifyUnit` al contrato, con cuatro comprobaciones evaluadas en orden sobre el estado público y el historial, y un veredicto estructurado con la misma forma que el de ADR-011.
- Las dos comprobaciones de **cadena de custodia** (camino de estados y pares de transferencia) son literalmente las comprobaciones 4 y 5 de ADR-011 y se implementan **una sola vez**, en helpers compartidos que CC-8 consume: el mismo criterio que ADR-008 aplica a la matriz —una sola implementación, no dos que haya que mantener coherentes—.
- Las dos comprobaciones restantes son propias del adquirente y ninguna existe en ADR-011: **unicidad** de la unidad y **aptitud del estado actual para operar**.
- Costo: agrega una operación al contrato congelado (MINOR) y lógica de verificación que la baseline debe replicar.
- Se adopta.

## Decisión

Se adopta la **alternativa C**. La verificación de autenticidad del adquirente se materializa como la operación de solo lectura `VerifyUnit`, con una **checklist determinística de cuatro comprobaciones evaluadas en orden** sobre el estado público y el historial (`GetHistoryForKey`) de la unidad identificada por GTIN + número de serie.

### Checklist de verificación

1. **Existencia.** La unidad (GTIN+serie) existe en el world state. Si no existe, el veredicto es `NO_ENCONTRADA`. Es la comprobación que responde «¿es original?» en el único sentido que el ledger puede acreditar: el serial fue dado de alta por un laboratorio a través de T01 y no es un código inventado.
2. **Unicidad.** El historial de la clave registra **una sola creación** y ninguna eliminación posterior: la unidad no fue dada de alta, borrada y vuelta a crear bajo el mismo identificador. Si no, el veredicto es `UNIDAD_DUPLICADA`.

   Esta comprobación es, por construcción, redundante con dos invariantes ya vigentes —la clave compuesta `MedicationUnit`+[`gtin`,`numeroSerie`] es única en el world state, y `RegisterUnit` rechaza un alta repetida con `UNIT_ALREADY_EXISTS`—, y **se conserva deliberadamente igual**. La razón es que CC-7 la enuncia como criterio de aceptación y que una comprobación **recomputada desde el historial** es evidencia verificable de la invariante, no una afirmación sobre ella: es el mismo criterio con el que la comprobación 4 de ADR-011 recomputa el camino de estados en vez de confiar en que el chaincode lo validó al escribirlo. Lo que la comprobación no puede detectar se declara en «Límites de la verificación».
3. **Cadena de custodia legítima.** El historial completo corresponde a una vida posible de la unidad según ADR-001 **y** ADR-004. Si falla cualquiera de las invariantes de secuencia el veredicto es `SECUENCIA_INVALIDA`; si el par de agentes de una transferencia no está autorizado, `TRANSFERENCIA_NO_AUTORIZADA`.

   Las invariantes son seis, y se verifican **conjuntamente**:

   1. el primer estado observado es `EN_LABORATORIO`, único estado inicial de ADR-001;
   2. el primer custodio resuelve, contra el registro, a `agentType=LABORATORY`: T01 habilita solo a ese actor y `RegisterUnit` persiste al laboratorio invocante como primer custodio;
   3. cada par consecutivo de estados observados es una transición declarada en la tabla de ADR-001. **Incluidas las escrituras que no cambian el estado**: ADR-001 no declara transiciones sobre sí mismas, de modo que una segunda escritura con el mismo estado no corresponde a ninguna transición y es, por definición, una escritura que la máquina de estados no sanciona;
   4. `CustodioActual` cambia **si y solo si** la transición observada es `EN_TRANSITO → EN_CUSTODIA` (T04);
   5. esa transición, recíprocamente, **debe** cambiar el custodio: una recepción que no mueve la custodia no es una recepción;
   6. cuando la custodia cambia, el par (`agentType` origen → `agentType` destino) está autorizado por la matriz embebida de ADR-008, resolviendo cada custodio contra el registro vigente.

   **Por qué conjuntamente y no como dos proyecciones independientes.** ADR-004 acopla estado y custodia: el despacho (T02/T03) lleva la unidad a `EN_TRANSITO` **sin** cambiar `CustodioActual`, y solo la recepción (T04) materializa el cambio. Verificar «los estados forman un camino válido» y «los cambios de custodio están autorizados» por separado deja pasar historiales que violan ese acoplamiento aunque ambas proyecciones sean válidas. Dos ejemplos concretos:

   - `EN_LABORATORIO/laboratorio → EN_TRANSITO/droguería`: el par de estados es una transición declarada (T02) y el par de agentes está autorizado por la matriz, pero durante el tránsito la custodia registrada debe seguir siendo la del laboratorio. La invariante 4 lo rechaza;
   - `EN_CUSTODIA/droguería → EN_CUSTODIA/farmacia`: una transferencia consumada sin haber pasado nunca por `EN_TRANSITO`. Sin la invariante 3 el par de estados ni siquiera se examina —son iguales—, y sin la 4 el cambio de custodio pasa por el solo hecho de que la matriz autorice `DRUGSTORE → PHARMACY`.

   Que `CustodioActual` cambie únicamente en T04 no es una interpretación: ADR-004 lo fija para la transferencia ordinaria, y [ADR-009](009-return-and-recovery-semantics.md) (Decisión, punto 1) lo confirma para las cuatro vías hacia `DEVUELTO` —la custodia permanece en quien la tenía—, descartando expresamente la alternativa que la cambiaba porque «viola el principio establecido por ADR-004 de que ningún cambio de custodia se asienta sin un acto propio del receptor». Ninguna otra transición de la tabla de ADR-001 mueve la custodia.

   Son las comprobaciones 4 y 5 de ADR-011 **más el acoplamiento que ADR-004 impone**, con idénticos veredictos nombrados. **Deben implementarse una sola vez y ser consumidas por ambas operaciones**; CC-7, que llega primero, deja los helpers, y CC-8 los usa. Dos implementaciones de la misma regla es exactamente lo que ADR-008 prohíbe para la matriz, por el mismo motivo: divergirían en silencio. Y aquí el argumento es más fuerte que el de la economía de código: un punto ciego en este helper se propaga automáticamente a la verificación del financiador.
4. **Aptitud para operar.** Se evalúan, en este orden, tres condiciones sobre el estado público:

   - el estado actual es **terminal** según ADR-001 —`DISPENSADO`, `ROBADO`, `EXTRAVIADO`, `DISPUESTO_FINAL`—: veredicto `ESTADO_TERMINAL`;
   - el estado actual es **bloqueante** —`VENCIDO`, `DETERIORADO`, `RETIRADO_MERCADO`, `PROHIBIDO`, `DEVUELTO`, `EN_CUARENTENA`—: veredicto `ESTADO_BLOQUEANTE`;
   - la **fecha de vencimiento ya pasó**, aunque el evento `INFORMAR_VENCIMIENTO` todavía no se haya registrado: veredicto `VENCIDO_POR_FECHA`.

   Los tres reportan en el detalle el dato observado —el estado, o la fecha—.

   La separación entre bloqueante y terminal no es cosmética: un estado bloqueante puede resolverse —ADR-001 les conserva transiciones de salida— y uno terminal no. Para el adquirente son dos decisiones distintas: rechazar la recepción a la espera de una resolución, o rechazarla definitivamente porque la unidad ya salió de circulación.

   **Por qué el vencimiento por fecha es una comprobación y no un supuesto.** El paso del tiempo no ejecuta transacciones: `VENCIDO` se alcanza por T11/T12/T13, que alguien tiene que invocar. Hasta que eso ocurra, una unidad cuya `fechaVencimiento` ya pasó sigue registrada como `EN_CUSTODIA` o `EN_TRANSITO`, y una verificación que solo mirara el estado la declararía apta. Sería el peor resultado posible de esta operación: decirle a quien está por adquirir que un producto vencido es apto, cuando la fecha que lo desmiente está en el mismo estado público que la verificación ya está leyendo.

   ADR-002 justifica la visibilidad amplia del estado mínimo —que incluye `fechaVencimiento`— con este caso exacto: «esa Farmacia necesita poder verificar, antes de aceptar la custodia, que el producto **no está vencido**, robado, en cuarentena o retirado del mercado». Una operación que se presenta como verificación **independiente** previa a la recepción no puede depender de que otro actor haya informado algo que ella misma recomputa a partir de un dato que ya está leyendo.

   **Qué NO dice ADR-001, para no apoyarse en una lectura que no se sostiene.** La precondición de T06 enumera «no está vencida, en cuarentena, retirada, prohibida, robada, extraviada, deteriorada ni devuelta», y esos ocho términos son exactamente los ocho **estados** correspondientes (`VENCIDO`, `EN_CUARENTENA`, …): es una lista de estados, no una condición sobre la fecha. Cuando ADR-001 quiere hablar de la fecha lo hace con otra redacción, la de T11–T13: «la fecha de vencimiento fue alcanzada». De modo que ADR-001 **no** exige hoy, en ninguna transición, comparar `fechaVencimiento` contra el reloj de la transacción. Lo que sí deja abierto es la cláusula previa de T06, «la unidad **está apta para dispensación**», que la tabla no define y que esta ADR interpreta —para la verificación y para T06— incluyendo la vigencia por fecha.

   **Se le da veredicto propio y no `ESTADO_BLOQUEANTE`.** Los dos casos exigen acciones distintas del adquirente: ante un estado bloqueante el ledger ya registró la causa y hay un proceso en curso; ante un vencimiento por fecha el ledger todavía **no** registró nada, el adquirente está descubriendo una condición no informada, y la acción correcta es rechazar y además detonar `ReportExpired` (T13). Reportarlo como `ESTADO_BLOQUEANTE` afirmaría que el ledger bloquea la unidad cuando el ledger no dice nada de ella.

   **Semántica exacta de la comparación.** `fechaVencimiento` es una fecha `YYYY-MM-DD` (`modelo-datos.md` §3.2) y se interpreta como el **último día operable**, conforme el uso corriente de la fecha de vencimiento de un medicamento. La unidad está vencida cuando la fecha calendario UTC del timestamp de la transacción es **estrictamente posterior** a `fechaVencimiento`. Con `fechaVencimiento = 2026-08-28`, una consulta del 28 la considera apta y una del 29 vencida.

   El instante de comparación sale **siempre** de `GetTxTimestamp()` y **nunca** del reloj local: el reloj local da un valor distinto en cada peer y rompería el determinismo que el modelo de endoso exige, además de volver el veredicto irreproducible para un auditor. Es el mismo criterio que ADR-007 (punto 6.f) ya fija para el vencimiento de las autorizaciones de intervención.

### Salida: veredicto estructurado

La operación devuelve un veredicto con la misma forma que el de ADR-011, más el estado observado:

```json
{
  "autentica": false,
  "motivo": "ESTADO_BLOQUEANTE",
  "estado": "RETIRADO_MERCADO",
  "verificaciones": [ { "check": "...", "resultado": "...", "detalle": "..." } ]
}
```

- `autentica` es `true` si y solo si las cuatro comprobaciones pasan; en ese caso `motivo` queda vacío.
- Valores de `motivo`: `NO_ENCONTRADA`, `UNIDAD_DUPLICADA`, `SECUENCIA_INVALIDA`, `TRANSFERENCIA_NO_AUTORIZADA`, `ESTADO_TERMINAL`, `ESTADO_BLOQUEANTE`, `VENCIDO_POR_FECHA`.
- `motivo` es el veredicto nombrado de la primera comprobación que falla, en el orden declarado.
- `estado` es el estado público actual de la unidad, vacío si la unidad no existe. Es el «booleano + estado» que CC-7 pide: le da al adquirente la decisión evaluable y el dato que necesita para saber qué hacer a continuación, sin obligarlo a interpretar un historial.
- `verificaciones` reporta cada comprobación con su resultado y su detalle.

### Qué significa «respuesta acotada», con precisión

CC-7 pide una respuesta «acotada a lo que el consultante puede saber, sin exponer la traza completa de terceros». Esta ADR fija el alcance exacto de esa frase, y lo hace declarando lo que el mecanismo **no** es:

- **No es una barrera de confidencialidad.** ADR-005 ya declaró que el acceso de lectura al estado público del canal no puede restringirse por chaincode, y `GetUnitHistory` está en el contrato y es invocable por cualquier miembro del canal. Un adquirente que quiera el historial crudo lo tiene, con o sin esta operación. Afirmar lo contrario sería vender como control de acceso algo que no lo es.
- **Sí es una decisión de forma, y sí tiene un efecto real de confidencialidad, pero por otra vía.** El veredicto se computa **exclusivamente sobre el estado mínimo de trazabilidad** que ADR-002 declaró de visibilidad amplia. No lee ninguna colección privada: ni el registro de operación de ADR-004 —destinatario declarado, remito, factura, cantidades— ni ningún marcador. La operación no puede filtrar información comercial porque no la consulta, lo cual es una propiedad estructural y no una promesa.
- **El acotamiento sustantivo es el mismo que ADR-011 justificó para el financiador**: al adquirente le sirve una condición evaluable —«¿la acepto?»— y no un dump para interpretar. La diferencia con `VerifyTrace` no es cuánto se oculta, sino qué se pregunta y en qué momento de la vida de la unidad.

### Autorización

`VerifyUnit` exige un invocador **registrado y habilitado** (ADR-003): produce `ORG_NOT_REGISTERED` y `ORG_INACTIVE` como cualquier operación que resuelve identidad. No exige un `agentType` ni un `snt.role` determinados.

Las dos decisiones tienen motivo, y conviene que quede escrito porque son asimétricas respecto de `VerifyTrace`:

- **Se exige registro y habilitación** porque la Disposición impone la obligación de consulta a los **miembros de la cadena**, y porque es la única condición que el chaincode puede verificar sin contradecir a ADR-005. Es además coherente con el resto del contrato: toda operación que resuelve identidad la exige.
- **No se exige `agentType` ni rol** —a diferencia de `VerifyTrace`, que sí restringe a `FINANCIER` y `REGULATOR`— porque el consultante es, en palabras de CC-7, «un actor común»: cualquier eslabón que esté por adquirir. Restringirlo a los `agentType` custodiales excluiría sin motivo al regulador y al financiador, y sobre todo sería una restricción **aparente**: la misma información es alcanzable con `ReadUnit` y `GetUnitHistory`, que no autorizan en absoluto. Una barrera que no detiene nada es peor que ninguna, porque induce a confiar en ella.

## Justificación

- **Cierra la única obligación normativa del prototipo que no tenía decisión de diseño.** ADR-011 hizo esto para el financiador; el adquirente quedaba con su caso de uso descrito en el Contexto de ADR-002 y sin operación que lo materializara.
- **Es la operación que ADR-002 necesita para sostener su propia justificación.** ADR-002 eligió canal único con PDC —en lugar de múltiples canales— argumentando que el destinatario debe poder «validar de forma independiente antes de aceptar una custodia», y que la lectura estricta del artículo 9 destruiría esa capacidad. Esa capacidad estaba, hasta acá, disponible pero no materializada: existía el dato, no la verificación. `VerifyUnit` la vuelve una operación del sistema y no un ejercicio de interpretación del consumidor.
- **No duplica reglas.** Las dos comprobaciones de cadena se comparten con CC-8 por construcción, con el mismo criterio con el que ADR-008 impuso una matriz única para chaincode y baseline. Las dos comprobaciones propias del adquirente —unicidad y aptitud del estado— no existen en ADR-011 y no tienen dónde reutilizarse.
- **El orden de evaluación sigue el costo y la precedencia lógica**, igual que ADR-011: existencia y unicidad se responden antes de auditar la cadena completa, y la aptitud del estado actual se evalúa al final porque es la única que puede fallar sobre una unidad cuya traza es impecable —el caso más frecuente en el uso real: producto legítimo, retirado del mercado—.
- **Los límites se declaran en lugar de simularse resueltos**, replicando el patrón «Límites de garantía» de ADR-003 y la sección homóloga de ADR-011.

## Límites de la verificación

Qué **no** acredita `VerifyUnit`. Estos límites deben acompañar a la operación en su documentación de contrato y en la tesis:

- **Autenticidad física.** La verificación acredita la traza registrada, no la posesión física efectiva, la autenticidad material del envase ni la ausencia de **clonación** del código serializado. Es el límite más importante de declarar acá, porque es el que la palabra «autenticidad» del enunciado de CC-7 puede sugerir que se resuelve: un envase falsificado que reproduzca un GTIN + serie legítimo obtiene un veredicto `autentica: true`, porque el ledger no distingue el envase del identificador. Límite heredado textualmente de los «Límites de garantía» de ADR-003.
- **Alcance de la comprobación de unicidad.** La comprobación 2 detecta una recreación de la clave en el world state; **no** detecta que dos envases físicos porten el mismo serial, que es la forma en que la duplicación ocurre en la realidad. Ese caso es indistinguible desde el ledger: ambos envases resuelven a la misma y única clave.
- **Habilitación histórica de actores.** El registro persiste el `active` **actual** (ADR-003): la verificación resuelve `agentType` contra el registro vigente y no valida que cada actor de la cadena estuviera habilitado en el momento de su evento. Límite heredado de ADR-011 (su alternativa C) y del diseño del registro.
- **Versión histórica de la matriz.** v1 opera con una única versión de matriz (ADR-008): los pares del historial se re-evalúan contra la matriz vigente, no contra la que autorizó cada despacho.
- **Transacciones inválidas o rechazadas.** `GetHistoryForKey` devuelve solo modificaciones confirmadas: la verificación audita lo que ocurrió, no lo que se intentó (ADR-005).
- **No es una autorización de recepción.** Un veredicto `autentica: true` informa que la traza es legítima y el estado admite operar; **no** dice que el invocador sea el destinatario declarado de una transferencia en curso ni que esté autorizado a recibirla. Eso lo decide `ReceiveTransfer` contra el registro de operación privado (ADR-004), y esta operación no lo sustituye ni lo anticipa.

## Consecuencias

- **Para `docs/api-contract.md` (DES-5)**: se incorpora `VerifyUnit(ctx, gtin, numeroSerie) (*UnitVerdict, error)` y el tipo `UnitVerdict`. Es un **agregado MINOR** —nueva operación, ninguna firma existente cambia, ningún `code` altera su semántica— y por lo tanto exige la aprobación explícita de B antes del merge, conforme la política de versionado del propio contrato.
- **Para CC-7 (#61)**: la story queda implementable con criterio de aceptación concreto —las cuatro comprobaciones en su orden, los veredictos nombrados y la forma del resultado— y sin decisiones de contrato pendientes. Implementa, no diseña.
- **Para CC-8 (#62)**: las comprobaciones 4 y 5 de ADR-011 quedan implementadas y probadas por CC-7 como helpers compartidos, **reforzadas con el acoplamiento estado-custodia de ADR-004** que ninguna de las dos enunciaba por separado. CC-8 las consume en lugar de reescribirlas; **no** debe volver a implementarlas. La contracara de compartir es que un punto ciego del helper se propaga a la verificación del financiador sin que nadie lo note, y por eso las seis invariantes de la comprobación 3 se prueban sobre el helper y no solo a través de `VerifyUnit`.
- **Para ADR-011**: su comprobación 4 («cada par consecutivo de estados observados es una transición declarada») queda precisada en dos puntos que su redacción dejaba abiertos y que esta ADR resuelve para ambas operaciones: las escrituras que no cambian el estado **no** se saltean —ADR-001 no declara transiciones sobre sí mismas—, y el camino de estados se verifica junto con la custodia, no como proyección independiente. No es una corrección de su semántica sino la explicitación de lo que ADR-004 ya imponía; si una revisión futura de ADR-011 quisiera apartarse, debe hacerlo en el mismo PR que revise esta ADR, porque comparten implementación.
- **Para BASE-3 (#39) y la baseline**: la baseline replica el mismo veredicto, comprobación por comprobación, sobre el mismo paquete compartido, para preservar la paridad funcional de DES-7.
- **Para ADR-002**: su justificación funcional —el destinatario valida de forma independiente antes de aceptar la custodia— pasa de ser una capacidad latente a una operación del contrato. No cambia ninguna de sus reglas.
- **Se gana**: la obligación de consulta del adquirente queda materializada como regla determinística del sistema y no como interpretación del consumidor; la justificación funcional de ADR-002 queda respaldada por una operación concreta; CC-8 se descarga de la mitad de su checklist.
- **Se pierde / costo**: una operación más en el contrato congelado y en la superficie que la baseline debe replicar; una noción de «autenticidad» que el ledger acredita solo parcialmente y cuyos límites hay que declarar con insistencia para no sobrevender la garantía.
- **Para `Dispense` (T06), y por coherencia del sistema**: la misma comparación se aplica a la dispensación. Sin ella el prototipo quedaría diciendo dos cosas distintas sobre la misma unidad —`VerifyUnit` respondiendo `VENCIDO_POR_FECHA` y `Dispense` dejando dispensarla—, y esa contradicción **la introduciría esta ADR**: antes de ella ninguna operación tenía opinión sobre la fecha. Entre las dos salidas coherentes posibles, la correcta es la que no permite entregarle a un paciente un medicamento cuya fecha de vencimiento el propio ledger registra como pasada. El rechazo usa `INVALID_STATE_TRANSITION`, que es el código con el que el contrato ya expresa que T06 no es admisible para esa unidad; no se agrega ningún `code`. La cláusula «la unidad está apta para dispensación» de la precondición de T06 es la que habilita la lectura, y esta ADR la interpreta explícitamente en lugar de dejarla indefinida.
- **Queda pendiente**: si una revisión futura quisiera detectar clonación de seriales, haría falta un mecanismo fuera del ledger (soporte físico verificable), que ninguna decisión actual contempla. La habilitación histórica de actores sigue pendiente del versionado del registro, igual que en ADR-011.

## Divergencia con el trabajo escrito

No hay divergencia. El trabajo escrito enuncia la obligación del adquirente de verificar origen y legitimidad de la cadena; esta ADR la materializa y declara explícitamente qué parte de «autenticidad» el ledger **no** acredita —autenticidad material y clonación del serial—, para que la tesis lo liste como limitación consciente y no como omisión.

## Contexto utilizado

- Disposición ANMAT 3683/2011, artículos 8 y 9: obligación de los miembros de la cadena de consultar el origen y la legitimidad de las unidades adquiridas, y confidencialidad de la información de las transacciones.
- [ADR-001: Máquina de estados del medicamento](001-maquina-estados-medicamento.md): tabla de transiciones para la comprobación 3; estados terminales y bloqueantes para la comprobación 4.
- [ADR-002: Topología de canales](002-topologia-canales.md): el caso de uso del adquirente en su Contexto y la regla de visibilidad —estado mínimo de trazabilidad amplio, información comercial en PDC— que fija el alcance de la respuesta.
- [ADR-003: Identidad de establecimientos mediante GLN/CUFE](003-establishment-identity-gln-cufe.md): registro usado para resolver `agentType`; patrón «Límites de garantía».
- [ADR-004: Transferencia como despacho y recepción](004-transfer-dispatch-reception.md): el registro de operación privado que esta operación no consulta y cuya validación no sustituye.
- [ADR-005: Rol del organismo financiador en la dispensación](005-rol-organismo-financiador.md): supuesto de confianza sobre la lectura del estado público, que determina que el acotamiento no sea una barrera de acceso.
- [ADR-008: Distribución y versionado de la matriz de transferencias](008-transfer-matrix-distribution.md): matriz embebida para la comprobación 3 y criterio de implementación única.
- [ADR-011: Criterios de verificación de traza para el organismo financiador](011-financier-trace-verification.md): comprobaciones 4 y 5 compartidas, forma del veredicto estructurado y precedente de la decisión «veredicto evaluable, no historial crudo».
- [`docs/api-contract.md`](../api-contract.md): superficie congelada y política de versionado que clasifica este agregado como MINOR.
- Issue GitHub #61: CC-7 · Verificación de autenticidad por el adquirente, consultada el 2026-08-28.
- Issue GitHub #62: CC-8 · Verificación de trazabilidad por el financiador, consultada el 2026-08-28.
