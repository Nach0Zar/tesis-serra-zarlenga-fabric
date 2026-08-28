#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=network/network.sh
source "${REPOSITORY_ROOT}/network/network.sh"

readonly NET6_ROOT="${EVIDENCE_DIR}/net-6"
RUN_TOKEN="${SNT_NET6_RUN_TOKEN:-$(date -u +%m%d%H%M%S)}"
[[ "${RUN_TOKEN}" =~ ^[[:alnum:]]{1,14}$ ]] || fail "SNT_NET6_RUN_TOKEN must contain 1-14 alphanumeric characters"
readonly RUN_TOKEN
readonly RUN_DIR="${NET6_ROOT}/run-${RUN_TOKEN}"
readonly GTIN="07791234567898"
readonly SERIAL_RECEIVE="N6A${RUN_TOKEN}"
readonly SERIAL_REJECT="N6B${RUN_TOKEN}"
readonly SERIAL_DIVERGENT="N6C${RUN_TOKEN}"
readonly EXPIRY_DATE="2035-12-31"
readonly TRANSFER_COLLECTION="transfer_DrogueriaMSP_LabMSP"
readonly SANITIZER="${NETWORK_DIR}/scripts/sanitize-pdc-evidence.py"

declare -a TARGET_ARGS=()
LAST_OUTPUT=""
LAST_STATUS=0
DIVERGENT_ACTIVE=false

