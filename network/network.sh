#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly NETWORK_DIR="${SCRIPT_DIR}"
REPOSITORY_ROOT="$(cd -- "${NETWORK_DIR}/.." && pwd)"
readonly REPOSITORY_ROOT
readonly COMPOSE_FILE="${SNT_COMPOSE_FILE:-${NETWORK_DIR}/compose.yaml}"
readonly MANIFEST="${SNT_ORGANIZATIONS_MANIFEST:-${NETWORK_DIR}/organizations-manifest.json}"
readonly MATRIX="${SNT_AUTHORIZED_TRANSFERS:-${REPOSITORY_ROOT}/domain/authorized-transfers.json}"
readonly COLLECTIONS_CONFIG="${SNT_COLLECTIONS_CONFIG:-${NETWORK_DIR}/collections_config.json}"
readonly DEFINITION_VERIFIER="${NETWORK_DIR}/scripts/verify-committed-definition.py"
readonly PACKAGE_LOCK="${SNT_PACKAGE_LOCK:-${NETWORK_DIR}/chaincode-package.lock}"
readonly CHANNEL_NAME="${SNT_CHANNEL_NAME:-snt-channel}"
readonly CHAINCODE_NAME="${SNT_CHAINCODE_NAME:-snt}"
readonly CHAINCODE_VERSION="${SNT_CHAINCODE_VERSION:-1.0}"
readonly CHAINCODE_LABEL="${SNT_CHAINCODE_LABEL:-${CHAINCODE_NAME}_${CHAINCODE_VERSION}}"
readonly BUILD_DIR="${SNT_NETWORK_BUILD_DIR:-${REPOSITORY_ROOT}/build/network}"
readonly EVIDENCE_DIR="${SNT_EVIDENCE_DIR:-${REPOSITORY_ROOT}/build/evidence}"
readonly CHANNEL_BLOCK="${BUILD_DIR}/${CHANNEL_NAME}.block"
readonly PACKAGE_FILE="${REPOSITORY_ROOT}/build/chaincode/${CHAINCODE_LABEL}.tar.gz"
readonly ORGANIZATIONS_DIR="${NETWORK_DIR}/organizations"

FABRIC_CLIENT_CFG_PATH=""
PRIMARY_ORDERER_HOST=""
PRIMARY_ORDERER_ENDPOINT=""
PRIMARY_ORDERER_CA=""
declare -a ORDERER_ARGS=()
declare -a ALL_PEER_TARGETS=()
declare -a APPROVAL_FLAGS=()

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '\n==> %s\n' "$*"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

compose() {
  docker compose -f "${COMPOSE_FILE}" "$@"
}

published_endpoint() {
  local service="$1"
  local container_port="$2"
  local endpoint
  endpoint="$(compose port "${service}" "${container_port}" 2>/dev/null | tail -n 1)"
  [[ -n "${endpoint}" ]] || fail "no published endpoint for ${service}:${container_port}"
  printf '%s\n' "${endpoint}"
}

