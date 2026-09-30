# Smoke de Hyperledger Caliper (EVAL-1)

Este módulo fija Hyperledger Caliper `0.7.1` y el binding
`fabric:fabric-gateway`. El binding usa Fabric Gateway y es compatible con la
red Fabric `2.5.16` del repositorio. Caliper se ejecuta en modo
`--caliper-flow-only-test`: la creación del canal y el lifecycle del chaincode
siguen perteneciendo a los scripts de `network/`.

La ronda de EVAL-1 es deliberadamente mínima y diagnóstica:

- canal `snt-channel`, chaincode `snt` e identidad `User1` de `LabMSP`;
- una sola operación `ReadUnit`;
- 1 worker, controlador `fixed-rate`, 1 TPS y 30 consultas;
- una unidad tomada del primer `RegisterUnit` de `LabMSP` del dataset
  compartido, registrada fuera de la ronda sólo si aún no existe.

Los cinco workloads core, la carga completa del dataset, las repeticiones y el
procesamiento estadístico permanecen fuera de este setup.

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
| `metadata.json` | Evidencia DES-20 emitida al cerrar la ronda. |

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
