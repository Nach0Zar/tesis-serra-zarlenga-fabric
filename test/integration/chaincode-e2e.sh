#!/usr/bin/env bash
#
# Smoke test de despliegue del chaincode `snt` sobre la TEST-NETWORK de
# fabric-samples. Cubre los dos ultimos criterios de aceptacion de CC-1 (#14):
# "despliega sobre la TEST-NETWORK" e "invocacion dummy responde".
#
# ------------------------------------------------------------------------------
# Que puede probarse aca, y por que no mas que eso
# ------------------------------------------------------------------------------
#
# El chaincode se despliega con --init-required (ADR-007, punto 5.c), de modo
# que la PRIMERA -- y, mientras Init no tenga exito, la UNICA -- transaccion que
# el peer admite es `Init`. Y `Init` no acepta el mspId regulatorio como
# argumento: lo resuelve contra el manifiesto fundacional EMBEBIDO en el paquete
# (ADR-010, punto 4), que declara `AnmatMSP` como REGULATOR.
#
# La test-network de fabric-samples tiene Org1MSP y Org2MSP. Ninguna es
# AnmatMSP. Por lo tanto, sobre la test-network estandar y con el artefacto
# REAL -- sin manifiestos adulterados para CI, que romperian la propiedad de
# "un unico artefacto construido una sola vez" de ADR-008 punto 5 -- la unica
# respuesta posible del chaincode es el rechazo tipificado REGULATORY_ONLY.
#
# Ese rechazo NO es una falla del smoke test: es la evidencia buscada. Para
# devolverlo, el chaincode tuvo que empaquetarse, instalarse, arrancar su
# contenedor, ejecutar Go, resolver el manifiesto embebido por go:embed,
# resolver cid.GetMSPID() y serializar un error del catalogo de
# docs/api-contract.md. Un chaincode que no desplegara, o que desplegara roto,
# devolveria un error de plataforma, no un objeto JSON con `code`.
#
# El Init EXITOSO -- y con el, el seed del registro y las operaciones de
# negocio -- exige una red cuyas MSP sean las del manifiesto fundacional. Esa
# red la construye NET-4 (#23) con `snt-channel`; el escenario funcional
# completo sobre ella pertenece a CC-3 (#16).
#
# ------------------------------------------------------------------------------
# Entorno
# ------------------------------------------------------------------------------
#
#   TEST_NETWORK_DIR   raiz de fabric-samples/test-network (obligatoria)
#   CHANNEL_NAME       canal ya creado (por defecto: mychannel)
#   CHAINCODE_NAME     nombre del chaincode en el canal (por defecto: snt)
#   CHAINCODE_VERSION  version de la definicion (por defecto: 1.0)
#   CHAINCODE_SEQUENCE secuencia de lifecycle (por defecto: 1)

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

TEST_NETWORK_DIR="${TEST_NETWORK_DIR:?TEST_NETWORK_DIR es obligatoria}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
CHAINCODE_NAME="${CHAINCODE_NAME:-snt}"
CHAINCODE_VERSION="${CHAINCODE_VERSION:-1.0}"
CHAINCODE_SEQUENCE="${CHAINCODE_SEQUENCE:-1}"
CC_LABEL="${CHAINCODE_NAME}_${CHAINCODE_VERSION}"

# La secuencia 1 del lifecycle lleva la politica estricta AND de todas las
# organizaciones fundacionales (ADR-007, punto 5.c). Sobre la test-network, las
# dos organizaciones del canal son Org1 y Org2.
ENDORSEMENT_POLICY="AND('Org1MSP.peer','Org2MSP.peer')"

PACKAGE_OUT="${REPO_ROOT}/build/chaincode/${CC_LABEL}.tar.gz"

ORDERER_CA="${TEST_NETWORK_DIR}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
ORG1_CA="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt"
ORG2_CA="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt"

export PATH="${TEST_NETWORK_DIR}/../bin:${PATH}"
export FABRIC_CFG_PATH="${TEST_NETWORK_DIR}/../config"
export CORE_PEER_TLS_ENABLED=true

