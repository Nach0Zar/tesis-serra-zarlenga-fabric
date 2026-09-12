# Docker Compose de la red

[`compose.yaml`](compose.yaml) materializa exclusivamente NET-3 conforme
[ADR-007](../docs/adr/007-network-topology.md):

- siete peers, uno por organización del dataset mínimo;
- LevelDB embebida en todos los peers;
- tres orderers Raft aportados por `AnmatMSP`, `LabMSP` y
  `DrogueriaMSP`;
- un proceso Fabric CA con siete CA lógicas y raíces separadas;
- TLS en todos los endpoints peer y orderer;
- healthchecks y límites de recursos reproducibles.

El canal definitivo es `snt-channel` y el chaincode es `snt`. El Compose
solo declara servicios; `network/network.sh` implementa la creación del canal
y el lifecycle de NET-4 sobre esos servicios.

## Versiones

| Componente | Imagen por defecto |
|---|---|
| Peer | `hyperledger/fabric-peer:2.5.16` |
| Orderer | `hyperledger/fabric-orderer:2.5.16` |
| Fabric CA | `hyperledger/fabric-ca:1.5.17` |

Son las mismas versiones fijadas por el workflow de integración. Solo se
reemplazan para una prueba explícita:

```bash
FABRIC_VERSION=2.5.16 FABRIC_CA_VERSION=1.5.17 \
  docker compose -f network/compose.yaml config --quiet
```

## Prerrequisito de NET-1

El Compose consume el layout definido por la rama de NET-1 bajo
`network/organizations/`; no genera ni versiona material criptográfico. Antes
de iniciar la red deben existir:

- `<slug>/peers/<peer-fqdn>/{msp,tls}` para las siete organizaciones;
- `{anmat,lab,drogueria}/orderers/<orderer-fqdn>/{msp,tls}`;
- `.fabric-ca-server/cas/<slug>/fabric-ca-server-config.yaml` para las siete
  CA lógicas;
- `.fabric-ca-server/tls/{server-tls-cert.pem,server-tls-key.pem}`.

`anmat` es la CA primaria (`ca-anmat`); las otras seis se cargan con
`--cafiles`. Cada MSP confía únicamente en la raíz de su propia CA lógica.
Usar `cryptogen` como sustituto definitivo contradice ADR-007; solo se admite
para pruebas locales descartables.

La presencia del commit de NET-1 no implica que esos archivos locales existan.
En cada checkout nuevo se generan y verifican antes de iniciar Compose:

```bash
./network/scripts/generate-crypto.sh
./network/scripts/verify-crypto.sh
```

Los bind mounts criptográficos tienen `create_host_path: false`: si falta una
ruta, Compose falla antes de crear un directorio vacío. No debe hacerse
escribible un MSP ni agregarse un bootstrap por defecto para ocultar material
ausente.

## Validación y salud

La sintaxis puede validarse sin iniciar los servicios:

```bash
docker compose -f network/compose.yaml config --quiet
```

Antes de cada arranque, el preflight comprueba Docker, los artefactos generados
por NET-1 y que Compose conserve exactamente once servicios:

```bash
./network/scripts/validate-compose-prerequisites.sh
docker compose -f network/compose.yaml up --detach --wait --wait-timeout 180
docker compose -f network/compose.yaml ps
```

Para una medición comparable entre hosts, aplicar primero
`network/wslconfig.example` como `%UserProfile%\.wslconfig`, ejecutar
`wsl --shutdown` desde PowerShell y reiniciar Docker Desktop. La medición exige
el perfil efectivo de 6 CPU, 8 GiB de memoria y 4 GiB de swap:

```bash
./network/scripts/validate-compose-prerequisites.sh --measurement
./network/scripts/measure-resources.sh
```

El margen de memoria del preflight contempla la memoria reservada por WSL2:
acepta entre 7,5 y 8 GiB visibles para una configuración nominal de 8 GiB. El
archivo `.wslconfig` pertenece al perfil de Windows, no a la raíz del checkout,
y está ignorado allí para evitar que una copia local se versione por error.

