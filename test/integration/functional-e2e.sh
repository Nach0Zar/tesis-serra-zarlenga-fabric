#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
export SNT_EVIDENCE_DIR="${REPOSITORY_ROOT}/build/evidence/ext-7"
# shellcheck source=network/network.sh
source "${REPOSITORY_ROOT}/network/network.sh"

RUN_TOKEN="${SNT_EXT7_RUN_TOKEN:-$(date -u +%m%d%H%M%S)}"
[[ "${RUN_TOKEN}" =~ ^[[:alnum:]]{1,14}$ ]] \
  || fail "SNT_EXT7_RUN_TOKEN must contain 1-14 alphanumeric characters"
readonly RUN_TOKEN

readonly GTIN="07791234567898"
readonly SERIAL_HAPPY="E7A${RUN_TOKEN}"
readonly SERIAL_EVENT="E7B${RUN_TOKEN}"
readonly EXPIRY_DATE="2035-12-31"
readonly HAPPY_HISTORY='["EN_LABORATORIO","EN_TRANSITO","EN_CUSTODIA","EN_TRANSITO","EN_CUSTODIA","DISPENSADO"]'
readonly CUSTODY_HISTORY='["EN_LABORATORIO","EN_TRANSITO","EN_CUSTODIA","EN_TRANSITO","EN_CUSTODIA"]'
readonly QUARANTINE_HISTORY='["EN_LABORATORIO","EN_TRANSITO","EN_CUSTODIA","EN_TRANSITO","EN_CUSTODIA","EN_CUARENTENA"]'

declare -a TARGET_ARGS=()
LAST_OUTPUT=""
LAST_STATUS=0

select_identity() {
  local wanted_msp="$1"
  local identity="${2:-User1}"
  local msp_id slug peer_hostname
  read -r msp_id slug peer_hostname < <(
    jq -r --arg msp "${wanted_msp}" '
      .organizations[]
      | select(.mspId == $msp)
      | [.mspId,.slug,.peerHostname]
      | @tsv
    ' "${MANIFEST}"
  )
  [[ "${msp_id}" == "${wanted_msp}" ]] || fail "organization not found: ${wanted_msp}"
  use_organization "${msp_id}" "${slug}" "${peer_hostname}" "${identity}"
}

canonical_id() {
  local wanted_msp="$1"
  jq -er --arg msp "${wanted_msp}" '
    .organizations[]
    | select(.mspId == $msp)
    | .idType + ":" + .id
  ' "${MANIFEST}"
}

build_targets() {
  local wanted_msp msp_id slug peer_hostname
  TARGET_ARGS=()
  for wanted_msp in "$@"; do
    read -r msp_id slug peer_hostname < <(
      jq -r --arg msp "${wanted_msp}" '
        .organizations[]
        | select(.mspId == $msp)
        | [.mspId,.slug,.peerHostname]
        | @tsv
      ' "${MANIFEST}"
    )
    [[ "${msp_id}" == "${wanted_msp}" ]] \
      || fail "endorser organization not found: ${wanted_msp}"
    TARGET_ARGS+=(
      --peerAddresses "$(published_endpoint "${peer_hostname}" 7051)"
      --tlsRootCertFiles "${ORGANIZATIONS_DIR}/${slug}/peers/${peer_hostname}/tls/ca.crt"
    )
  done
}

request_ctor() {
  local function="$1"
  local request="$2"
  jq -cn --arg function "${function}" --arg request "${request}" \
    '{function:$function,Args:[$request]}'
}

read_ctor() {
  local function="$1"
  local serial="$2"
  jq -cn --arg function "${function}" --arg gtin "${GTIN}" --arg serial "${serial}" \
    '{function:$function,Args:[$gtin,$serial]}'
}

dispatch_transient() {
  local destination="$1"
  local marker="$2"
  jq -cn --arg destination "${destination}" --arg marker "${marker}" '
    {
      destinatario: ({destino:$destination} | tojson | @base64),
      commercial: ({numeroRemito:$marker,numeroFactura:("FACT-" + $marker),cantidad:1} | tojson | @base64)
    }
  '
}

