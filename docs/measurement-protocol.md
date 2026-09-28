# Protocolo de medicion y comparacion experimental

## 1. Objetivo

Este documento define el protocolo experimental para comparar el prototipo Hyperledger Fabric del Sistema Nacional de Trazabilidad de Medicamentos con una linea base centralizada funcionalmente equivalente.

La comparacion debe ejecutarse antes de cualquier conclusion sobre desempeno o disponibilidad. Su objetivo es producir evidencia reproducible sobre:

- latencia de operaciones de escritura y lectura;
- throughput de operaciones exitosas;
- disponibilidad ante fallas controladas;
- condiciones de paridad entre Fabric y baseline;
- tratamiento estadistico de repeticiones.

El protocolo cubre la dimension cuantitativa del trabajo. Integridad y auditabilidad se tratan como propiedades cualitativas o estructurales mediante evidencia de diseno, endoso, ledger, historial y logs cuando aplique.

## 2. Alcance experimental

Operaciones core a medir:

| Grupo | Operacion conceptual | Tipo | Observaciones |
|---|---|---|---|
| Registro | Registro de lote o unidades | Write | Debe representar el alta inicial por laboratorio. |
| Transferencia | Transferencia de custodia | Write | Debe usar pares validos e invalidos derivados de la matriz regulatoria aprobada. Conforme ADR-004, una transferencia son DOS transacciones: despacho y recepcion (ver seccion 3.4). |
| Dispensacion | Dispensacion | Write | Debe cerrar el ciclo de una unidad dispensada sin persistir datos personales sensibles. |
| Consulta puntual | Consulta de unidad | Read | Debe leer el estado actual de una unidad por identificador. |
| Consulta de historial | Consulta de traza o historial | Read | Debe recuperar evidencia de trazabilidad/auditoria segun el contrato vigente. |

Los nombres concretos de funciones de chaincode y endpoints REST se definen en los artefactos de interfaz e implementacion correspondientes. Este protocolo solo fija la equivalencia metodologica.

Quedan fuera de alcance:

- implementar Caliper, workloads, scripts, clientes o generadores;
- definir firmas del chaincode o endpoints REST;
- cambiar modelos de datos, estados, permisos, MSP, canales o politicas de endoso;
- ejecutar benchmarks finales;
- registrar resultados experimentales reales.

## 3. Definiciones de metricas

### 3.1 Latencia

**Latencia write**: tiempo transcurrido desde que el cliente envia la solicitud hasta que recibe confirmacion de commit durable.

- En Fabric, la medicion termina cuando el cliente confirma que la transaccion fue commiteada y validada por los peers del canal.
- En baseline, la medicion termina cuando la API responde despues de confirmar la escritura en la base relacional.
- No se debe medir solo el tiempo de propuesta, endoso o envio al orderer si la operacion todavia no fue confirmada como commiteada.

**Latencia read/query**: tiempo transcurrido desde que el cliente envia la consulta hasta que recibe la respuesta completa.

Reportar para cada ronda:

- minimo;
- maximo;
- media;
- desvio estandar;
- p50;
- p95;
- p99.

### 3.2 Throughput

**Throughput exitoso**: cantidad de operaciones exitosas por segundo durante la ventana medida.

Debe reportarse junto con:

- operaciones intentadas;
- operaciones exitosas;
- rechazos esperados por reglas de negocio;
- reintentos operativos transitorios;
- errores inesperados;
- timeouts;
- tasa efectiva enviada por el cliente.

Los rechazos esperados por reglas regulatorias o de dominio no se mezclan con el throughput exitoso de caminos felices. Si se miden, deben aparecer en rondas separadas de rechazo esperado, con su propia latencia y tasa de rechazo.

Los reintentos operativos transitorios tampoco son rechazos de negocio. Se registran por separado con su causa, cantidad de intentos y tiempo acumulado, pero el trabajo necesario hasta confirmar la operación permanece dentro de la latencia y del throughput observados del camino feliz correspondiente.

### 3.3 Disponibilidad

**Disponibilidad experimental**: proporcion de operaciones que completan correctamente durante una ventana de falla controlada.