select_identity() {
  local wanted_msp="$1"
  local identity="${2:-User1}"
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname < <(
    jq -r --arg msp "${wanted_msp}" '
      .organizations[]
      | select(.mspId == $msp)
      | [.mspId,.slug,.peerHostname,.agentType,.id,.idType,(.active|tostring),(.ordererHostname // "")]
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
    [[ "${msp_id}" == "${wanted_msp}" ]] || fail "endorser organization not found: ${wanted_msp}"
    TARGET_ARGS+=(
      --peerAddresses "$(published_endpoint "${peer_hostname}" 7051)"
      --tlsRootCertFiles "${ORGANIZATIONS_DIR}/${slug}/peers/${peer_hostname}/tls/ca.crt"
    )
  done
}

request_ctor() {
  local function="$1"
  local request="$2"
  jq -cn --arg function "${function}" --arg request "${request}"     '{function:$function,Args:[$request]}'
}

read_ctor() {
  local function="$1"
  local gtin="$2"
  local serial="$3"
  jq -cn --arg function "${function}" --arg gtin "${gtin}" --arg serial "${serial}"     '{function:$function,Args:[$gtin,$serial]}'
}

dispatch_transient() {
  local destination="$1"
  local commercial_marker="$2"
  jq -cn --arg destination "${destination}" --arg marker "${commercial_marker}" '
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
  printf '%s\n' "${LAST_OUTPUT}" >"${RUN_DIR}/${label}-invoke.txt"
  printf '%s\n' "${LAST_STATUS}" >"${RUN_DIR}/${label}-status.txt"
}

current_height() {
  local output
  output="$(peer channel getinfo --channelID "${CHANNEL_NAME}")"
  jq -r '.height' <<<"${output#Blockchain info: }"
}

wait_for_height() {
  local previous="$1"
  local current
  for _ in {1..30}; do
    current="$(current_height)"
    if [[ "${current}" -gt "${previous}" ]]; then
      printf '%s\n' "${current}"
      return
    fi
    sleep 1
  done
  fail "channel height did not advance after submitted transaction"
}

capture_block() {
  local label="$1"
  local block_number="$2"
  local raw="${RUN_DIR}/${label}-block-${block_number}.pb"
  local decoded="${RUN_DIR}/${label}-block-${block_number}.json"
  local filter codes
  select_identity AnmatMSP Admin
  peer channel fetch "${block_number}" "${raw}" --channelID "${CHANNEL_NAME}" "${ORDERER_ARGS[@]}"     >"${RUN_DIR}/${label}-block-fetch.txt" 2>&1
  configtxlator proto_decode --input "${raw}" --type common.Block --output "${decoded}"
  filter="$(jq -er '.metadata.metadata[2]' "${decoded}")"
  codes="$(printf '%s' "${filter}" | base64 --decode | od -An -t u1 | xargs)"
  printf '%s\n' "${codes}" >"${RUN_DIR}/${label}-validation-codes.txt"
}

captured_block_json() {
  local label="$1"
  local -a blocks=("${RUN_DIR}/${label}"-block-*.json)
  [[ "${#blocks[@]}" -eq 1 && -f "${blocks[0]}" ]] \
    || fail "expected one decoded block for ${label}"
  printf '%s\n' "${blocks[0]}"
}

sanitize_captured_block() {
  local label="$1"
  local collection="$2"
  local output_name="$3"
  local forbidden_value="$4"
  local decoded
  decoded="$(captured_block_json "${label}")"
  python3 "${SANITIZER}" \
    --input "${decoded}" \
    --collection "${collection}" \
    --output "${RUN_DIR}/${output_name}" \
    --forbidden-value "${forbidden_value}"
}

assert_block_sbe() {
  local label="$1"
  shift
  local decoded policy_value policy_pb policy_json principal role_pb
  local expected_sorted actual_sorted index field_role peer_role field_msp msp_length byte_count
  local -a expected=("$@")
  local -a principals=() actual=()
  [[ "${#expected[@]}" -gt 0 ]] || fail "assert_block_sbe requires at least one MSP"
  decoded="$(captured_block_json "${label}")"
  policy_pb="${RUN_DIR}/${label}-sbe.pb"
  policy_json="${RUN_DIR}/${label}-sbe.json"
  policy_value="$(jq -er --arg namespace "${CHAINCODE_NAME}" '
    [
      .data.data[]?.payload.data.actions[]?.payload.action.proposal_response_payload.extension.results.ns_rwset[]?
      | select(.namespace == $namespace)
      | .rwset.metadata_writes[]?
      | .entries[]?
      | select(.name == "VALIDATION_PARAMETER")
      | .value
    ]
    | if length == 1 then .[0] else error("expected exactly one validation parameter") end
  ' "${decoded}")"
  printf '%s' "${policy_value}" | base64 --decode >"${policy_pb}"
  configtxlator proto_decode --input "${policy_pb}" --type common.SignaturePolicyEnvelope --output "${policy_json}"
  mapfile -t principals < <(jq -er '.identities[].principal' "${policy_json}")
  jq -e 'all(.identities[]; .principal_classification == "ROLE")' "${policy_json}" >/dev/null \
    || fail "${label} SBE contains a non-role principal"
  [[ "${#principals[@]}" -eq "${#expected[@]}" ]] \
    || fail "${label} SBE has an unexpected number of principals"
  for index in "${!principals[@]}"; do
    principal="${principals[${index}]}"
    role_pb="${RUN_DIR}/${label}-sbe-role-${index}.pb"
    printf '%s' "${principal}" | base64 --decode >"${role_pb}"
    read -r field_role peer_role field_msp msp_length < <(od -An -t u1 -N4 "${role_pb}")
    [[ "${field_role}" -eq 8 && "${peer_role}" -eq 3 && "${field_msp}" -eq 18 ]] \
      || fail "${label} SBE principal is not an MSP peer role"
    byte_count="$(wc -c <"${role_pb}")"
    [[ "${msp_length}" -lt 128 && "${byte_count}" -eq $((4 + msp_length)) ]] \
      || fail "${label} SBE principal has an unexpected protobuf encoding"
    actual+=("$(tail -c +5 "${role_pb}")")
  done
  expected_sorted="$(printf '%s\n' "${expected[@]}" | sort)"
  actual_sorted="$(printf '%s\n' "${actual[@]}" | sort)"
  [[ "${actual_sorted}" == "${expected_sorted}" ]] \
    || fail "${label} SBE principals differ from the expected organizations"
  jq -e --argjson expected "${#expected[@]}" '(.rule.n_out_of.n | tonumber) == $expected' "${policy_json}" >/dev/null \
    || fail "${label} SBE does not require every listed organization"
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

expect_valid_with_block() {
  local label="$1"
  local creator_msp="$2"
  local ctor="$3"
  local transient="$4"
  shift 4
  local before
  select_identity "${creator_msp}" User1
  before="$(current_height)"
  expect_valid "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  wait_for_height "${before}" >/dev/null
  capture_block "${label}" "${before}"
}

expect_platform_rejection() {
  local label="$1"
  local creator_msp="$2"
  local ctor="$3"
  local transient="$4"
  shift 4
  local before codes
  select_identity "${creator_msp}" User1
  before="$(current_height)"
  run_invoke "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  [[ "${LAST_STATUS}" -ne 0 ]] || fail "${label} unexpectedly committed"
  grep -q 'ENDORSEMENT_POLICY_FAILURE' <<<"${LAST_OUTPUT}"     || fail "${label} did not report ENDORSEMENT_POLICY_FAILURE"
  wait_for_height "${before}" >/dev/null
  capture_block "${label}" "${before}"
  codes="$(<"${RUN_DIR}/${label}-validation-codes.txt")"
  grep -qw '10' <<<"${codes}"     || fail "${label} block does not contain Fabric validation code 10 (ENDORSEMENT_POLICY_FAILURE)"
}

expect_logic_rejection() {
  local label="$1"
  local expected_code="$2"
  local creator_msp="$3"
  local ctor="$4"
  local transient="$5"
  shift 5
  local before after
  select_identity "${creator_msp}" User1
  before="$(current_height)"
  run_invoke "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  [[ "${LAST_STATUS}" -ne 0 ]] || fail "${label} unexpectedly committed"
  grep -q "${expected_code}" <<<"${LAST_OUTPUT}"     || fail "${label} did not return application code ${expected_code}"
  sleep 2
  after="$(current_height)"
  [[ "${after}" -eq "${before}" ]]     || fail "${label} reached the ledger despite being rejected during proposal simulation"
  printf '%s\n' "${before}" >"${RUN_DIR}/${label}-unchanged-height.txt"
}

assert_not_stub() {
  local operation="$1"
  local creator_msp="$2"
  local ctor="$3"
  local expected_code="$4"
  local output status
  select_identity "${creator_msp}" User1
  set +e
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor "${ctor}" 2>&1)"
  status=$?
  set -e
  printf '%s\n' "${output}" >"${RUN_DIR}/preflight-${operation}.txt"
  [[ "${status}" -ne 0 ]] || fail "preflight ${operation} unexpectedly succeeded"
  if grep -q 'pertenece a CC-' <<<"${output}"; then
    fail "${operation} is still a chaincode stub; merge the Core CC issues into develop first"
  fi
  grep -q "${expected_code}" <<<"${output}"     || fail "${operation} preflight did not return ${expected_code}"
}

assert_unit_state() {
  local label="$1"
  local serial="$2"
  local expected_state="$3"
  local ctor output
  ctor="$(read_ctor ReadUnit "${GTIN}" "${serial}")"
  select_identity AnmatMSP User1
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor "${ctor}")"
  printf '%s\n' "${output}" >"${RUN_DIR}/${label}-unit.json"
  jq -e --arg state "${expected_state}" '.estado == $state' <<<"${output}" >/dev/null     || fail "${label} expected state ${expected_state}"
}

register_request() {
  local serial="$1"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" --arg expiry "${EXPIRY_DATE}" '
    {gtin:$gtin,numeroSerie:$serial,lote:"NET6-CORE",fechaVencimiento:$expiry}
  '
}

unit_ref_request() {
  local serial="$1"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}"     '{gtin:$gtin,numeroSerie:$serial}'
}

reject_request() {
  local serial="$1"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}"     '{gtin:$gtin,numeroSerie:$serial,motivo:"Rechazo controlado de evidencia NET-6"}'
}