organization_rows() {
  jq -r '
    .organizations[]
    | [
        .mspId,
        .slug,
        .peerHostname,
        .agentType,
        .id,
        .idType,
        (.active | tostring),
        (.ordererHostname // "")
      ]
    | @tsv
  ' "${MANIFEST}"
}

regulator_row() {
  jq -r '
    .organizations[]
    | select(.agentType == "REGULATOR")
    | [
        .mspId,
        .slug,
        .peerHostname,
        .agentType,
        .id,
        .idType,
        (.active | tostring),
        (.ordererHostname // "")
      ]
    | @tsv
  ' "${MANIFEST}"
}

resolve_fabric_environment() {
  local sibling_config
  if [[ -n "${SNT_FABRIC_BIN_DIR:-}" ]]; then
    export PATH="${SNT_FABRIC_BIN_DIR}:${PATH}"
  fi
  require_command peer
  require_command osnadmin
  require_command configtxgen
  require_command configtxlator
  require_command base64
  if [[ -n "${SNT_FABRIC_CFG_PATH:-}" ]]; then
    FABRIC_CLIENT_CFG_PATH="${SNT_FABRIC_CFG_PATH}"
  elif [[ -n "${FABRIC_CFG_PATH:-}" && -f "${FABRIC_CFG_PATH}/core.yaml" ]]; then
    FABRIC_CLIENT_CFG_PATH="${FABRIC_CFG_PATH}"
  else
    sibling_config=""
    if sibling_config="$(cd -- "$(dirname -- "$(command -v peer)")/../config" 2>/dev/null && pwd)"; then
      :
    fi
    [[ -f "${sibling_config}/core.yaml" ]] || fail "core.yaml not found; set SNT_FABRIC_CFG_PATH"
    FABRIC_CLIENT_CFG_PATH="${sibling_config}"
  fi
  [[ -f "${FABRIC_CLIENT_CFG_PATH}/core.yaml" ]] || fail "missing ${FABRIC_CLIENT_CFG_PATH}/core.yaml"
  export FABRIC_CFG_PATH="${FABRIC_CLIENT_CFG_PATH}"
  export CORE_PEER_TLS_ENABLED=true
}

validate_sources() {
  require_command python3
  require_command jq
  python3 "${NETWORK_DIR}/scripts/validate-organizations-manifest.py"
  python3 "${NETWORK_DIR}/scripts/generate-collections.py" --check
}

ensure_network_running() {
  local running service
  running="$(compose ps --status running --services)"
  while IFS= read -r service; do
    grep -Fqx -- "${service}" <<<"${running}" || fail "network service ${service} is not running; execute ./network/network.sh up"
  done < <(jq -r '.organizations[].peerHostname, (.organizations[].ordererHostname // empty)' "${MANIFEST}")
}

use_organization() {
  local msp_id="$1"
  local slug="$2"
  local peer_hostname="$3"
  local identity="${4:-Admin}"
  local identity_dir="${ORGANIZATIONS_DIR}/${slug}/users/${identity}@${slug}.snt.local/msp"
  local peer_tls="${ORGANIZATIONS_DIR}/${slug}/peers/${peer_hostname}/tls/ca.crt"
  [[ -d "${identity_dir}" ]] || fail "missing identity MSP: ${identity_dir}"
  [[ -f "${peer_tls}" ]] || fail "missing peer TLS root: ${peer_tls}"
  export CORE_PEER_LOCALMSPID="${msp_id}"
  export CORE_PEER_MSPCONFIGPATH="${identity_dir}"
  export CORE_PEER_TLS_ROOTCERT_FILE="${peer_tls}"
  export CORE_PEER_ADDRESS
  CORE_PEER_ADDRESS="$(published_endpoint "${peer_hostname}" 7051)"
}

set_primary_orderer() {
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(regulator_row)
  [[ -n "${orderer_hostname}" ]] || fail "regulator has no orderer"
  PRIMARY_ORDERER_HOST="${orderer_hostname}"
  PRIMARY_ORDERER_ENDPOINT="$(published_endpoint "${orderer_hostname}" 7050)"
  PRIMARY_ORDERER_CA="${ORGANIZATIONS_DIR}/${slug}/orderers/${orderer_hostname}/tls/ca.crt"
  ORDERER_ARGS=(
    --orderer "${PRIMARY_ORDERER_ENDPOINT}"
    --ordererTLSHostnameOverride "${PRIMARY_ORDERER_HOST}"
    --tls
    --cafile "${PRIMARY_ORDERER_CA}"
  )
}

build_all_peer_targets() {
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  ALL_PEER_TARGETS=()
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    ALL_PEER_TARGETS+=(
      --peerAddresses "$(published_endpoint "${peer_hostname}" 7051)"
      --tlsRootCertFiles "${ORGANIZATIONS_DIR}/${slug}/peers/${peer_hostname}/tls/ca.crt"
    )
  done < <(organization_rows)
}

all_organizations_policy() {
  jq -r '"AND(" + ([.organizations[].mspId | "\u0027" + . + ".peer\u0027"] | join(",")) + ")"' "${MANIFEST}"
}

operational_policy() {
  jq -nr --slurpfile manifest "${MANIFEST}" --slurpfile matrix "${MATRIX}" '
    ($matrix[0].agentTypes | map(.code)) as $custodial
    | ($manifest[0].organizations
        | map(select(.agentType == "REGULATOR" or (.agentType as $type | $custodial | index($type))))
        | map(.mspId)
        | sort) as $msps
    | "OR(" + ($msps | map("\u0027" + . + ".peer\u0027") | join(",")) + ")"
  '
}

up_network() {
  require_command docker
  validate_sources
  "${NETWORK_DIR}/scripts/validate-compose-prerequisites.sh"
  info "Starting the eleven Fabric services"
  compose up --detach --wait --wait-timeout "${SNT_START_TIMEOUT:-180}"
  compose ps
}

down_network() {
  require_command docker
  info "Stopping the Fabric network while preserving named volumes"
  compose down --remove-orphans
}

restart_network() {
  require_command docker
  info "Restarting the Fabric services"
  compose restart
  compose up --detach --wait --wait-timeout "${SNT_START_TIMEOUT:-180}"
}

orderer_status() {
  local endpoint="$1"
  local tls_dir="$2"
  local -a args=(
    --orderer-address "${endpoint}"
    --ca-file "${tls_dir}/ca.crt"
    --client-cert "${tls_dir}/server.crt"
    --client-key "${tls_dir}/server.key"
    --channelID "${CHANNEL_NAME}"
    --no-status
  )
  osnadmin channel list "${args[@]}"
}

orderer_is_active() {
  local output
  output="$(orderer_status "$1" "$2" 2>/dev/null)" || return 1
  jq -e --arg channel "${CHANNEL_NAME}" '.name == $channel and .status == "active" and .consensusRelation == "consenter"' <<<"${output}" >/dev/null
}

join_orderers() {
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local admin_endpoint tls_dir
  local -a args
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    [[ -n "${orderer_hostname}" ]] || continue
    admin_endpoint="$(published_endpoint "${orderer_hostname}" 7053)"
    tls_dir="${ORGANIZATIONS_DIR}/${slug}/orderers/${orderer_hostname}/tls"
    if orderer_is_active "${admin_endpoint}" "${tls_dir}"; then
      info "${orderer_hostname} already participates in ${CHANNEL_NAME}"
      continue
    fi
    args=(
      --orderer-address "${admin_endpoint}"
      --ca-file "${tls_dir}/ca.crt"
      --client-cert "${tls_dir}/server.crt"
      --client-key "${tls_dir}/server.key"
      --channelID "${CHANNEL_NAME}"
      --config-block "${CHANNEL_BLOCK}"
    )
    info "Joining ${orderer_hostname} to ${CHANNEL_NAME}"
    osnadmin channel join "${args[@]}"
  done < <(organization_rows)
}

wait_for_orderers() {
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local admin_endpoint tls_dir
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    [[ -n "${orderer_hostname}" ]] || continue
    admin_endpoint="$(published_endpoint "${orderer_hostname}" 7053)"
    tls_dir="${ORGANIZATIONS_DIR}/${slug}/orderers/${orderer_hostname}/tls"
    for _ in {1..30}; do
      orderer_is_active "${admin_endpoint}" "${tls_dir}" && break
      sleep 2
    done
    orderer_is_active "${admin_endpoint}" "${tls_dir}" || fail "${orderer_hostname} did not become an active consenter"
  done < <(organization_rows)
}

peer_has_channel() {
  peer channel list 2>/dev/null | grep -qx "${CHANNEL_NAME}"
}

join_peers() {
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
    if peer_has_channel; then
      info "${peer_hostname} already participates in ${CHANNEL_NAME}"
    else
      info "Joining ${peer_hostname} to ${CHANNEL_NAME}"
      peer channel join --blockpath "${CHANNEL_BLOCK}"
    fi
  done < <(organization_rows)
}

verify_channel() {
  local output_dir="${EVIDENCE_DIR}/net-4/channel"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local admin_endpoint tls_dir status_file
  mkdir -p "${output_dir}" "${BUILD_DIR}"
  set_primary_orderer
  info "Verifying the three active Raft consenters"
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    [[ -n "${orderer_hostname}" ]] || continue
    admin_endpoint="$(published_endpoint "${orderer_hostname}" 7053)"
    tls_dir="${ORGANIZATIONS_DIR}/${slug}/orderers/${orderer_hostname}/tls"
    status_file="${output_dir}/orderer-${slug}.json"
    orderer_status "${admin_endpoint}" "${tls_dir}" >"${status_file}"
    jq -e --arg channel "${CHANNEL_NAME}" '.name == $channel and .status == "active" and .consensusRelation == "consenter"' "${status_file}" >/dev/null || fail "invalid channel status from ${orderer_hostname}"
  done < <(organization_rows)

  info "Verifying channel membership on all seven peers"
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
    peer_has_channel || fail "${peer_hostname} is not joined to ${CHANNEL_NAME}"
    peer channel getinfo --channelID "${CHANNEL_NAME}" >"${output_dir}/peer-${slug}.txt"
  done < <(organization_rows)

  info "Fetching the newest block through the Raft ordering service"
  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(regulator_row)
  use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
  peer channel fetch newest "${BUILD_DIR}/orderer-probe.block" --channelID "${CHANNEL_NAME}" "${ORDERER_ARGS[@]}" >"${output_dir}/orderer-fetch.txt" 2>&1
  printf 'OK: 3 consenters and 7 peers participate in %s\n' "${CHANNEL_NAME}"
}

create_channel() {
  require_command docker
  resolve_fabric_environment
  validate_sources
  ensure_network_running
  mkdir -p "${BUILD_DIR}" "${EVIDENCE_DIR}/net-4/channel"
  info "Generating the application-channel genesis block"
  FABRIC_CFG_PATH="${NETWORK_DIR}" configtxgen -profile SNTChannelGenesis -channelID "${CHANNEL_NAME}" -outputBlock "${CHANNEL_BLOCK}"
  join_orderers
  wait_for_orderers
  join_peers
  verify_channel
}

LOCK_LABEL=""
LOCK_VERSION=""
LOCK_PACKAGE_ID=""
LOCK_SHA256=""

read_package_lock() {
  local key value
  local seen=0
  LOCK_LABEL=""
  LOCK_VERSION=""
  LOCK_PACKAGE_ID=""
  LOCK_SHA256=""
  [[ -f "${PACKAGE_LOCK}" ]] || fail "missing ${PACKAGE_LOCK}; run ./network/network.sh package-lock"
  while IFS='=' read -r key value; do
    [[ -n "${key}" && -n "${value}" ]] || fail "malformed package lock line"
    case "${key}" in
      label) [[ -z "${LOCK_LABEL}" ]] || fail "duplicate lock key: label"; LOCK_LABEL="${value}" ;;
      version) [[ -z "${LOCK_VERSION}" ]] || fail "duplicate lock key: version"; LOCK_VERSION="${value}" ;;
      package_id) [[ -z "${LOCK_PACKAGE_ID}" ]] || fail "duplicate lock key: package_id"; LOCK_PACKAGE_ID="${value}" ;;
      sha256) [[ -z "${LOCK_SHA256}" ]] || fail "duplicate lock key: sha256"; LOCK_SHA256="${value}" ;;
      *) fail "unknown package lock key: ${key}" ;;
    esac
    seen=$((seen + 1))
  done <"${PACKAGE_LOCK}"
  [[ "${seen}" -eq 4 ]] || fail "package lock must contain exactly four keys"
  [[ "${LOCK_LABEL}" == "${CHAINCODE_LABEL}" ]] || fail "lock label differs from ${CHAINCODE_LABEL}"
  [[ "${LOCK_VERSION}" == "${CHAINCODE_VERSION}" ]] || fail "lock version differs from ${CHAINCODE_VERSION}"
  [[ "${LOCK_SHA256}" =~ ^[0-9a-f]{64}$ ]] || fail "invalid sha256 in package lock"
  [[ "${LOCK_PACKAGE_ID}" == "${LOCK_LABEL}:${LOCK_SHA256}" ]] || fail "package_id and sha256 are inconsistent"
}