Para cada escenario de falla se debe reportar:

- tasa de exito antes, durante y despues de la falla;
- latencia p95 antes, durante y despues de la falla;
- throughput antes, durante y despues de la falla;
- tiempo de recuperacion;
- errores y timeouts observados;
- logs que expliquen la condicion de falla.

**Tiempo de recuperacion**: intervalo entre la inyeccion de la falla y el primer tramo estable de 30 segundos en el que la tasa de exito vuelve al menos al 95% de la tasa previa a la falla.

### 3.4 Unidad de medida de la transferencia (ADR-004)

Conforme ADR-004, la transferencia de custodia se implementa como dos transacciones encadenadas: despacho (invocado por el emisor) y recepcion (invocada por el receptor). Para la medicion:

- Cada transaccion se mide por separado, con su propia latencia write (confirmacion de commit) y su contribucion propia al throughput.
- Se reporta ademas la metrica derivada **latencia end-to-end del par**: tiempo desde el envio del despacho hasta la confirmacion de commit de la recepcion correspondiente, ejecutando la recepcion inmediatamente despues de confirmado el despacho (sin espera artificial).
- En los perfiles de carga, "1 operacion de transferencia" = 1 par completo despacho+recepcion = 2 transacciones write. Las tasas objetivo de transferencia se expresan en pares por segundo y la tasa efectiva de transacciones es el doble.
- La baseline debe exponer y medir los mismos dos pasos con la misma semantica (BASE-2); de lo contrario la comparacion no mide los mismos procesos.
- Con `requiredPeerCount: 1` (ADR-006), el peer receptor puede no disponer todavía del registro privado cuando se intenta `ReceiveTransfer`. El cliente aplica un reintento controlado hasta que el *pull* o la reconciliación permitan leerlo, o hasta el timeout de la operación. Este caso es un **reintento operativo transitorio**, no T05 ni un rechazo esperado de negocio.
- La latencia end-to-end del par incluye el primer intento fallido, la espera entre intentos y todos los reintentos hasta la confirmación de commit de la recepción. Se reportan además la cantidad y tasa de pares que necesitaron reintento. Excluir ese tiempo ocultaría un costo real del diseño Fabric frente a la baseline.
- Los rechazos en recepcion (transicion T05) pertenecen a las rondas de rechazo esperado (seccion 6.5), no al camino feliz.

### 3.5 Costo de los marcadores de participación (ADR-007)

Los marcadores de participación definidos en ADR-007, punto 6, agregan una escritura privada y el hash correspondiente al *read-write set*. Ese trabajo adicional se registra como métrica específica y no se atribuye implícitamente sólo a la escritura del estado público.

Por escenario se reportan:

- cantidad total de escrituras de marcador confirmadas, desglosada entre altas `RegisterUnit` y eventos regulatorios;
- tasa de escrituras de marcador por segundo y proporción respecto de las transacciones write exitosas;
- cantidad esperada frente a cantidad observada, para detectar marcadores omitidos o duplicados.

La latencia y el throughput de la operación ya incluyen el costo del marcador. La métrica separada atribuye el volumen de trabajo adicional, pero no lo resta ni presenta una ejecución sin marcadores como resultado comparable, porque esa ablación cambiaría la política de endoso evaluada.

## 4. Dataset compartido

El dataset debe ser sintetico, deterministico y consumido por Fabric y baseline sin modificaciones semanticas.

Parametros obligatorios:

| Parametro | Valor |
|---|---|
| Seed | `20260727` |
| Unidades minimas | 50.000 |
| Identificacion | GTIN + numero de serie |
| Metadatos minimos | lote, vencimiento, custodio inicial, estado inicial |
| Cadenas validas | Deben cubrir registro, transferencia y dispensacion |
| Cadenas invalidas | Deben cubrir pares prohibidos y estados no operables cuando esos artefactos existan |

Reglas de uso:

