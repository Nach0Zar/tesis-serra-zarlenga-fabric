# Red Fabric del prototipo

Este directorio contiene la configuración de la red Hyperledger Fabric del prototipo conforme a [ADR-007](../docs/adr/007-network-topology.md). La interfaz de topología permanece definida por `configtx.yaml`: un canal `snt-channel`, siete MSP, un peer por organización y tres nodos Raft aportados por ANMAT, laboratorio y droguería.

## Inventario de organizaciones

`organizations-manifest.json` es el inventario versionado compartido por la configuración criptográfica y los futuros consumidores de privacidad y bootstrap, según [ADR-010](../docs/adr/010-non-custodial-identity.md). `organizations-manifest.schema.json` usa JSON Schema Draft 2020-12 y cierra el formato con `additionalProperties: false`.

| MSP | Slug | Identidad | Tipo de agente | Rol del usuario | Orderer |
|---|---|---|---|---|---|
| `AnmatMSP` | `anmat` | `REG:ANMAT` | `REGULATOR` | `regulatory-admin` | sí |
| `LabMSP` | `lab` | `GLN:7791234500017` | `LABORATORY` | `operator` | sí |
| `DrogueriaMSP` | `drogueria` | `GLN:7791234500024` | `DRUGSTORE` | `operator` | sí |
| `DistribuidorMSP` | `distribuidor` | `GLN:7791234500031` | `DISTRIBUTOR` | `operator` | no |
| `FarmaciaMSP` | `farmacia` | `GLN:7791234500048` | `PHARMACY` | `operator` | no |
| `CentroMedicoMSP` | `centromedico` | `GLN:7791234500055` | `HEALTHCARE_FACILITY` | `operator` | no |
| `FinanciadorMSP` | `financiador` | `REG:INSSJP-PAMI` | `FINANCIER` | `financier-auditor` | no |

El validador aplica el schema y además comprueba unicidad de MSP, slugs, identidades y hosts; compatibilidad de catálogos; checksum de los GLN; un único regulador; y exactamente los tres orderers de ADR-007. También contrasta MSP, paths, peers, endpoints y consenters contra `configtx.yaml`.

## Requisitos

La generación requiere:

- Bash, `jq`, OpenSSL, Python 3, GNU coreutils y utilidades POSIX usadas por los scripts;
- los módulos Python `jsonschema` con soporte Draft 2020-12 y `PyYAML`;
- `fabric-ca-server` y `fabric-ca-client` versión exacta `1.5.17`.

Para las verificaciones de desarrollo también se usan `shellcheck` y `configtxgen`. El bloque de canal fue validado con Fabric `2.5.16`.

## Uso

Desde la raíz del repositorio:

```bash
python3 network/scripts/validate-organizations-manifest.py
network/scripts/generate-crypto.sh
network/scripts/verify-crypto.sh
```

El generador valida todas las entradas antes de crear archivos, comprueba que el puerto local `7054` esté libre, inicia temporalmente Fabric CA con TLS y lo detiene mediante `trap`. El puerto puede cambiarse sin alterar la topología de peers y orderers:

```bash
SNT_CA_PORT=17054 network/scripts/generate-crypto.sh
```

Para inspeccionar la configuración consumiendo el material generado:

```bash
cd network
FABRIC_CFG_PATH="$PWD" configtxgen \
  -profile SNTChannelGenesis \
  -channelID snt-channel \
  -outputBlock /tmp/snt-channel.block
configtxgen -inspectBlock /tmp/snt-channel.block
```

## Material generado

Todo el resultado queda debajo de `network/organizations/`, que está ignorado por Git:

```text
organizations/
├── .fabric-ca-server/                  # configuraciones, raíces y SQLite de las CA
├── .state/
│   ├── manifest.sha256
│   ├── secrets.env                    # modo 0600
│   └── logs/
└── <slug>/
    ├── msp/
    ├── peers/peer0.<slug>.snt.local/
    │   ├── msp/
    │   └── tls/
    ├── orderers/orderer.<slug>.snt.local/  # solo ANMAT, lab y droguería
    │   ├── msp/
    │   └── tls/
    └── users/
        ├── Admin@<slug>.snt.local/msp/
        └── User1@<slug>.snt.local/msp/
```

Cada MSP habilita NodeOUs para `client`, `peer`, `admin` y `orderer`. Solamente el ECert de `User1` incluye el atributo `snt.role` indicado por el manifiesto; los certificados de peer, administrador y orderer no lo incluyen. Los certificados TLS contienen el SAN del hostname que consume `configtx.yaml`.

El campo `clientRole` describe el rol de esa única identidad `User1` exigida por NET-1; no es el catálogo exhaustivo de identidades internas de DES-6. En particular, la identidad de solo lectura `auditor` de la organización regulatoria no se emite en este issue: incorporarla requiere ampliar el manifiesto a múltiples usuarios por organización y queda diferida a la configuración de autorización e integración que la consuma.

## Idempotencia y secretos

En la primera ejecución se generan secretos aleatorios con OpenSSL y se guardan sin imprimir en `.state/secrets.env`, con permisos `0600`. No se deben copiar, mostrar ni incorporar al repositorio las claves, certificados, bases SQLite, logs o secretos de `network/organizations/`.

`.state/manifest.sha256` vincula el material emitido con el manifiesto exacto. Una nueva ejecución con el mismo checksum reutiliza registros y certificados existentes. Si cambia el manifiesto, el script termina con un error y no elimina ni regenera material automáticamente; el operador debe preservar o retirar explícitamente el directorio anterior antes de generar otro conjunto.

## Modelo operativo y límite de confianza

El proceso implementa el mecanismo `cafiles` soportado por Fabric CA: un único proceso hospeda siete CA lógicas, cada una con nombre, raíz criptográfica y base SQLite independientes. Esto reduce componentes para el prototipo, pero concentra disponibilidad, operación del proceso y acceso al host. No representa el aislamiento administrativo que tendría una CA desplegada y operada por cada organización.

## Fuera de alcance

NET-1 no define Docker Compose, puertos definitivos de peers u orderers, Private Data Collections, lifecycle de chaincode, bootstrap del ledger ni políticas de endoso. Esas responsabilidades permanecen en NET-3 a NET-6 y en sus ADR correspondientes.
