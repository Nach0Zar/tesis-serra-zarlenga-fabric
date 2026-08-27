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

El canal definitivo es `snt-channel` y el chaincode es `snt`. La creación del
canal y el lifecycle no forman parte de este Compose.

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

## Validación y salud

La validación estática no requiere que el daemon esté iniciado:

```bash
docker compose -f network/compose.yaml config --quiet
```

Cuando NET-1 esté integrada y Docker disponible:

```bash
docker compose -f network/compose.yaml up --detach --wait --wait-timeout 180
docker compose -f network/compose.yaml ps
./network/scripts/measure-resources.sh
```

`up --wait` falla si alguno de los once contenedores no alcanza `healthy`.
Los peers ejecutan `peer node status`; los orderers consultan `/healthz` del
endpoint de operaciones; Fabric CA ejecuta `getcainfo` contra `ca-anmat`.

NET-4 es responsable de los wrappers idempotentes `up/down/restart`, de crear
`snt-channel`, unir los nodos y ejecutar las dos secuencias de lifecycle.

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

Este cambio no genera identidades (NET-1), no modifica `configtx.yaml` (NET-2),
no crea canal ni despliega chaincode (NET-4), no genera
`collections_config.json` (NET-5) y no implementa ni prueba políticas de
endoso (NET-6).