- Fabric y baseline deben iniciar cada repeticion desde el mismo snapshot logico del dataset.
- Una unidad no debe reutilizarse dentro de la misma repeticion para dos escrituras incompatibles de estado.
- Las operaciones de transferencia validas deben derivar de la matriz regulatoria aprobada para pares origen-destino.
- Las operaciones invalidas deben separarse en rondas especificas de rechazo esperado.
- Si 50.000 unidades no alcanzan para todas las rondas sin reutilizacion indebida, el generador debe producir `max(50000, unidades_requeridas * 1.2)`.
- El dataset o su receta de generacion debe registrar seed, parametros, version del generador y hash del archivo producido.

La construcción del snapshot lógico inicial se realiza mediante al menos 50.000 altas `RegisterUnit` exitosas en cada SUT. En Fabric, cada alta agrega un marcador en la colección implícita del laboratorio, por lo que el mínimo implica 50.000 escrituras privadas adicionales y sus hashes. La preparación del dataset debe reportar por separado duración, throughput efectivo y cantidad de marcadores; esas cifras no se mezclan con las rondas medidas. Si una repetición restaura un snapshot en vez de repetir las altas, debe verificarse que conserva el estado público, los datos privados y los marcadores equivalentes.

## 5. Condiciones identicas entre Fabric y baseline

Cada medicion comparativa debe cumplir:

- mismo host fisico;
- misma configuracion de CPU, memoria y WSL/Docker documentada;
- mismo commit del repositorio o commits registrados si Fabric y baseline se ejecutan desde ramas distintas;
- mismo dataset, seed y orden de entradas;
- mismas operaciones conceptuales;
- misma duracion de rondas;
- misma cantidad de workers;
- misma tasa objetivo;
- misma politica de warm-up y descarte;
- mismo tratamiento de errores;
- misma ventana temporal para inyeccion de fallas;
- ejecucion no concurrente de Fabric y baseline, salvo decision experimental posterior.

Antes de cada repeticion medida:

1. detener cualquier corrida anterior;
2. limpiar estado temporal no versionado;
3. reiniciar el SUT correspondiente;
4. cargar el dataset inicial;
5. verificar conectividad con una corrida smoke;
6. registrar metadatos de entorno.

Para reducir sesgo por orden de ejecucion, las repeticiones deben alternar el SUT inicial:

```text
R1: Fabric -> baseline
R2: baseline -> Fabric
R3: Fabric -> baseline
R4: baseline -> Fabric
R5: Fabric -> baseline
```

## 6. Perfiles de carga

Los perfiles usan `fixed-rate` como controlador base. En Caliper esto corresponde a una tasa objetivo acumulada entre todos los workers.

### 6.1 Smoke

Objetivo: verificar conectividad, contrato minimo y disponibilidad del SUT antes de medir.

| Parametro | Valor |
|---|---|
| Workers | 1 |
| Rate controller | `fixed-rate` |
| TPS | 1 |
| Transacciones | 30 por operacion core |
| Repeticiones | 1 |
| Uso estadistico | No se incluye en resultados finales |

Si smoke falla, no se ejecutan rondas medidas para ese SUT.

### 6.2 Escrituras core

Objetivo: medir registro, transferencia y dispensacion por separado.

| Operacion | Workers | TPS | Duracion | Rondas |
|---|---:|---:|---:|---|
| Registro | 2 | 5, 10, 20 | 120 s | Una por tasa |
| Transferencia valida | 2 | 5, 10, 20 | 120 s | Una por tasa |
| Dispensacion | 2 | 5, 10, 20 | 120 s | Una por tasa |

Cada ronda debe usar `txDuration: 120`. La tasa efectiva observada debe registrarse junto con la tasa objetivo.

Para la fila Transferencia valida, la tasa objetivo se expresa en pares despacho+recepcion por segundo; la tasa efectiva de transacciones write es aproximadamente el doble (seccion 3.4).

### 6.3 Lecturas

Objetivo: medir consulta puntual e historial/traza.

| Operacion | Workers | TPS | Duracion | Rondas |
|---|---:|---:|---:|---|
| Consulta de unidad | 4 | 10, 25, 50 | 120 s | Una por tasa |
| Consulta de historial | 4 | 10, 25, 50 | 120 s | Una por tasa |

Las consultas deben leer unidades existentes y distribuidas de forma deterministica entre workers.

### 6.4 Carga mixta