verify_core_implementations() {
  local empty_unit request
  empty_unit='{"gtin":"","numeroSerie":""}'
  request="$(request_ctor RegisterUnit '{"gtin":"","numeroSerie":"","lote":"","fechaVencimiento":""}')"
  assert_not_stub RegisterUnit LabMSP "${request}" INVALID_REQUEST
  request="$(request_ctor DispatchTransfer "${empty_unit}")"
  assert_not_stub DispatchTransfer LabMSP "${request}" INVALID_REQUEST
  request="$(request_ctor Dispense "${empty_unit}")"
  assert_not_stub Dispense FarmaciaMSP "${request}" INVALID_REQUEST
  request="$(read_ctor ReadUnit "" "")"
  assert_not_stub ReadUnit AnmatMSP "${request}" INVALID_REQUEST
  request="$(read_ctor GetUnitHistory "" "")"
  assert_not_stub GetUnitHistory AnmatMSP "${request}" INVALID_REQUEST
  request="$(read_ctor VerifyTrace "" "")"
  assert_not_stub VerifyTrace FinanciadorMSP "${request}" INVALID_REQUEST
}

verify_regulatory_registry_write() {
  local height request ctor
  select_identity AnmatMSP User1
  height="$(current_height)"
  request="$(jq -cn --arg msp "Net6Evidence${height}MSP" --arg id "NET6-${height}" '
    {mspId:$msp,id:$id,idType:"REG",agentType:"FINANCIER",active:true}
  ')"
  ctor="$(request_ctor RegisterOrganization "${request}")"
  expect_logic_rejection registry-wrong-creator REGULATORY_ONLY LabMSP "${ctor}" "" LabMSP
  expect_valid_with_block registry-regulator-only AnmatMSP "${ctor}" "" AnmatMSP
  sanitize_captured_block registry-regulator-only _implicit_org_AnmatMSP registry-marker-sanitized.json "NET6-${height}"
}