log() { printf '\n=== %s\n' "$1"; }
fail() { printf '\nFALLO: %s\n' "$1" >&2; exit 1; }

use_org() {
  case "$1" in
    1)
      export CORE_PEER_LOCALMSPID=Org1MSP
      export CORE_PEER_TLS_ROOTCERT_FILE="${ORG1_CA}"
      export CORE_PEER_MSPCONFIGPATH="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp"
      export CORE_PEER_ADDRESS=localhost:7051
      ;;
    2)
      export CORE_PEER_LOCALMSPID=Org2MSP
      export CORE_PEER_TLS_ROOTCERT_FILE="${ORG2_CA}"
      export CORE_PEER_MSPCONFIGPATH="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp"
      export CORE_PEER_ADDRESS=localhost:9051
      ;;
    *) fail "organizacion desconocida: $1" ;;
  esac
}

# ------------------------------------------------------------------------------
# 1. Empaquetado reproducible (ADR-008 punto 5, ADR-010 punto 4)
# ------------------------------------------------------------------------------
#
# `make package-reproducible` empaqueta dos veces y falla si el packageID
# cambia. Es la comprobacion mecanica de la propiedad sobre la que se apoya el
# bootstrap: un artefacto construido una sola vez, distribuido identico y
# verificable por checksum.
log "Empaquetado reproducible del chaincode"
make -C "${REPO_ROOT}/chaincode" package-reproducible

test -f "${PACKAGE_OUT}" || fail "no se genero ${PACKAGE_OUT}"

PACKAGE_ID="$(peer lifecycle chaincode calculatepackageid "${PACKAGE_OUT}")"
printf 'packageID: %s\n' "${PACKAGE_ID}"

# El artefacto no puede quedar dentro del arbol que `--path` empaqueta.
if find "${REPO_ROOT}/chaincode" -maxdepth 1 -name '*.tar.gz' | grep -q .; then
  fail "se genero un tar.gz dentro de chaincode/: el empaquetado dejaria de ser reproducible"
fi

# ------------------------------------------------------------------------------
# 2. Instalacion en los peers de ambas organizaciones
# ------------------------------------------------------------------------------
for org in 1 2; do
  log "Instalando el paquete en el peer de Org${org}"
  use_org "${org}"
  peer lifecycle chaincode install "${PACKAGE_OUT}"

  peer lifecycle chaincode queryinstalled --output json \
    | grep -qF "${PACKAGE_ID}" \
    || fail "el packageID ${PACKAGE_ID} no figura instalado en el peer de Org${org}"
done

# ------------------------------------------------------------------------------
# 3. Aprobacion y commit de la definicion CON --init-required
# ------------------------------------------------------------------------------
for org in 1 2; do
  log "approveformyorg de Org${org} (--init-required)"
  use_org "${org}"
  peer lifecycle chaincode approveformyorg \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --tls --cafile "${ORDERER_CA}" \
    --channelID "${CHANNEL_NAME}" \
    --name "${CHAINCODE_NAME}" \
    --version "${CHAINCODE_VERSION}" \
    --package-id "${PACKAGE_ID}" \
    --sequence "${CHAINCODE_SEQUENCE}" \
    --signature-policy "${ENDORSEMENT_POLICY}" \
    --init-required
done

log "checkcommitreadiness"
use_org 1
peer lifecycle chaincode checkcommitreadiness \
  --channelID "${CHANNEL_NAME}" \
  --name "${CHAINCODE_NAME}" \
  --version "${CHAINCODE_VERSION}" \
  --sequence "${CHAINCODE_SEQUENCE}" \
  --signature-policy "${ENDORSEMENT_POLICY}" \
  --init-required \
  --output json

log "commit de la definicion"
peer lifecycle chaincode commit \
  -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.example.com \
  --tls --cafile "${ORDERER_CA}" \
  --channelID "${CHANNEL_NAME}" \
  --name "${CHAINCODE_NAME}" \
  --version "${CHAINCODE_VERSION}" \
  --sequence "${CHAINCODE_SEQUENCE}" \
  --signature-policy "${ENDORSEMENT_POLICY}" \
  --init-required \
  --peerAddresses localhost:7051 --tlsRootCertFiles "${ORG1_CA}" \
  --peerAddresses localhost:9051 --tlsRootCertFiles "${ORG2_CA}"

