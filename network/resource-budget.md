# Presupuesto de recursos de la red

Este presupuesto materializa el dimensionamiento exigido por NET-3 y ADR-007.
Distingue límites configurados, estimaciones conservadoras y mediciones
empíricas. Una estimación no se presenta como resultado medido.

## Techo configurado

`compose.yaml` aplica límites por contenedor para que una corrida no consuma
silenciosamente más recursos que los disponibles en el host común.

| Componente | Cantidad | Límite unitario | CPU unitaria | Memoria total | CPU total |
|---|---:|---:|---:|---:|---:|
| Fabric CA | 1 | 128 MiB | 0,20 | 128 MiB | 0,20 |
| Orderer | 3 | 192 MiB | 0,25 | 576 MiB | 0,75 |
| Peer | 7 | 384 MiB | 0,40 | 2.688 MiB | 2,80 |
| Subtotal de red | 11 | — | — | 3.392 MiB | 3,75 |
| Chaincode dinámico, peor caso | 7 | 192 MiB | 0,20 | 1.344 MiB | 1,40 |
| Total modelado | 18 | — | — | 4.736 MiB | 5,15 |

El peor caso supone una instancia del chaincode activa por peer. Fabric puede
crear más contenedores durante upgrades o pruebas paralelas; esas corridas no
son comparables hasta volver al inventario documentado.

Se reservan 2 GiB adicionales para kernel WSL2, daemon Docker, page cache,
cliente, Caliper y herramientas. El total planificado es 6.784 MiB, por debajo
del techo homogéneo de 8 GiB de [`wslconfig.example`](wslconfig.example), con
1.408 MiB de margen. Si una corrida alcanza límites u OOM, se invalida: no se
aumenta un host de forma aislada porque el protocolo exige igual CPU y memoria
para Fabric y baseline.

## Medición reproducible

Con los once servicios en estado `healthy`:

```bash
./network/scripts/measure-resources.sh
```

El script registra versiones, memoria/CPU de WSL, estado de salud, una muestra
`docker stats --no-stream` y ocupación persistente por contenedor. Para una
medición de carga se ejecuta al menos:

1. en reposo, después de estabilizar la red;
2. durante el perfil sostenido;
3. después de construir el snapshot de 50.000 unidades;
4. tras detener la carga y sincronizar todos los peers.

Cada host debe ejecutar la misma revisión del repositorio, imágenes y
`wslconfig`. Los resultados crudos se guardan con la metadata exigida en
`docs/measurement-protocol.md`; este documento no inventa valores para el
segundo equipo.

En el host disponible al implementar NET-3, WSL informó 33.201.737.728 bytes de
RAM y 16 procesadores, pero no existía `.wslconfig`; Docker Compose era 5.3.0.
Con Docker 29.6.1 y el material local de NET-1, los once contenedores alcanzaron
`healthy`. La muestra en reposo registró un máximo de 68,57 MiB por peer, 32,38
MiB para Fabric CA y 12,87 MiB para un orderer, todos bajo los límites de
`compose.yaml`. Esta observación valida el arranque y el instrumento, no el
consumo bajo carga.

La repetición en el segundo host se ejecutó sobre el commit `7ba7e39`, con el
perfil homogéneo aplicado: 6 procesadores, 8.327.917.568 bytes visibles de RAM y
4.294.967.296 bytes de swap. Con Docker 29.7.2 y Compose 5.4.0, los once
contenedores alcanzaron `healthy`, sin reinicios ni OOM. La muestra en reposo
registró máximos de 42,61 MiB para un peer, 33,32 MiB para Fabric CA y 14,18 MiB
para un orderer, todos bajo sus límites.

Esta repetición confirma que el arranque y el instrumento entran en el techo
homogéneo en el segundo equipo. La carga sostenida, los 50.000 marcadores,
benchmark y recuperación permanecen como perfiles de DES-7 y no se presentan
como resultados de NET-3.

## Costo de los marcadores de participación

ADR-007, punto 6.g, exige que cada `RegisterUnit` escriba un marcador en la
colección implícita del laboratorio. El dataset mínimo del protocolo crea
50.000 unidades: son exactamente 50.000 escrituras privadas adicionales y
50.000 hashed write sets públicos. No hay contador compartido y, por lo tanto,
el presupuesto no supone contención MVCC entre altas.

### Modelo conservador

Para la variante de unidad, con GTIN de 14 caracteres, serie máxima de 20 y
`txId` hexadecimal de 64:

- clave compuesta máxima: 123 bytes;
- valor JSON presupuestado: 128 bytes para operación, MSP y timestamp;
- registro público hasheado: 128 bytes presupuestados, incluidos hashes SHA-256
  de clave/valor y estructura serializada;
- copias del ledger público: siete peers y tres orderers;
- factor conservador de 4 para protobuf, metadatos de versión, índices LevelDB,
  archivos de bloques y amplificación de almacenamiento.

Cálculo para 50.000 altas:

| Concepto lógico | Cálculo | Resultado aproximado |
|---|---:|---:|
| Privado en el peer dueño | 50.000 × (123 + 128) B | 12,0 MiB |
| Hash público en 10 ledgers | 50.000 × 128 B × 10 | 61,0 MiB |
| Subtotal lógico | 12,0 + 61,0 MiB | 73,0 MiB |
| Presupuesto con factor ×4 | 73,0 × 4 | 292 MiB |
| Reserva operativa redondeada | — | **512 MiB** |

La reserva de 512 MiB es un techo de ingeniería para el conjunto de la red, no
una medición. Se agrega al presupuesto de disco, no al límite residente de
memoria. Para `M` marcadores se escala linealmente:

```text
markerDiskBudgetMiB = ceil(M / 50000) * 512
```

Los eventos regulatorios y las intervenciones agregan marcadores con el mismo
orden de costo. El protocolo debe reportar el número real por clase y el delta
empírico de almacenamiento, sin mezclar la preparación de las 50.000 unidades
con las rondas de rendimiento. La medición posterior reemplaza esta estimación,
pero nunca elimina el costo del reporte.

El límite de 384 MiB por peer es provisional hasta medir la carga representativa.
El protocolo de DES-7 debe incluir como punto de control una corrida con las
50.000 unidades cargadas y comprobar el pico residente, los reinicios y
`OOMKilled`. Si un peer alcanza el límite, la corrida se considera inválida y
el mismo límite debe revisarse de forma simétrica para ambos hosts.

## Criterio para ambos hosts

Antes de una corrida comparativa, en ambos equipos:

- copiar los valores de `wslconfig.example` a `%UserProfile%/.wslconfig`;
- ejecutar `wsl --shutdown` para aplicar el cambio;
- verificar 8 GiB, 6 CPU y 4 GiB de swap dentro de WSL;
- ejecutar el script de medición y adjuntar su salida a la metadata;
- abortar si algún contenedor no está `healthy`, usa swap sostenidamente,
  alcanza su límite o difiere el inventario de once servicios.
