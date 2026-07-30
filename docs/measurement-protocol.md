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
| Transferencia | Transferencia de custodia | Write | Debe usar pares validos e invalidos derivados de la matriz regulatoria aprobada. |
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
- errores inesperados;
- timeouts;
- tasa efectiva enviada por el cliente.

Los rechazos esperados por reglas regulatorias o de dominio no se mezclan con el throughput exitoso de caminos felices. Si se miden, deben aparecer en rondas separadas de rechazo esperado, con su propia latencia y tasa de rechazo.

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
| Peer-1 | Mixta, 20 TPS | Caida de 1 peer de una organizacion | Las demas organizaciones continuan segun politicas vigentes. |

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

`metadata.json` debe registrar:

| Campo | Descripcion |
|---|---|
| `protocol` | `measurement-protocol`. |
| `repositoryCommit` | Commit del repositorio. |
| `sut` | `fabric` o `baseline`. |
| `host` | CPU, memoria, sistema operativo, WSL si aplica. |
| `docker` | Versiones de Docker y Compose. |
| `fabric` | Version de Fabric y Fabric CA si aplica. |
| `caliper` | Version de Caliper si aplica. |
| `datasetSeed` | `20260727`. |
| `datasetHash` | Hash del dataset usado. |
| `scenario` | Nombre estable del escenario. |
| `workers` | Cantidad de workers. |
| `targetTps` | TPS objetivo. |
| `durationSeconds` | Duracion medida. |
| `warmup` | Si la corrida fue warm-up o medicion. |

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
- [ ] Se definio la carpeta de salida de crudos.
- [ ] Se confirmo que no hay carga externa significativa en el host.

## Anexo A. Fuentes tecnicas externas

- Hyperledger Caliper: metricas de throughput/latencia, benchmark configuration, workloads, rate controllers y monitores: <https://caliper-doc-trial.readthedocs.io/en/latest/>.
- Hyperledger Caliper benchmark configuration: <https://caliper-doc-trial.readthedocs.io/en/latest/overview/bench-config/>.
- Hyperledger Caliper workload modules: <https://caliper-doc-trial.readthedocs.io/en/latest/overview/workload-module/>.
- Hyperledger Caliper rate controllers: <https://caliper-doc-trial.readthedocs.io/en/latest/references/rate-controllers/>.
- Hyperledger Fabric ordering service, release 2.5: <https://hyperledger-fabric.readthedocs.io/en/release-2.5/orderer/ordering_service.html>.