log "querycommitted"
COMMITTED="$(peer lifecycle chaincode querycommitted \
  --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --output json)"
printf '%s\n' "${COMMITTED}"

printf '%s' "${COMMITTED}" | grep -q '"init_required": *true' \
  || fail "la definicion confirmada no exige inicializacion (--init-required)"

# ------------------------------------------------------------------------------
# 4. --init-required esta efectivamente vigente
# ------------------------------------------------------------------------------
#
# Antes de que Init tenga exito, la plataforma debe rechazar cualquier otra
# funcion. Es lo que distingue un despliegue con --init-required de uno sin el.
log "Una funcion distinta de Init se rechaza antes de inicializar"
set +e
NOT_INITIALIZED_OUTPUT="$(peer chaincode query \
  -C "${CHANNEL_NAME}" -n "${CHAINCODE_NAME}" \
  -c '{"function":"ReadUnit","Args":["07791234567898","SN-0001-ABCD"]}' 2>&1)"
NOT_INITIALIZED_STATUS=$?
set -e
printf '%s\n' "${NOT_INITIALIZED_OUTPUT}"

[ "${NOT_INITIALIZED_STATUS}" -ne 0 ] \
  || fail "el peer acepto una invocacion previa a Init pese a --init-required"

# La redaccion exacta del rechazo cambio entre versiones de Fabric; se aceptan
# las tres formas conocidas para no atar el smoke test a un texto de la
# plataforma. Lo que se comprueba es que el rechazo sea POR falta de
# inicializacion y no por otra causa.
printf '%s' "${NOT_INITIALIZED_OUTPUT}" \
  | grep -Eqi 'must call as init first|requires initialization|has not been initialized' \
  || fail "el rechazo previo a Init no fue el de --init-required"

# ------------------------------------------------------------------------------
# 5. Invocacion dummy: Init responde con un error del contrato
# ------------------------------------------------------------------------------
#
# Init se invoca desde Org1MSP. El manifiesto embebido declara AnmatMSP como
# REGULATOR, de modo que la respuesta esperada es el codigo REGULATORY_ONLY del
# catalogo de docs/api-contract.md. Ver el encabezado de este archivo.
log "Invocacion dummy: Init (--isInit) desde Org1MSP"
use_org 1
set +e
INIT_OUTPUT="$(peer chaincode invoke \
  -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.example.com \
  --tls --cafile "${ORDERER_CA}" \
  -C "${CHANNEL_NAME}" -n "${CHAINCODE_NAME}" \
  --isInit \
  -c '{"function":"Init","Args":[]}' \
  --peerAddresses localhost:7051 --tlsRootCertFiles "${ORG1_CA}" \
  --peerAddresses localhost:9051 --tlsRootCertFiles "${ORG2_CA}" 2>&1)"
INIT_STATUS=$?
set -e
printf '%s\n' "${INIT_OUTPUT}"

if [ "${INIT_STATUS}" -eq 0 ]; then
  fail "Init tuvo exito desde Org1MSP: el chaincode acepto un regulador que el manifiesto fundacional no declara"
fi

# Un error de plataforma (contenedor caido, paquete corrupto, builder sin
# toolchain) nunca lleva un codigo del catalogo del contrato. Exigirlo separa el
# rechazo por logica del rechazo por infraestructura.
printf '%s' "${INIT_OUTPUT}" | grep -q 'REGULATORY_ONLY' \
  || fail "Init no devolvio el error tipificado REGULATORY_ONLY del contrato"

log "Smoke test de CC-1 (#14) OK"
printf '%s\n' \
  "  - el paquete se construye con packageID reproducible: ${PACKAGE_ID}" \
  "  - la definicion quedo confirmada en ${CHANNEL_NAME} con --init-required" \
  "  - la plataforma rechaza toda funcion previa a Init" \
  "  - la invocacion dummy de Init responde REGULATORY_ONLY (manifiesto embebido: AnmatMSP)"