package_lock() {
  local sha256 package_id temporary
  resolve_fabric_environment
  require_command make
  require_command sha256sum
  info "Building the package twice to verify reproducibility"
  make -C "${REPOSITORY_ROOT}/chaincode" package-reproducible
  sha256="$(sha256sum "${PACKAGE_FILE}" | awk '{print $1}')"
  package_id="$(peer lifecycle chaincode calculatepackageid "${PACKAGE_FILE}")"
  [[ "${package_id}" == "${CHAINCODE_LABEL}:${sha256}" ]] || fail "packageID is inconsistent with artifact sha256"
  mkdir -p "${BUILD_DIR}"
  temporary="${BUILD_DIR}/chaincode-package.lock"
  printf 'label=%s\nversion=%s\npackage_id=%s\nsha256=%s\n' "${CHAINCODE_LABEL}" "${CHAINCODE_VERSION}" "${package_id}" "${sha256}" >"${temporary}"
  mv -- "${temporary}" "${PACKAGE_LOCK}"
  printf 'OK: updated %s with %s\n' "${PACKAGE_LOCK}" "${package_id}"
}

build_and_verify_package() {
  local actual_sha actual_package_id
  read_package_lock
  require_command make
  require_command sha256sum
  info "Building the chaincode package once for distribution"
  make -C "${REPOSITORY_ROOT}/chaincode" package
  actual_sha="$(sha256sum "${PACKAGE_FILE}" | awk '{print $1}')"
  actual_package_id="$(peer lifecycle chaincode calculatepackageid "${PACKAGE_FILE}")"
  [[ "${actual_sha}" == "${LOCK_SHA256}" ]] || fail "package sha256 differs from lock"
  [[ "${actual_package_id}" == "${LOCK_PACKAGE_ID}" ]] || fail "packageID differs from lock"
}