verify_register_unit_endorsement() {
  local request ctor
  request="$(register_request "${SERIAL_RECEIVE}")"
  ctor="$(request_ctor RegisterUnit "${request}")"
  expect_platform_rejection register-unit-regulator-only LabMSP "${ctor}" "" AnmatMSP
  expect_valid_with_block register-unit-lab LabMSP "${ctor}" "" LabMSP
  sanitize_captured_block register-unit-lab _implicit_org_LabMSP register-unit-marker-sanitized.json "${SERIAL_RECEIVE}"
  assert_block_sbe register-unit-lab LabMSP
  expect_logic_rejection register-unit-duplicate UNIT_ALREADY_EXISTS LabMSP "${ctor}" "" LabMSP
  assert_unit_state register-unit "${SERIAL_RECEIVE}" EN_LABORATORIO
}

verify_receive_and_dispense() {
  local request ctor transient destination marker

  destination="$(canonical_id DrogueriaMSP)"
  marker="NET6-COMMERCIAL-${RUN_TOKEN}-A"
  request="$(unit_ref_request "${SERIAL_RECEIVE}")"
  ctor="$(request_ctor DispatchTransfer "${request}")"
  transient="$(dispatch_transient "${destination}" "${marker}")"
  expect_valid_with_block dispatch-lab-drugstore LabMSP "${ctor}" "${transient}" LabMSP
  sanitize_captured_block dispatch-lab-drugstore "${TRANSFER_COLLECTION}" dispatch-lab-drugstore-sanitized.json "${marker}"
  assert_block_sbe dispatch-lab-drugstore LabMSP DrogueriaMSP

  ctor="$(request_ctor ReceiveTransfer "${request}")"
  expect_platform_rejection receive-one-party DrogueriaMSP "${ctor}" "" DrogueriaMSP
  expect_valid_with_block receive-two-parties DrogueriaMSP "${ctor}" "" LabMSP DrogueriaMSP
  assert_block_sbe receive-two-parties DrogueriaMSP
  assert_unit_state receive "${SERIAL_RECEIVE}" EN_CUSTODIA

  destination="$(canonical_id FarmaciaMSP)"
  marker="NET6-COMMERCIAL-${RUN_TOKEN}-B"
  ctor="$(request_ctor DispatchTransfer "${request}")"
  transient="$(dispatch_transient "${destination}" "${marker}")"
  expect_valid dispatch-drugstore-pharmacy DrogueriaMSP "${ctor}" "${transient}" DrogueriaMSP

  ctor="$(request_ctor ReceiveTransfer "${request}")"
  expect_platform_rejection receive-without-sender FarmaciaMSP "${ctor}" "" FarmaciaMSP AnmatMSP
  expect_valid_with_block receive-drugstore-pharmacy FarmaciaMSP "${ctor}" "" DrogueriaMSP FarmaciaMSP
  assert_block_sbe receive-drugstore-pharmacy FarmaciaMSP
  assert_unit_state pharmacy-receive "${SERIAL_RECEIVE}" EN_CUSTODIA

  ctor="$(request_ctor Dispense "${request}")"
  expect_platform_rejection dispense-regulator-only FarmaciaMSP "${ctor}" "" AnmatMSP
  expect_valid dispense-custodian FarmaciaMSP "${ctor}" "" FarmaciaMSP
  assert_unit_state dispense "${SERIAL_RECEIVE}" DISPENSADO
}