run_invoke() {
  local label="$1"
  local creator_msp="$2"
  local ctor="$3"
  local transient="$4"
  shift 4
  local -a args

  info "EXT-7: ${label}"
  select_identity "${creator_msp}" User1
  build_targets "$@"
  args=(
    "${ORDERER_ARGS[@]}"
    --channelID "${CHANNEL_NAME}"
    --name "${CHAINCODE_NAME}"
    --ctor "${ctor}"
    "${TARGET_ARGS[@]}"
    --waitForEvent
    --waitForEventTimeout "${SNT_COMMIT_TIMEOUT:-180s}"
  )
  [[ -z "${transient}" ]] || args+=(--transient "${transient}")

  set +e
  LAST_OUTPUT="$(peer chaincode invoke "${args[@]}" 2>&1)"
  LAST_STATUS=$?
  set -e
}

expect_valid() {
  local label="$1"
  local creator_msp="$2"
  local ctor="$3"
  local transient="$4"
  shift 4
  run_invoke "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  if [[ "${LAST_STATUS}" -ne 0 ]]; then
    printf '%s\n' "${LAST_OUTPUT}" >&2
    fail "${label} was expected to commit"
  fi
}

expect_logic_rejection() {
  local label="$1"
  local expected_code="$2"
  local creator_msp="$3"
  local ctor="$4"
  local transient="$5"
  shift 5
  run_invoke "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  [[ "${LAST_STATUS}" -ne 0 ]] || fail "${label} unexpectedly committed"
  if ! grep -Fq -- "${expected_code}" <<<"${LAST_OUTPUT}"; then
    printf '%s\n' "${LAST_OUTPUT}" >&2
    fail "${label} did not return application code ${expected_code}"
  fi
}

register_request() {
  local serial="$1"
  jq -cn \
    --arg gtin "${GTIN}" \
    --arg serial "${serial}" \
    --arg lot "EXT7${RUN_TOKEN}" \
    --arg expiry "${EXPIRY_DATE}" '
      {gtin:$gtin,numeroSerie:$serial,lote:$lot,fechaVencimiento:$expiry}
    '
}

unit_ref_request() {
  local serial="$1"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" \
    '{gtin:$gtin,numeroSerie:$serial}'
}

event_request() {
  local serial="$1"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" \
    --arg reason "Cuarentena funcional EXT-7" \
    '{gtin:$gtin,numeroSerie:$serial,motivo:$reason}'
}

query_unit() {
  local serial="$1"
  local ctor
  ctor="$(read_ctor ReadUnit "${serial}")"
  select_identity AnmatMSP User1
  peer chaincode query \
    --channelID "${CHANNEL_NAME}" \
    --name "${CHAINCODE_NAME}" \
    --ctor "${ctor}"
}

query_history() {
  local serial="$1"
  local ctor
  ctor="$(read_ctor GetUnitHistory "${serial}")"
  select_identity AnmatMSP User1
  peer chaincode query \
    --channelID "${CHANNEL_NAME}" \
    --name "${CHAINCODE_NAME}" \
    --ctor "${ctor}"
}

assert_unit() {
  local label="$1"
  local serial="$2"
  local expected_state="$3"
  local expected_custodian="$4"
  local output
  output="$(query_unit "${serial}")"
  jq -e \
    --arg state "${expected_state}" \
    --arg custodian "${expected_custodian}" '
      .estado == $state and .custodioActual == $custodian
    ' <<<"${output}" >/dev/null \
    || fail "${label} did not preserve state ${expected_state} and custodian ${expected_custodian}"
}

assert_history_states() {
  local label="$1"
  local history="$2"
  local expected_states="$3"
  jq -e --argjson expected "${expected_states}" '
    length == ($expected | length)
    and all(.[]; .isDelete == false and .value != null)
    and ([.[].value.estado] == $expected)
  ' <<<"${history}" >/dev/null \
    || fail "${label} history does not match the expected chronological states"
}

register_unit() {
  local serial="$1"
  local request ctor
  request="$(register_request "${serial}")"
  ctor="$(request_ctor RegisterUnit "${request}")"
  expect_valid "register-${serial}" LabMSP "${ctor}" "" LabMSP
}

