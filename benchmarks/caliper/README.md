# Workloads de Hyperledger Caliper

Este módulo fija Hyperledger Caliper `0.7.1` y el binding
`fabric:fabric-gateway`. El binding usa Fabric Gateway y es compatible con la
red Fabric `2.5.16` del repositorio. Caliper se ejecuta en modo
`--caliper-flow-only-test`: la creación del canal y el lifecycle del chaincode
siguen perteneciendo a los scripts de `network/`.

La ronda de smoke es deliberadamente mínima y diagnóstica:

- canal `snt-channel`, chaincode `snt` e identidad `User1` de `LabMSP`;
- una sola operación `ReadUnit`;
- 1 worker, controlador `fixed-rate`, 1 TPS y 30 consultas;
- una unidad tomada del primer `RegisterUnit` de `LabMSP` del dataset
  compartido, registrada fuera de la ronda sólo si aún no existe.

El runner medible agrega una ronda individual por operación core, la carga
mixta y los rechazos esperados. La carga completa del snapshot, la serie de
repeticiones y el procesamiento comparativo continúan fuera de este módulo.

## Requisitos

- Node.js 22 y npm 11.5.1 o posterior dentro de la serie 11;
- Go según `client/go.mod`;
- Docker Engine y Docker Compose;
- red local levantada, canal creado, chaincode desplegado y verificación de red
  exitosa;
- bundle determinístico de 50.000 unidades en `build/dataset`.

Desde la raíz del repositorio:

```bash
./network/network.sh up
./network/network.sh createChannel
./network/network.sh deployCC
./network/network.sh verify

cd client
go run ./cmd/datasetgen --units 50000 --output-dir ../build/dataset

cd ../benchmarks/caliper
npm ci
npm run check
npm test
npm run smoke
```

La documentación de Caliper 0.7.1 enumera Node.js 20 y 22, pero los paquetes
NPM publicados para esa versión declaran `node >=22` y `npm >=11.5.1`. Este
proyecto usa la intersección soportada y fija npm `11.6.2` mediante
`packageManager`.

No es necesario repetir `caliper bind`: el `package.json` y el lockfile ya
registran las dependencias exactas que produjo el binding
`fabric:fabric-gateway`. El script `npm run bind:fabric` se conserva únicamente
para actualizar conscientemente ese binding junto con el lockfile.

## Salidas

Cada ejecución crea un directorio nuevo en
`build/benchmarks/caliper/<timestamp>/` con:

| Archivo | Contenido |
|---|---|
| `network-config.json` | Configuración efectiva de `LabMSP`, peer, canal y chaincode; referencia las credenciales generadas sin copiarlas. |
| `benchmark-config.json` | Ronda exacta de 30 `ReadUnit` a 1 TPS. |
| `run-context.json` | Fuentes, versiones y host usados para construir el metadata. |
| `report.html` | Reporte Caliper con throughput y latencias. |
| `metadata.json` | Evidencia validada contra el contrato de metadata al cerrar la ronda. |

El runner valida el hash del dataset, lee la versión contractual desde
`docs/api-contract.md`, lee el `packageID` desde
`network/chaincode-package.lock` y los registra en `info`, en el contexto y en
`metadata.json`. Al terminar vuelve a validar el metadata con
`client/cmd/runmeta` y falla si no hubo exactamente 30 lecturas exitosas o si
el reporte no contiene throughput y latencias.

Las salidas, el dataset, certificados, claves y demás material generado están
ignorados por Git. El runner nunca imprime ni incorpora contenido PEM en sus
archivos; la configuración efectiva contiene solamente rutas locales.

Para elegir un identificador explícito y repetible del directorio de salida se
puede definir `SNT_CALIPER_RUN_TOKEN` con letras, números, punto, guion o guion
bajo. Para usar otro bundle generado, se puede definir
`SNT_CALIPER_DATASET_DIR` con su ruta absoluta o relativa a la raíz.

## Rondas medibles

El runner `npm run round` consume exclusivamente recetas del bundle compartido
con seed `20260727`. Prepara fuera de la ventana medida sólo los prefijos de
receta requeridos por la ronda: conserva el orden dentro de cada receta y
procesa con concurrencia acotada unidades disjuntas. Nunca crea el canal, instala chaincode
ni reinicia la red. Cada ronda debe comenzar sobre el estado limpio que
corresponda al protocolo; encontrar una unidad objetivo ya mutada se considera
contaminación experimental y la operación falla.

La interfaz es:

```text
npm run round -- \
  --scenario <write-register|write-transfer|write-dispense|read-unit|read-history|mixed|expected-rejections> \
  --phase <warmup|measurement> \
  --repetition <0..8> \
  --rate <tasa permitida> \
  [--family <UNAUTHORIZED_TRANSFER|DUPLICATE_IDENTITY|BLOCKING_STATE>] \
  [--operation <transfer|dispense>] \
  [--dataset-dir <ruta>] \
  [--run-token <identificador>]
```

`warmup` exige repetición `0`; `measurement`, una repetición entre `1` y `8`.
Las tasas admitidas son las fijadas por el protocolo de medición:

| Escenario | Workers | Operaciones/s | Duración |
|---|---:|---:|---:|
| `write-register`, `write-transfer`, `write-dispense` | 2 | 5, 10 o 20 | 120 s |
| `read-unit`, `read-history` | 4 | 10, 25 o 50 | 120 s |
| `mixed` | 2 | 20 | 120 s |
| `expected-rejections` | 2 | 5 | 60 s |

La transferencia se agenda en pares/s: cada operación conceptual ejecuta
`DispatchTransfer` y, apenas se confirma su commit, envía `ReceiveTransfer` sin
sondeo ni espera de sincronización previa. Registra ambas transacciones, cada
intento fallido y la latencia end-to-end completa. El controlador Caliper se
configura con la tasa de transacciones equivalente (dos por par; el factor de la
mezcla se deriva de sus porcentajes) para conservar la tasa conceptual del
protocolo.

La recepción correlacionada se reintenta cuando el peer receptor aún no ve los
datos privados (`INTERNAL_ERROR` reintentable con causa
`PRIVATE_DATA_NOT_DISSEMINATED`) o todavía observa el estado público anterior
(`NOT_IN_TRANSIT`). Este último código sólo es transitorio dentro del par,
inmediatamente después de un despacho confirmado; fuera de esa ruta conserva su
semántica contractual final. La preparación, que está fuera de la ventana
medida, sí puede esperar a que los peers observen el estado requerido. En la
carga mixta, el ciclo determinístico contiene 2 registros, 11 transferencias, 2
dispensaciones y 5 `ReadUnit` por cada 20 operaciones.

Los rechazos usan las familias y códigos esperados del dataset. Se ejecutan en
rondas separadas: `UNAUTHORIZED_TRANSFER`, `DUPLICATE_IDENTITY` y
`BLOCKING_STATE`; esta última admite una ronda `transfer` y otra `dispense`.
Un rechazo con otro código, o un éxito inesperado, descarta la ronda.

Ejemplos:

```bash
npm run round -- --scenario write-transfer --phase warmup --repetition 0 --rate 5
npm run round -- --scenario mixed --phase measurement --repetition 1 --rate 20
npm run round -- --scenario expected-rejections --phase measurement --repetition 1 \
  --rate 5 --family BLOCKING_STATE --operation dispense
```

Además de `network-config.json`, `benchmark-config.json`, `report.html` y
`metadata.json`, el runner conserva:

| Archivo | Contenido |
|---|---|
| `work-plan.json` | Secuencia determinística, particionada por worker, y preparaciones fuera de medición. |
| `raw.json` | Una entrada por operación conceptual, con las transacciones, timestamps, latencias, reintentos y códigos de rechazo. |
| `summary.json` | Conteos, tasas y latencias por función en JSON procesable. |
| `raw-worker-*.jsonl` | Partes append-only producidas por cada proceso worker. |

Los percentiles del resumen usan nearest-rank sobre las observaciones de la
ronda. Las tasas efectivas dividen los conteos por la ventana observada entre la
primera operación iniciada y la última finalizada, sin usar una duración nominal
menor; `summary.json` expone además los pares transferidos, los pares que
requirieron reintento y su proporción. Producir menos operaciones que el objetivo
es un resultado de throughput, no por sí solo una causa de descarte. El HTML de
Caliper es complementario: `raw.json`, `summary.json` y `metadata.json` son
la evidencia procesable. El runner conserva los crudos, marca
`discarded.reason` y termina con error ante fallos operativos, resultados
inesperados o pares incompletos. Los crudos y resúmenes no contienen transient data ni material
PEM; las configuraciones efectivas sólo referencian rutas locales.
`work-plan.json` sí contiene las recetas transient sintéticas provenientes del
dataset para que los workers puedan ejecutarlas, por lo que permanece bajo el
directorio `build/` ignorado por Git y no debe publicarse como resultado.
