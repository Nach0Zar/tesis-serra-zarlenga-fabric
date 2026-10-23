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
readonly EVIDENCE_ROOT="${NET6_ROOT}"
readonly EVIDENCE_VALIDATOR="${NETWORK_DIR}/scripts/verify-net6-evidence.py"

# shellcheck source=test/integration/endorsement-evidence-common.sh
source "${REPOSITORY_ROOT}/test/integration/endorsement-evidence-common.sh"

DIVERGENT_ACTIVE=false

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
  request="$(read_ctor VerifyUnit "" "")"
  assert_not_stub VerifyUnit AnmatMSP "${request}" INVALID_REQUEST
}

verify_regulatory_registry_write() {
  local height request ctor
  select_identity AnmatMSP User1
  height="$(network_channel_height)"
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
  sanitize_captured_block register-unit-lab _implicit_org_LabMSP register-unit-marker-sanitized.json \
    '{"operacion":"RegisterUnit","mspId":"LabMSP","timestamp":"'
  assert_block_sbe register-unit-lab LabMSP
  expect_logic_rejection register-unit-duplicate UNIT_ALREADY_EXISTS LabMSP "${ctor}" "" LabMSP
  assert_unit_state register-unit "${SERIAL_RECEIVE}" EN_LABORATORIO
}

verify_receive_and_dispense() {
  local request ctor transient destination marker output

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

  ctor="$(read_ctor VerifyUnit "${GTIN}" "${SERIAL_RECEIVE}")"
  select_identity FarmaciaMSP User1
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor "${ctor}")"
  printf '%s\n' "${output}" >"${RUN_DIR}/core-unit-verdict.json"
  jq -e '.autentica == true and .motivo == "" and .estado == "EN_TRANSITO"
    and (.verificaciones | length == 4) and all(.verificaciones[]; .resultado == "OK")' <<<"${output}" >/dev/null \
    || fail "VerifyUnit did not accept the in-transit Core trace"

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
  before="$(network_channel_height)"
  run_invoke matrix-divergent-receive DrogueriaMSP "${ctor}" "" LabMSP DrogueriaMSP
  [[ "${LAST_STATUS}" -ne 0 ]] || fail "divergent receiver unexpectedly completed ReceiveTransfer"
  grep -Eq 'TRANSFER_NOT_AUTHORIZED|ProposalResponsePayloads do not match|endorsement' <<<"${LAST_OUTPUT}"     || fail "divergent receiver failed for an unexpected reason"
  sleep 2
  after="$(network_channel_height)"
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
      artifactManifest:"artifacts.json",
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
        coreAuthenticityVerified:true
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
  require_command sed
  require_command xargs
  resolve_fabric_environment
  validate_sources
  ensure_network_running
  prepare_run_directory
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
  python3 "${NETWORK_DIR}/scripts/verify-net6-evidence.py" run \
    --directory "${RUN_DIR}" --channel "${CHANNEL_NAME}" >"${RUN_DIR}/artifacts.json"
  write_result

  printf 'OK: NET-6 Core endorsement evidence completed under %s\n' "${RUN_DIR}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main_net6 "$@"
fi