install_package_everywhere() {
  local evidence="${EVIDENCE_DIR}/net-4/lifecycle"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local installed
  mkdir -p "${evidence}"
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
    installed="$(peer lifecycle chaincode queryinstalled --output json)"
    if jq -e --arg package "${LOCK_PACKAGE_ID}" 'any(.installed_chaincodes[]?; .package_id == $package)' <<<"${installed}" >/dev/null; then
      info "${CHAINCODE_LABEL} is already installed on ${peer_hostname}"
    else
      info "Installing ${CHAINCODE_LABEL} on ${peer_hostname}"
      peer lifecycle chaincode install "${PACKAGE_FILE}"
      installed="$(peer lifecycle chaincode queryinstalled --output json)"
    fi
    printf '%s\n' "${installed}" >"${evidence}/queryinstalled-${slug}.json"
    jq -e --arg package "${LOCK_PACKAGE_ID}" 'any(.installed_chaincodes[]?; .package_id == $package)' <<<"${installed}" >/dev/null || fail "${LOCK_PACKAGE_ID} is not installed on ${peer_hostname}"
  done < <(organization_rows)
}

approval_flags() {
  local sequence="$1"
  local policy="$2"
  local init_required="$3"
  APPROVAL_FLAGS=(
    --channelID "${CHANNEL_NAME}"
    --name "${CHAINCODE_NAME}"
    --version "${CHAINCODE_VERSION}"
    --sequence "${sequence}"
    --signature-policy "${policy}"
    --collections-config "${COLLECTIONS_CONFIG}"
  )
  [[ "${init_required}" == "true" ]] && APPROVAL_FLAGS+=(--init-required)
  return 0
}