Objetivo: medir un flujo representativo con operaciones combinadas.

| Parametro | Valor |
|---|---|
| Workers | 2 |
| Rate controller | `fixed-rate` |
| TPS | 20 |
| Duracion | 120 s |
| Registro | 10% |
| Transferencia valida | 55% |
| Dispensacion | 10% |
| Consulta | 25% |

La mezcla exacta debe implementarse de forma deterministica a partir de la seed y del indice de worker para que Fabric y baseline reciban la misma secuencia conceptual.

El 55% de transferencia valida se cuenta en pares completos despacho+recepcion (seccion 3.4).

### 6.5 Rechazos esperados

Objetivo: medir costo de validacion de operaciones invalidas sin contaminar el camino feliz.

| Parametro | Valor |
|---|---|
| Workers | 2 |
| TPS | 5 |
| Duracion | 60 s |
| Casos | Pares prohibidos, duplicados o estados bloqueantes segun artefactos disponibles |
| Resultado esperado | Rechazo controlado, no error inesperado |

Estas rondas reportan latencia y tasa de rechazo esperado. No se suman al throughput exitoso.

La indisponibilidad temporal del registro privado en el peer receptor (ADR-006, `requiredPeerCount: 1`) no pertenece a estas rondas: no expresa una regla regulatoria ni de dominio. Se conserva como reintento operativo del camino feliz conforme la seccion 3.4.

## 7. Repeticiones y estadistica

Para cada escenario medido:

1. ejecutar 1 warm-up descartado;
2. ejecutar 5 repeticiones medidas;
3. calcular media, desvio estandar, p50, p95 y p99 por repeticion;
4. calcular media y desvio estandar entre repeticiones;
5. calcular coeficiente de variacion sobre throughput y p95.

Si el coeficiente de variacion de throughput o p95 supera 15%, ejecutar 3 repeticiones adicionales y reportar:

- serie original de 5 repeticiones;
- serie extendida de 8 repeticiones;
- posible causa observada de variabilidad.

No se deben eliminar outliers sin justificacion documentada. Si una corrida se invalida por una falla operacional externa al escenario, debe conservarse el registro crudo y marcarse como corrida descartada con motivo.

## 8. Disponibilidad

### 8.1 Fabric Raft

Escenarios de disponibilidad para Fabric:

| Escenario | Carga activa | Falla | Resultado a observar |
|---|---|---|---|
| Raft-1 | Mixta, 20 TPS | Caida de 1 orderer en cluster de 3 | La red conserva quorum y continua operando. |
| Raft-2 | Mixta, 20 TPS | Caida de 2 orderers en cluster de 3 | La red pierde quorum y deja de ordenar nuevas transacciones. |
| Peer-1a · custodio | Mixta, 20 TPS | Caída del peer del custodio actual de las unidades objetivo | Ninguna escritura cuya SBE de reposo exija ese peer puede confirmarse; las operaciones sobre unidades que no lo requieren sirven como control. |
| Peer-1b · tránsito | Mixta, 20 TPS | Caída del peer de una de las dos partes de un tránsito activo | El tránsito objetivo no puede resolverse por recepción, rechazo, evento extraordinario ni intervención regulatoria mientras falte uno de los endosos de `AND(emisor, receptor)`. |
| Peer-1c · regulador | Mixta, 20 TPS | Caída del peer de la organización regulatoria | Ningún evento regulatorio que escriba su marcador de participación puede confirmarse; las operaciones custodiales que no requieren al regulador sirven como control. |
| Peer-1d · laboratorio | Mixta, 20 TPS | Caída del peer del laboratorio que intenta registrar unidades | Las altas `RegisterUnit` de ese laboratorio no pueden confirmarse porque falta el endoso exigido por su marcador; las altas de otros laboratorios y las operaciones no relacionadas sirven como control. |

Peer-1a–Peer-1d se ejecutan como rondas separadas. Cada ronda combina operaciones objetivo cuyo conjunto de endosos incluye al peer caído con operaciones de control que no lo requieren; la disponibilidad se reporta por clase de operación y endoso requerido, no sólo como un agregado global de throughput.