verify_core_queries() {
  local ctor output

  ctor="$(read_ctor GetUnitHistory "${GTIN}" "${SERIAL_RECEIVE}")"
  select_identity AnmatMSP User1
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor "${ctor}")"
  printf '%s\n' "${output}" >"${RUN_DIR}/core-history.json"
  jq -e 'length >= 6 and all(.[]; .isDelete == false and .value != null)' <<<"${output}" >/dev/null \
    || fail "Core history does not contain the complete successful flow"

  ctor="$(read_ctor VerifyTrace "${GTIN}" "${SERIAL_RECEIVE}")"
  select_identity FinanciadorMSP User1
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor "${ctor}")"
  printf '%s\n' "${output}" >"${RUN_DIR}/core-trace-verdict.json"
  jq -e '.legitima == true and .motivo == ""' <<<"${output}" >/dev/null \
    || fail "VerifyTrace did not accept the completed Core trace"
}

verify_reject_restoration() {
  local request ctor transient destination
  request="$(register_request "${SERIAL_REJECT}")"
  ctor="$(request_ctor RegisterUnit "${request}")"
  expect_valid register-unit-reject-path LabMSP "${ctor}" "" LabMSP

  request="$(unit_ref_request "${SERIAL_REJECT}")"
  ctor="$(request_ctor DispatchTransfer "${request}")"
  destination="$(canonical_id DrogueriaMSP)"
  transient="$(dispatch_transient "${destination}" "NET6-COMMERCIAL-${RUN_TOKEN}-REJECT")"
  expect_valid dispatch-reject-path LabMSP "${ctor}" "${transient}" LabMSP

  request="$(reject_request "${SERIAL_REJECT}")"
  ctor="$(request_ctor RejectTransfer "${request}")"
  expect_platform_rejection reject-one-party DrogueriaMSP "${ctor}" "" DrogueriaMSP
  expect_valid_with_block reject-two-parties DrogueriaMSP "${ctor}" "" LabMSP DrogueriaMSP
  assert_block_sbe reject-two-parties LabMSP
  assert_unit_state reject "${SERIAL_REJECT}" DEVUELTO
}

restore_receiver_package() {
  local policy approved
  [[ "${DIVERGENT_ACTIVE}" == "true" ]] || return 0
  policy="$(operational_policy)"
  select_identity DrogueriaMSP Admin
  approval_flags 2 "${policy}" false
  peer lifecycle chaincode approveformyorg "${ORDERER_ARGS[@]}" "${APPROVAL_FLAGS[@]}"     --package-id "${LOCK_PACKAGE_ID}" >"${RUN_DIR}/matrix-restore-approval.txt" 2>&1
  approved="$(peer lifecycle chaincode queryapproved     --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --sequence 2 --output json)"
  printf '%s\n' "${approved}" >"${RUN_DIR}/matrix-restore-queryapproved.json"
  document_package_id_matches "${approved}" "${LOCK_PACKAGE_ID}"     || fail "receiver approval was not restored to the canonical package"
  DIVERGENT_ACTIVE=false
}

restore_receiver_on_exit() {
  local status=$?
  if [[ "${DIVERGENT_ACTIVE}" == "true" ]]; then
    set +e
    restore_receiver_package
    local restore_status=$?
    set -e
    if [[ "${restore_status}" -ne 0 ]]; then
      printf 'ERROR: could not restore canonical receiver package\n' >&2
      exit "${restore_status}"
    fi
  fi
  exit "${status}"
}