approval_matches() {
  local document="$1"
  local sequence="$2"
  local init_required="$3"
  local policy_kind="$4"
  local artifact="$5"
  document_package_id_matches "${document}" "${LOCK_PACKAGE_ID}" \
    && definition_matches "${document}" "${sequence}" "${init_required}" "${policy_kind}" "${artifact}"
}

document_package_id_matches() {
  local document="$1"
  local expected_package_id="$2"
  jq -e --arg package "${expected_package_id}" '
      (.package_id // "") == $package
      or (.source.local_package.package_id // "") == $package
      or (.source.LocalPackage.package_id // "") == $package
      or (.source.Type.LocalPackage.package_id // "") == $package
  ' <<<"${document}" >/dev/null
}

definition_matches() {
  local document="$1"
  local sequence="$2"
  local init_required="$3"
  local policy_kind="$4"
  local artifact="$5"
  local evidence="${EVIDENCE_DIR}/net-4/lifecycle/definition-checks"
  local definition_file="${evidence}/${artifact}.json"
  local policy_proto="${evidence}/${artifact}-policy.pb"
  local policy_json="${evidence}/${artifact}-policy.json"
  local validation_parameter
  mkdir -p "${evidence}"
  printf '%s\n' "${document}" >"${definition_file}"
  validation_parameter="$(jq -er '.validation_parameter | select(type == "string" and length > 0)' <<<"${document}")" || {
    printf 'ERROR: lifecycle definition has no validation parameter\n' >&2
    return 1
  }
  printf '%s' "${validation_parameter}" | base64 --decode >"${policy_proto}" || return 1
  configtxlator proto_decode --input "${policy_proto}" --type common.ApplicationPolicy --output "${policy_json}" || return 1
  python3 "${DEFINITION_VERIFIER}" \
    --definition "${definition_file}" \
    --decoded-policy "${policy_json}" \
    --collections "${COLLECTIONS_CONFIG}" \
    --manifest "${MANIFEST}" \
    --matrix "${MATRIX}" \
    --sequence "${sequence}" \
    --version "${CHAINCODE_VERSION}" \
    --init-required "${init_required}" \
    --policy-kind "${policy_kind}"
}

approve_sequence() {
  local sequence="$1"
  local policy="$2"
  local init_required="$3"
  local policy_kind="$4"
  local evidence="${EVIDENCE_DIR}/net-4/lifecycle"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local approved status expected_count
  approval_flags "${sequence}" "${policy}" "${init_required}"
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
    set +e
    approved="$(peer lifecycle chaincode queryapproved --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --sequence "${sequence}" --output json 2>/dev/null)"
    status=$?
    set -e
    if [[ "${status}" -eq 0 ]]; then
      approval_matches "${approved}" "${sequence}" "${init_required}" "${policy_kind}" "approved-seq${sequence}-${slug}" || fail "${msp_id} has an incompatible approval for sequence ${sequence}; use a new lifecycle sequence for changed collections or policy"
      info "${msp_id} already approved sequence ${sequence}"
    else
      info "${msp_id} approving sequence ${sequence}"
      peer lifecycle chaincode approveformyorg "${ORDERER_ARGS[@]}" "${APPROVAL_FLAGS[@]}" --package-id "${LOCK_PACKAGE_ID}"
      approved="$(peer lifecycle chaincode queryapproved --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --sequence "${sequence}" --output json)"
      approval_matches "${approved}" "${sequence}" "${init_required}" "${policy_kind}" "approved-seq${sequence}-${slug}" || fail "${msp_id} approval for sequence ${sequence} is inconsistent"
    fi
    printf '%s\n' "${approved}" >"${evidence}/queryapproved-seq${sequence}-${slug}.json"
  done < <(organization_rows)

  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(organization_rows)
  use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
  peer lifecycle chaincode checkcommitreadiness "${APPROVAL_FLAGS[@]}" --output json >"${evidence}/readiness-seq${sequence}.json"
  expected_count="$(jq '.organizations | length' "${MANIFEST}")"
  jq -e --argjson expected "${expected_count}" '(.approvals | length) == $expected and all(.approvals[]; . == true)' "${evidence}/readiness-seq${sequence}.json" >/dev/null || fail "not all organizations approved sequence ${sequence}"
}

committed_definition() {
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(organization_rows)
  use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
  peer lifecycle chaincode querycommitted --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --output json 2>/dev/null
}

committed_matches() {
  local document="$1"
  local sequence="$2"
  local init_required="$3"
  local policy_kind="$4"
  local artifact="${5:-committed-seq${sequence}}"
  definition_matches "${document}" "${sequence}" "${init_required}" "${policy_kind}" "${artifact}"
}

commit_sequence() {
  local sequence="$1"
  local policy="$2"
  local init_required="$3"
  local policy_kind="$4"
  local evidence="${EVIDENCE_DIR}/net-4/lifecycle"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local committed
  approval_flags "${sequence}" "${policy}" "${init_required}"
  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(organization_rows)
  use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
  info "Committing chaincode definition sequence ${sequence}"
  peer lifecycle chaincode commit "${ORDERER_ARGS[@]}" "${APPROVAL_FLAGS[@]}" "${ALL_PEER_TARGETS[@]}"
  committed="$(committed_definition)"
  printf '%s\n' "${committed}" >"${evidence}/committed-seq${sequence}.json"
  committed_matches "${committed}" "${sequence}" "${init_required}" "${policy_kind}" "committed-seq${sequence}" || fail "committed sequence ${sequence} is inconsistent"
}

verify_pre_init_gate() {
  local evidence="${EVIDENCE_DIR}/net-4/lifecycle"
  local output status
  set +e
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor '{"function":"ReadUnit","Args":["07791234567898","SN-0001-ABCD"]}' 2>&1)"
  status=$?
  set -e
  printf '%s\n' "${output}" >"${evidence}/pre-init-gate.txt"
  [[ "${status}" -ne 0 ]] || fail "peer accepted a non-Init transaction before registry initialization"
  grep -Eqi 'must call as init first|requires initialization|has not been initialized' <<<"${output}" || {
    printf '%s\n' "${output}" >&2
    fail "pre-Init rejection was not caused by --init-required"
  }
}

initialize_registry() {
  local evidence="${EVIDENCE_DIR}/net-4/lifecycle"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local query_output query_status invoke_output
  local -a invoke_args
  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(regulator_row)
  use_organization "${msp_id}" "${slug}" "${peer_hostname}" User1
  set +e
  query_output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor '{"function":"Init","Args":[]}' 2>&1)"
  query_status=$?
  set -e
  if [[ "${query_status}" -ne 0 ]] && grep -q 'ALREADY_INITIALIZED' <<<"${query_output}"; then
    printf '%s\n' "${query_output}" >"${evidence}/init-reinvocation.txt"
    info "Registry is already initialized"
    return
  fi

  verify_pre_init_gate

  invoke_args=(
    "${ORDERER_ARGS[@]}"
    --channelID "${CHANNEL_NAME}"
    --name "${CHAINCODE_NAME}"
    --isInit
    --ctor '{"function":"Init","Args":[]}'
    "${ALL_PEER_TARGETS[@]}"
    --waitForEvent
    --waitForEventTimeout "${SNT_COMMIT_TIMEOUT:-180s}"
  )
  info "Invoking Init from the regulatory identity with all seven endorsers"
  invoke_output="$(peer chaincode invoke "${invoke_args[@]}" 2>&1)"
  printf '%s\n' "${invoke_output}" >"${evidence}/init-invoke.txt"

  set +e
  query_output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor '{"function":"Init","Args":[]}' 2>&1)"
  query_status=$?
  set -e
  printf '%s\n' "${query_output}" >"${evidence}/init-reinvocation.txt"
  if [[ "${query_status}" -eq 0 ]] || ! grep -q 'ALREADY_INITIALIZED' <<<"${query_output}"; then
    fail "regulatory seed could not be verified"
  fi
}

seed_organizations() {
  local evidence="${EVIDENCE_DIR}/net-4/bootstrap"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local regulator_msp regulator_slug regulator_peer
  local request ctor output status
  local -a invoke_args
  mkdir -p "${evidence}"
  read -r regulator_msp regulator_slug regulator_peer _ < <(regulator_row)
  use_organization "${regulator_msp}" "${regulator_slug}" "${regulator_peer}" User1
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    [[ "${agent_type}" != "REGULATOR" ]] || continue
    request="$(jq -cn --arg msp_id "${msp_id}" --arg id "${id}" --arg id_type "${id_type}" --arg agent_type "${agent_type}" --argjson active "${active}" '{mspId:$msp_id,id:$id,idType:$id_type,agentType:$agent_type,active:$active}')"
    ctor="$(jq -cn --arg request "${request}" '{function:"RegisterOrganization",Args:[$request]}')"
    invoke_args=(
      "${ORDERER_ARGS[@]}"
      --channelID "${CHANNEL_NAME}"
      --name "${CHAINCODE_NAME}"
      --ctor "${ctor}"
      --peerAddresses "${CORE_PEER_ADDRESS}"
      --tlsRootCertFiles "${CORE_PEER_TLS_ROOTCERT_FILE}"
      --waitForEvent
      --waitForEventTimeout "${SNT_COMMIT_TIMEOUT:-180s}"
    )
    info "Registering ${msp_id} in the organization registry"
    set +e
    output="$(peer chaincode invoke "${invoke_args[@]}" 2>&1)"
    status=$?
    set -e
    printf '%s\n' "${output}" >"${evidence}/register-${slug}.txt"
    if [[ "${status}" -eq 0 ]]; then
      continue
    fi
    if grep -q 'INVALID_REQUEST' <<<"${output}" && grep -q 'ya tiene entrada en el registro' <<<"${output}"; then
      info "${msp_id} was already registered; controlled rerun accepted"
      continue
    fi
    printf '%s\n' "${output}" >&2
    fail "RegisterOrganization failed for ${msp_id}"
  done < <(organization_rows)
}

verify_lifecycle() {
  local evidence="${EVIDENCE_DIR}/net-4/lifecycle"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local installed approved committed query_output status
  mkdir -p "${evidence}"
  read_package_lock
  committed="$(committed_definition)"
  printf '%s\n' "${committed}" >"${evidence}/committed-current.json"
  committed_matches "${committed}" 2 false operational committed-current || fail "operational sequence 2 differs from the versioned collections or policy; approve a new lifecycle sequence"
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
    installed="$(peer lifecycle chaincode queryinstalled --output json)"
    printf '%s\n' "${installed}" >"${evidence}/queryinstalled-${slug}.json"
    jq -e --arg package "${LOCK_PACKAGE_ID}" 'any(.installed_chaincodes[]?; .package_id == $package)' <<<"${installed}" >/dev/null || fail "${LOCK_PACKAGE_ID} is missing on ${peer_hostname}"
    approved="$(peer lifecycle chaincode queryapproved --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --sequence 2 --output json)"
    printf '%s\n' "${approved}" >"${evidence}/queryapproved-seq2-${slug}.json"
    approval_matches "${approved}" 2 false operational "approved-seq2-${slug}" || fail "${msp_id} lacks the expected sequence 2 approval"
  done < <(organization_rows)
  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(regulator_row)
  use_organization "${msp_id}" "${slug}" "${peer_hostname}" User1
  set +e
  query_output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor '{"function":"Init","Args":[]}' 2>&1)"
  status=$?
  set -e
  printf '%s\n' "${query_output}" >"${evidence}/init-reinvocation.txt"
  if [[ "${status}" -eq 0 ]] || ! grep -q 'ALREADY_INITIALIZED' <<<"${query_output}"; then
    fail "regulatory seed is not verifiable"
  fi
  printf 'OK: %s sequence 2 and regulatory seed verified\n' "${CHAINCODE_NAME}"
}