El modo de medición también exige que los archivos que gobiernan la corrida
estén versionados y sin cambios staged o unstaged. Así, el commit informado
identifica efectivamente la configuración ejecutada en ambos hosts.

`up --wait` falla si alguno de los once contenedores no alcanza `healthy`.
Los peers y orderers consultan `/healthz` en sus endpoints de operaciones.
Fabric CA ejecuta `getcainfo` para las siete CA lógicas configuradas. El script
de medición también registra `RestartCount` y `OOMKilled`, e invalida la
corrida si detecta un reinicio o una terminación por falta de memoria.

Antes de crear y unir un canal, `healthy` sólo demuestra que los procesos y sus
endpoints de operaciones responden. No demuestra que el consenter set de Raft
haya formado quorum ni elegido líder.

`network/network.sh createChannel` verifica que los tres orderers sean
consenters activos y que el ordering service entregue bloques, además de
comprobar la membresía de los siete peers. `deployCC` ejecuta las dos
secuencias de lifecycle y conserva la evidencia cruda bajo `build/evidence/`.

Los endpoints administrativos de los orderers exigen mTLS y cada uno confía en
la CA TLS de su propia organización. Por lo tanto, NET-4 no debe asumir una
identidad TLS administrativa compartida: las invocaciones de `osnadmin` deben
usar, para cada orderer, una identidad cliente aceptada por su raíz configurada.

## Puertos publicados

Todos los puertos se enlazan a `127.0.0.1`. Los peers y orderers conservan
internamente `7051` y `7050`, respectivamente, para coincidir con
[`configtx.yaml`](configtx.yaml). Los endpoints de chaincode `7052` no se
publican.

| Contenedor | Organización | gRPC/API | Admin | Operaciones |
|---|---|---:|---:|---:|
| `fabric-ca.snt.local` | todas las CA lógicas | 7054 | — | — |
| `orderer.anmat.snt.local` | `AnmatMSP` | 7050 | 7053 | 9440 |
| `orderer.lab.snt.local` | `LabMSP` | 8050 | 8053 | 9441 |
| `orderer.drogueria.snt.local` | `DrogueriaMSP` | 9050 | 9053 | 9442 |
| `peer0.anmat.snt.local` | `AnmatMSP` | 7051 | — | 9444 |
| `peer0.lab.snt.local` | `LabMSP` | 8051 | — | 9445 |
| `peer0.drogueria.snt.local` | `DrogueriaMSP` | 9051 | — | 9446 |
| `peer0.distribuidor.snt.local` | `DistribuidorMSP` | 10051 | — | 9447 |
| `peer0.farmacia.snt.local` | `FarmaciaMSP` | 11051 | — | 9448 |
| `peer0.centromedico.snt.local` | `CentroMedicoMSP` | 12051 | — | 9449 |
| `peer0.financiador.snt.local` | `FinanciadorMSP` | 13051 | — | 9450 |

No hay puertos host repetidos. Los nombres de los tres orderers y siete peers
coinciden con los endpoints de `configtx.yaml`.

## Recursos y seguridad

[`resource-budget.md`](resource-budget.md) documenta el techo homogéneo de
WSL2 y el costo de los 50.000 marcadores de participación. La plantilla
[`wslconfig.example`](wslconfig.example) fija 8 GiB, 6 procesadores y 4 GiB
de swap para ambos hosts.

Cada peer monta el socket Docker porque el runtime estándar de Fabric 2.5 crea
los contenedores de chaincode. Ese acceso equivale a control del daemon y solo
es aceptable en este prototipo local; no es una recomendación productiva.

El Compose no genera identidades, no modifica `configtx.yaml`, no crea el
canal y no despliega chaincode por sí solo. Esas operaciones se mantienen en
scripts separados para conservar idempotencia y evidencia. La validación
exhaustiva de SBE del chaincode productivo permanece en NET-6.