activate_divergent_receiver_package() {
  local canonical_stage divergent_root divergent_stage matrix_file temporary
  local divergent_package divergent_label divergent_id policy approved
  require_command go
  require_command make
  read_package_lock
  make -C "${REPOSITORY_ROOT}/chaincode" stage >/dev/null
  canonical_stage="${REPOSITORY_ROOT}/build/chaincode/${CHAINCODE_LABEL}"
  divergent_root="${REPOSITORY_ROOT}/build/test/net-6-divergent"
  divergent_stage="${divergent_root}/stage"
  divergent_label="snt_net6_divergent_${RUN_TOKEN}"
  divergent_package="${divergent_root}/${divergent_label}.tar.gz"
  [[ "${divergent_root}" == "${REPOSITORY_ROOT}/build/test/"* ]] || fail "unsafe divergent build path"
  rm -rf -- "${divergent_root}"
  mkdir -p "${divergent_stage}"
  cp -R -- "${canonical_stage}/." "${divergent_stage}/"
  matrix_file="${divergent_stage}/vendor/github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/authorized-transfers.json"
  [[ -f "${matrix_file}" ]] || fail "vendored transfer matrix not found in staged package"
  temporary="${matrix_file}.tmp"
  jq '(.authorizedTransfers[] | select(.id == "LABORATORY_TO_DRUGSTORE").id) = "LABORATORY_TO_DRUGSTORE_NET6_DIVERGENT"'     "${matrix_file}" >"${temporary}"
  mv -- "${temporary}" "${matrix_file}"
  mkdir -p "${divergent_root}/stage-check"
  (cd "${divergent_stage}" && go build -mod=vendor -o "${divergent_root}/stage-check/" ./...)
  peer lifecycle chaincode package "${divergent_package}"     --path "${divergent_stage}" --lang golang --label "${divergent_label}"
  divergent_id="$(peer lifecycle chaincode calculatepackageid "${divergent_package}")"
  printf '%s\n' "${divergent_id}" >"${RUN_DIR}/matrix-divergent-package-id.txt"

  select_identity DrogueriaMSP Admin
  peer lifecycle chaincode install "${divergent_package}" >"${RUN_DIR}/matrix-divergent-install.txt" 2>&1
  policy="$(operational_policy)"
  approval_flags 2 "${policy}" false
  peer lifecycle chaincode approveformyorg "${ORDERER_ARGS[@]}" "${APPROVAL_FLAGS[@]}"     --package-id "${divergent_id}" >"${RUN_DIR}/matrix-divergent-approval.txt" 2>&1
  DIVERGENT_ACTIVE=true
  approved="$(peer lifecycle chaincode queryapproved     --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --sequence 2 --output json)"
  printf '%s\n' "${approved}" >"${RUN_DIR}/matrix-divergent-queryapproved.json"
  document_package_id_matches "${approved}" "${divergent_id}"     || fail "receiver did not approve the divergent package"
}

verify_matrix_divergence() {
  local request ctor transient destination before after
  request="$(register_request "${SERIAL_DIVERGENT}")"
  ctor="$(request_ctor RegisterUnit "${request}")"
  expect_valid register-unit-divergence LabMSP "${ctor}" "" LabMSP

  request="$(unit_ref_request "${SERIAL_DIVERGENT}")"
  ctor="$(request_ctor DispatchTransfer "${request}")"
  destination="$(canonical_id DrogueriaMSP)"
  transient="$(dispatch_transient "${destination}" "NET6-COMMERCIAL-${RUN_TOKEN}-DIVERGENT")"
  expect_valid dispatch-divergence LabMSP "${ctor}" "${transient}" LabMSP

  trap restore_receiver_on_exit EXIT
  activate_divergent_receiver_package

  ctor="$(request_ctor ReceiveTransfer "${request}")"
  select_identity DrogueriaMSP User1
  before="$(current_height)"
  run_invoke matrix-divergent-receive DrogueriaMSP "${ctor}" "" LabMSP DrogueriaMSP
  [[ "${LAST_STATUS}" -ne 0 ]] || fail "divergent receiver unexpectedly completed ReceiveTransfer"
  grep -Eq 'TRANSFER_NOT_AUTHORIZED|ProposalResponsePayloads do not match|endorsement' <<<"${LAST_OUTPUT}"     || fail "divergent receiver failed for an unexpected reason"
  sleep 2
  after="$(current_height)"
  [[ "${after}" -eq "${before}" ]] || fail "divergent endorsement reached the ledger"
  printf '%s\n' "${before}" >"${RUN_DIR}/matrix-divergent-unchanged-height.txt"

  restore_receiver_package
  trap - EXIT
  expect_valid matrix-canonical-receive DrogueriaMSP "${ctor}" "" LabMSP DrogueriaMSP
  assert_unit_state matrix-canonical "${SERIAL_DIVERGENT}" EN_CUSTODIA
}