deploy_chaincode() {
  local committed sequence status
  local bootstrap_policy operational
  require_command docker
  resolve_fabric_environment
  validate_sources
  ensure_network_running
  mkdir -p "${EVIDENCE_DIR}/net-4/lifecycle"
  set_primary_orderer
  build_all_peer_targets
  verify_channel
  build_and_verify_package
  install_package_everywhere
  bootstrap_policy="$(all_organizations_policy)"
  operational="$(operational_policy)"
  set +e
  committed="$(committed_definition)"
  status=$?
  set -e
  if [[ "${status}" -ne 0 || -z "${committed}" ]]; then
    sequence=0
  else
    sequence="$(jq -r '.sequence' <<<"${committed}")"
  fi
  case "${sequence}" in
    0)
      approve_sequence 1 "${bootstrap_policy}" true bootstrap
      commit_sequence 1 "${bootstrap_policy}" true bootstrap
      initialize_registry
      sequence=1
      ;;
    1)
      committed_matches "${committed}" 1 true bootstrap || fail "sequence 1 is incompatible; follow documented recovery"
      initialize_registry
      ;;
    2)
      committed_matches "${committed}" 2 false operational || fail "existing sequence 2 differs from the versioned collections or policy; approve a new lifecycle sequence"
      ;;
    *) fail "unsupported sequence ${sequence}; automatic downgrade is forbidden" ;;
  esac
  if [[ "${sequence}" -eq 1 ]]; then
    approve_sequence 2 "${operational}" false operational
    commit_sequence 2 "${operational}" false operational
  fi
  seed_organizations
  verify_lifecycle
}

usage() {
  cat <<'USAGE'
Usage: ./network/network.sh <command>

Commands:
  up             validate inputs and start all services
  down           stop services and preserve ledger volumes
  restart        restart services and wait for health
  createChannel  create/join/verify snt-channel idempotently
  package-lock   rebuild twice and update the versioned package lock
  deployCC       deploy snt through bootstrap sequences 1 and 2
  verify         verify channel participation and lifecycle sequence 2
USAGE
}

main() {
  local command="${1:-}"
  case "${command}" in
    up) up_network ;;
    down) down_network ;;
    restart) restart_network ;;
    createChannel) create_channel ;;
    package-lock) package_lock ;;
    deployCC) deploy_chaincode ;;
    verify)
      resolve_fabric_environment
      validate_sources
      ensure_network_running
      set_primary_orderer
      verify_channel
      verify_lifecycle
      ;;
    -h|--help|help|"") usage ;;
    *) usage >&2; fail "unknown command: ${command}" ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