Ventana recomendada por escenario:

| Fase | Duracion | Descripcion |
|---|---:|---|
| Pre-falla | 60 s | Carga estable antes de inyectar la falla. |
| Falla | 60 s | Falla activa y observacion de impacto. |
| Recuperacion | 60 s | Nodo restaurado cuando el escenario lo requiera. |

Registrar logs de orderers, peers y cliente. Para Raft, conservar evidencia de quorum, perdida de quorum o re-eleccion de lider segun corresponda.

### 8.2 Baseline centralizada

Escenarios de disponibilidad para baseline:

| Escenario | Carga activa | Falla | Resultado a observar |
|---|---|---|---|
| DB-1 | Mixta, 20 TPS | Caida de PostgreSQL | El sistema completo deja de operar para operaciones dependientes de la base. |
| API-1 | Mixta, 20 TPS | Caida de API REST | El sistema completo deja de responder a clientes. |

Usar las mismas ventanas temporales que Fabric. Reportar tasa de exito, timeouts, errores HTTP y tiempo de recuperacion despues de restaurar el componente.

## 9. Evidencia cruda y estructura de resultados

Cada corrida debe dejar evidencia procesable. El HTML de Caliper puede guardarse como complemento, pero no reemplaza los crudos.

Estructura sugerida para resultados:

```text
benchmarks/results/
  <YYYYMMDD-HHMMSS>/
    metadata.json
    dataset/
      manifest.json
      dataset.sha256
    fabric/
      <scenario>/
        warmup/
        run-01/
        run-02/
        run-03/
        run-04/
        run-05/
    baseline/
      <scenario>/
        warmup/
        run-01/
        run-02/
        run-03/
        run-04/
        run-05/
```

Cada `run-*` debe incluir, cuando aplique:

- `raw.csv` o `raw.json` con una fila por operacion;
- reporte Caliper procesable;
- logs del cliente de carga;
- logs del SUT;
- errores y timeouts;
- timestamp de inicio y fin;
- parametros efectivos de la ronda.

### 9.1 Contrato de metadatos de la corrida (DES-20)

`metadata.json` no es una lista libre de campos: es un documento validable contra un contrato unico y versionado, para que Fabric y la baseline produzcan evidencia comparable de forma automatica.

| Artefacto | Ubicacion |
|---|---|
| Contrato | [`benchmarks/schema/run-metadata.schema.json`](../benchmarks/schema/run-metadata.schema.json) |
| Identificador | `urn:pfi-snt:run-metadata:schema:1.0.0` |
| Ejemplos validos | [`benchmarks/examples/`](../benchmarks/examples/) |
| Validador | `go run ./cmd/runmeta <metadata.json>` desde `client/` |

El contrato es JSON Schema Draft 2020-12 con `additionalProperties: false` en todos sus niveles: un campo no previsto invalida el documento en vez de pasar inadvertido.

Los contadores enteros tienen techo en 2^53-1, el mayor entero exacto en doble precision. Por encima, un consumidor que interprete el JSON con numeros de punto flotante --- como los workloads de Caliper, escritos en JavaScript --- leeria un valor distinto del escrito, y la evidencia dejaria de significar lo mismo para quien la produce y para quien la procesa.

Bloque comun, obligatorio para ambos SUT:

| Campo | Descripcion |
|---|---|
| `$schema`, `schemaVersion` | Version del contrato aplicada. |
| `protocol` | `measurement-protocol`. |
| `repositoryCommit` | Commit completo del repositorio desde el que se ejecuto (seccion 5). |
| `sut` | `fabric` o `baseline`. |
| `scenario` | Nombre estable del escenario, de la lista cerrada de la seccion 9.2. |
| `phase` | `preparation` (seccion 4), `warmup` o `measurement` (seccion 7). |
| `repetition` | `0` en warm-up; `1`..`8` en mediciones, cubriendo la serie original de 5 y la extendida de 3. La preparacion usa el numero de la repeticion que habilita. |
| `dataset` | `seed`, `sha256` del bundle y cantidad de unidades (seccion 4). |
| `workers` | Cantidad de workers. |
| `durationSeconds` | Duracion efectivamente medida. Obligatoria salvo en el smoke, que la seccion 6.1 acota por cantidad. |
| `transactions` | Exclusiva del smoke: cantidad de transacciones de la ronda. |
| `rate` | Tasas objetivo y efectiva, desambiguadas segun la seccion 9.3. |
| `rejectionFamily` | Exclusiva de `expected-rejections` (seccion 9.4). |
| `faultInjection` | Obligatoria y exclusiva de los escenarios de disponibilidad (seccion 9.5). |
| `participationMarkers` | Marcadores de participacion escritos; solo Fabric, y obligatoria en la preparacion (seccion 9.6). |
| `startedAt`, `endedAt` | Inicio y fin en RFC 3339. |
| `host` | CPU, memoria, sistema operativo, kernel y WSL si aplica. |
| `environment` | Versiones del entorno y de los artefactos medidos. |
| `discarded` | Solo si la corrida se invalida por una falla externa al escenario (seccion 7); el crudo se conserva igual. |

Identificadores propios de cada SUT, exigidos condicionalmente por `sut`. Son obligatorios porque sin ellos la evidencia no identifica que artefacto se midio, y son mutuamente excluyentes porque declarar los del otro SUT indica que la corrida se etiqueto mal:

| `sut` | Campos obligatorios en `environment` | Campos prohibidos |
|---|---|---|
| `fabric` | `contractVersion`, `packageID`, `fabric`, `fabricCA`, `caliper`, `docker`, `dockerCompose` | `baselineCommit`, `baselineImage` |
| `baseline` | `baselineCommit`, `baselineImage`, `docker`, `dockerCompose` | `contractVersion`, `packageID`, `fabric`, `fabricCA` |

`contractVersion` es la version congelada en [`docs/api-contract.md`](api-contract.md) y `packageID` es el `package_id` de [`network/chaincode-package.lock`](../network/chaincode-package.lock), en formato `label:sha256`. Juntos fijan que codigo de chaincode produjo la medicion.

### 9.2 Escenarios validos por SUT

Los nombres salen de las tablas de las secciones 6 y 8 y son una lista cerrada; un escenario no previsto invalida el documento.

| Escenario | Origen | SUT que puede declararlo |
|---|---|---|
| `dataset-preparation` | Seccion 4 | Ambos |
| `smoke` | Seccion 6.1 | Ambos |
| `write-register`, `write-transfer`, `write-dispense` | Seccion 6.2 | Ambos |
| `read-unit`, `read-history` | Seccion 6.3 | Ambos |
| `mixed` | Seccion 6.4 | Ambos |
| `expected-rejections` | Seccion 6.5 | Ambos |
| `raft-1`, `raft-2`, `peer-1a`, `peer-1b`, `peer-1c`, `peer-1d` | Seccion 8.1 | Solo `fabric` |
| `db-1`, `api-1` | Seccion 8.2 | Solo `baseline` |

El escenario y la operacion medida deben coincidir: `write-transfer` exige `rate.operation: transfer`, `read-history` exige `query-history`, y la carga mixta junto con todos los escenarios de disponibilidad exigen `mixed`.

### 9.3 Tasa de operaciones frente a tasa de transacciones

`targetTps` era ambiguo: no distinguia la operacion conceptual del protocolo de la transaccion efectiva enviada al SUT. Conforme la seccion 3.4, una transferencia es UNA operacion conceptual y DOS transacciones write, de modo que una misma cifra podia significar dos cargas distintas. El objeto `rate` separa ambas magnitudes:

| Campo | Significado |
|---|---|
| `operation` | Operacion conceptual medida: `register`, `transfer`, `dispense`, `query-unit`, `query-history` o `mixed`. |
| `transactionsPerOperation` | Transacciones efectivas por operacion conceptual: `2` para transferencia, `1` para el resto, y el promedio ponderado por la mezcla en `mixed`. |
| `targetOperationsPerSecond` | Tasa objetivo en operaciones conceptuales por segundo. Para transferencia son pares despacho+recepcion por segundo. |
| `targetTransactionsPerSecond` | Tasa objetivo en transacciones efectivas por segundo. Debe ser igual a `targetOperationsPerSecond` por `transactionsPerOperation`. |
| `effectiveTransactionsPerSecond` | Tasa efectiva observada durante la ronda. |
| `mix` | Obligatoria y exclusiva de `operation: mixed`: porcentajes de la seccion 6.4 sobre operaciones conceptuales, que deben sumar 100. |