transfer_unit() {
  local serial="$1"
  local source_msp="$2"
  local destination_msp="$3"
  local label="$4"
  local request ctor destination transient

  request="$(unit_ref_request "${serial}")"
  ctor="$(request_ctor DispatchTransfer "${request}")"
  destination="$(canonical_id "${destination_msp}")"
  transient="$(dispatch_transient "${destination}" "EXT7${RUN_TOKEN}${label}")"
  expect_valid "dispatch-${label}" "${source_msp}" "${ctor}" "${transient}" "${source_msp}"

  ctor="$(request_ctor ReceiveTransfer "${request}")"
  expect_valid "receive-${label}" "${destination_msp}" "${ctor}" "" \
    "${source_msp}" "${destination_msp}"
}

run_happy_path() {
  local pharmacy_id request ctor history
  pharmacy_id="$(canonical_id FarmaciaMSP)"

  info "EXT-7: running registration, transfers and dispensing"
  register_unit "${SERIAL_HAPPY}"
  transfer_unit "${SERIAL_HAPPY}" LabMSP DrogueriaMSP A
  transfer_unit "${SERIAL_HAPPY}" DrogueriaMSP FarmaciaMSP B

  request="$(unit_ref_request "${SERIAL_HAPPY}")"
  ctor="$(request_ctor Dispense "${request}")"
  expect_valid dispense-happy FarmaciaMSP "${ctor}" "" FarmaciaMSP

  assert_unit happy-final "${SERIAL_HAPPY}" DISPENSADO "${pharmacy_id}"
  history="$(query_history "${SERIAL_HAPPY}")"
  assert_history_states happy-final "${history}" "${HAPPY_HISTORY}"
}

run_rejection_and_extraordinary_event() {
  local pharmacy_id drugstore_id request ctor transient before_history after_history
  pharmacy_id="$(canonical_id FarmaciaMSP)"
  drugstore_id="$(canonical_id DrogueriaMSP)"

  info "EXT-7: running prohibited transfer and extraordinary event"
  register_unit "${SERIAL_EVENT}"
  transfer_unit "${SERIAL_EVENT}" LabMSP DrogueriaMSP C
  transfer_unit "${SERIAL_EVENT}" DrogueriaMSP FarmaciaMSP D

  assert_unit prohibited-before "${SERIAL_EVENT}" EN_CUSTODIA "${pharmacy_id}"
  before_history="$(query_history "${SERIAL_EVENT}")"
  assert_history_states prohibited-before "${before_history}" "${CUSTODY_HISTORY}"

  request="$(unit_ref_request "${SERIAL_EVENT}")"
  ctor="$(request_ctor DispatchTransfer "${request}")"
  transient="$(dispatch_transient "${drugstore_id}" "EXT7${RUN_TOKEN}PROHIBITED")"
  expect_logic_rejection prohibited-pharmacy-drugstore TRANSFER_NOT_AUTHORIZED \
    FarmaciaMSP "${ctor}" "${transient}" FarmaciaMSP

  assert_unit prohibited-after "${SERIAL_EVENT}" EN_CUSTODIA "${pharmacy_id}"
  after_history="$(query_history "${SERIAL_EVENT}")"
  assert_history_states prohibited-after "${after_history}" "${CUSTODY_HISTORY}"
  [[ "$(jq -S -c . <<<"${after_history}")" == "$(jq -S -c . <<<"${before_history}")" ]] \
    || fail "prohibited transfer changed the unit history"

  request="$(event_request "${SERIAL_EVENT}")"
  ctor="$(request_ctor Quarantine "${request}")"
  expect_valid quarantine-custodian FarmaciaMSP "${ctor}" "" FarmaciaMSP

  assert_unit quarantine-final "${SERIAL_EVENT}" EN_CUARENTENA "${pharmacy_id}"
  after_history="$(query_history "${SERIAL_EVENT}")"
  assert_history_states quarantine-final "${after_history}" "${QUARANTINE_HISTORY}"
}

main_ext7() {
  require_command docker
  require_command jq
  resolve_fabric_environment
  validate_sources
  ensure_network_running
  set_primary_orderer
  verify_channel
  verify_lifecycle

  run_happy_path
  run_rejection_and_extraordinary_event

  printf 'OK: EXT-7 functional E2E completed with run token %s\n' "${RUN_TOKEN}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main_ext7 "$@"
fi