verify_net5_evidence() {
  local result="${EVIDENCE_DIR}/net-5/result.json"
  local excerpt="${EVIDENCE_DIR}/net-5/sanitized-block-excerpt.json"
  [[ -f "${result}" && -f "${excerpt}" ]]     || fail "NET-5 evidence is missing; run ./test/integration/pdc-evidence.sh first"
  jq -e '
    .publicReadableByAllOrganizations == true
    and .privateReadableByPairAndRegulator == true
    and .privateRejectedForNonMember == true
    and .regulatorOnlyWriteRejected == true
    and .implicitOwnerWriteVerified == true
    and .implicitThirdPartyReadRejected == true
    and .implicitNonOwnerWriteRejected == true
  ' "${result}" >/dev/null || fail "NET-5 evidence does not cover the required explicit and implicit collection properties"
  jq -e '
    .assertions.collectionNameVisible == true
    and .assertions.privatePayloadIncluded == false
  ' "${excerpt}" >/dev/null || fail "NET-5 sanitized block excerpt is incomplete"
}

write_result() {
  local commit package_id
  commit="$(git -C "${REPOSITORY_ROOT}" rev-parse HEAD)"
  read_package_lock
  package_id="${LOCK_PACKAGE_ID}"
  jq -n     --arg schemaVersion "1.0.0"     --arg runToken "${RUN_TOKEN}"     --arg commit "${commit}"     --arg channel "${CHANNEL_NAME}"     --arg chaincode "${CHAINCODE_NAME}"     --arg packageID "${package_id}" '
    {
      schemaVersion:$schemaVersion,
      runToken:$runToken,
      repositoryCommit:$commit,
      channel:$channel,
      chaincode:$chaincode,
      packageID:$packageID,
      assertions:{
        bootstrapEvidencePresent:true,
        regulatorRegistryWrite:true,
        registryWrongCreatorRejected:true,
        regulatoryMarkerHashed:true,
        registerUnitMarkerHashed:true,
        stateEndorsementPoliciesDecoded:true,
        registerUnitRequiresLaboratory:true,
        receiveRequiresSenderAndReceiver:true,
        rejectRequiresSenderAndReceiver:true,
        receiveRestoresReceiverSBE:true,
        rejectRestoresSenderSBE:true,
        regulatorCannotReplaceSender:true,
        dispenseRequiresCustodian:true,
        actualDispatchHashedPDC:true,
        explicitPDCPolicyVerifiedByProbe:true,
        implicitCollectionPolicyVerifiedByProbe:true,
        matrixDivergenceRejected:true,
        platformAndLogicRejectionsSeparated:true,
        coreQueriesCompleted:true,
        coreTraceVerified:true
      }
    }
  ' >"${RUN_DIR}/result.json"
}

main_net6() {
  require_command jq
  require_command python3
  require_command configtxlator
  require_command base64
  require_command od
  require_command git
  require_command sort
  require_command tail
  require_command wc
  require_command xargs
  resolve_fabric_environment
  validate_sources
  ensure_network_running
  mkdir -p "${RUN_DIR}"
  set_primary_orderer
  build_all_peer_targets
  verify_channel
  verify_lifecycle

  [[ -f "${EVIDENCE_DIR}/net-6/bootstrap/init-insufficient-endorsers.txt" ]] \
    || fail "bootstrap evidence is missing; deploy snt from a clean ledger with the NET-6-aware network script"

  verify_core_implementations
  verify_net5_evidence
  verify_regulatory_registry_write
  verify_register_unit_endorsement
  verify_receive_and_dispense
  verify_core_queries
  verify_reject_restoration
  verify_matrix_divergence
  write_result

  printf 'OK: NET-6 Core endorsement evidence completed under %s\n' "${RUN_DIR}"
}

main_net6 "$@"