La equivalencia de dos transacciones por transferencia rige en el camino feliz. En una ronda de rechazo esperado no: el dataset compartido invoca `DispatchTransfer` y el rechazo se resuelve ahi, de modo que el par nunca se completa y la operacion vale **una** transaccion. Exigir dos obligaria a declarar una recepcion que no se envio, que es la misma ambiguedad que `targetTps` producia.

El validador comprueba la aritmetica, que JSON Schema no puede expresar: que la tasa de transacciones derive de la de operaciones, que la mezcla sume 100 y que su promedio ponderado coincida con `transactionsPerOperation`. Tambien verifica que la ventana entre `startedAt` y `endedAt` no sea mas corta que la duracion que la corrida dice haber medido.

### 9.4 Rondas de rechazo esperado

`rejectionFamily` es obligatoria y exclusiva de `expected-rejections`, y usa la misma nomenclatura que la categoria del dataset compartido, para que la evidencia se pueda cruzar con el bundle que la produjo:

| Familia | Operacion admitida | Invocacion del dataset |
|---|---|---|
| `UNAUTHORIZED_TRANSFER` | `transfer` | `DispatchTransfer` |
| `DUPLICATE_IDENTITY` | `register` | `RegisterUnit` |
| `BLOCKING_STATE` | `transfer` o `dispense` | `DispatchTransfer` o `Dispense` |

Cada familia se mide en su propia ronda. Sin esa separacion, el costo de validacion de las tres se promedia en una sola cifra y el analisis comparativo de EVAL-8 (#48) no puede distinguirlas.

Reejecutar un mismo caso dentro de una ronda es valido: un rechazo no muta estado, de modo que repetirlo mide lo mismo. Lo que no se admite es fabricar casos por fuera del dataset compartido.

### 9.5 Escenarios de disponibilidad

`faultInjection` es obligatoria y exclusiva de los escenarios de la seccion 8. Sin el instante de inyeccion, el crudo no se puede separar en las tres ventanas y el tiempo de recuperacion de la seccion 3.3 no es calculable, de modo que la corrida no sirve para EVAL-4 (#44) ni EVAL-5 (#45) aunque contenga datos.

| Campo | Descripcion |
|---|---|
| `target` | Componente dado de baja, en los terminos de las tablas 8.1 y 8.2. |
| `injectedAt` | Instante de la inyeccion, dentro de la ventana medida. |
| `recoveredAt` | Instante de la restauracion. Ausente si el escenario no restaura el componente, como la perdida de quorum de Raft-2. |
| `preFaultSeconds`, `faultSeconds`, `recoverySeconds` | Duracion de cada ventana. La recomendada es 60 s cada una. |

`recoveredAt` y `recoverySeconds` van juntas o no van. El validador comprueba ademas que `injectedAt` caiga dentro de la corrida, que el tramo entre el inicio y la inyeccion coincida con `preFaultSeconds`, que el tramo entre inyeccion y recuperacion coincida con `faultSeconds`, y que las ventanas no excedan la duracion medida.

### 9.6 Preparacion del snapshot y marcadores de participacion

La construccion del snapshot inicial de la seccion 4 es su propia fase y no se mezcla con las rondas medidas: se declara con `scenario: dataset-preparation` y `phase: preparation`, y ambos se exigen mutuamente.

`participationMarkers` registra las escrituras de marcador de ADR-007 punto 6 que la seccion 3.5 pide atribuir. Solo Fabric las produce, asi que la baseline tiene prohibido el bloque. Es obligatorio en la preparacion de Fabric, donde las 50.000 altas implican 50.000 escrituras privadas adicionales, y opcional en el resto de los escenarios de Fabric.

El bloque cubre los tres reportes que pide la seccion 3.5 y son obligatorios juntos:

| Campo | Descripcion |
|---|---|
| `expected` | Cantidad esperada segun el escenario. |
| `observed` | Escrituras de marcador confirmadas. |
| `fromRegistrations`, `fromRegulatoryEvents` | Desglose por origen; debe sumar `observed`. |
| `successfulWriteTransactions` | Denominador de la proporcion: transacciones write exitosas de la ronda. |
| `perSecond` | Tasa de marcadores por segundo; debe ser `observed` dividido `durationSeconds`. |
| `shareOfSuccessfulWrites` | Proporcion entre 0 y 1; debe ser `observed` dividido `successfulWriteTransactions`. |

La comparacion entre `expected` y `observed` es la que detecta marcadores omitidos o duplicados. La tasa y la proporcion son las que atribuyen el volumen de trabajo adicional sin restarlo de la latencia ni presentar una ejecucion sin marcadores como resultado comparable, conforme la seccion 3.5.

El validador recalcula la tasa y la proporcion con una tolerancia mas laxa que la de las tasas objetivo, porque son cifras informadas y redondeadas para el reporte; alcanza igual para detectar un denominador equivocado.

## 10. Procesamiento de resultados

Para cada SUT y escenario, el procesamiento debe producir:

- tabla de resultados por repeticion;
- tabla agregada con media, desvio estandar, p50, p95, p99 y coeficiente de variacion;
- comparacion Fabric vs baseline por operacion y tasa;
- grafico de throughput efectivo;
- grafico de latencia p50/p95/p99;
- grafico de disponibilidad por ventana temporal en escenarios de falla;
- listado de errores inesperados y timeouts.

La comparacion debe distinguir:

- resultados cuantitativos medidos;
- propiedades cualitativas observadas;
- interpretaciones tecnicas;
- limitaciones del entorno experimental.

No se debe concluir superioridad absoluta de una arquitectura a partir de una sola dimension. El analisis final debe caracterizar el trade-off entre costo operativo, latencia, disponibilidad, integridad y auditabilidad.

## 11. Criterios de aceptacion

- [x] Latencia, throughput y disponibilidad quedan definidos con precision.
- [x] Cargas, cantidad de transacciones, tasa, workers y duracion quedan especificados.
- [x] Repeticiones y tratamiento estadistico quedan definidos.
- [x] Condiciones identicas para Fabric y baseline quedan explicitadas.
- [x] Dataset sintetico compartido queda especificado.
- [x] El artefacto se entrega como `docs/measurement-protocol.md`.

## 12. Checklist previo a medir

Antes de medir:

- [ ] El contrato de operaciones equivalentes esta congelado para la corrida.
- [ ] El generador produce el dataset con seed `20260727`.
- [ ] Fabric y baseline implementan los mismos procesos core.
- [ ] El dataset fue cargado en ambos SUT desde el mismo snapshot logico.
- [ ] Smoke pasa en Fabric.
- [ ] Smoke pasa en baseline.
- [ ] Se registro metadata de entorno.
- [ ] El `metadata.json` de la corrida valida contra el contrato de la seccion 9.1.
- [ ] Se definio la carpeta de salida de crudos.
- [ ] Se confirmo que no hay carga externa significativa en el host.

## Anexo A. Fuentes tecnicas externas

- Hyperledger Caliper: metricas de throughput/latencia, benchmark configuration, workloads, rate controllers y monitores: <https://caliper-doc-trial.readthedocs.io/en/latest/>.
- Hyperledger Caliper benchmark configuration: <https://caliper-doc-trial.readthedocs.io/en/latest/overview/bench-config/>.
- Hyperledger Caliper workload modules: <https://caliper-doc-trial.readthedocs.io/en/latest/overview/workload-module/>.
- Hyperledger Caliper rate controllers: <https://caliper-doc-trial.readthedocs.io/en/latest/references/rate-controllers/>.
- Hyperledger Fabric ordering service, release 2.5: <https://hyperledger-fabric.readthedocs.io/en/release-2.5/orderer/ordering_service.html>.
- ADR-004 (docs/adr/004-transfer-dispatch-reception.md): modelo de dos transacciones que fija la unidad de medida de la transferencia.
