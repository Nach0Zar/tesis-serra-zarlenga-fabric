#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=network/network.sh
source "${REPOSITORY_ROOT}/network/network.sh"

readonly NET9_ROOT="${EVIDENCE_DIR}/net-9"
RUN_TOKEN="${SNT_NET9_RUN_TOKEN:-$(date -u +%m%d%H%M%S)}"
[[ "${RUN_TOKEN}" =~ ^[[:alnum:]]{1,14}$ ]] || fail "SNT_NET9_RUN_TOKEN must contain 1-14 alphanumeric characters"
readonly RUN_TOKEN
readonly RUN_DIR="${NET9_ROOT}/run-${RUN_TOKEN}"
readonly GTIN="07791234567898"
readonly FUTURE_EXPIRY="2035-12-31"
readonly PAST_EXPIRY="2020-01-01"
readonly TRANSFER_COLLECTION="transfer_DrogueriaMSP_LabMSP"
readonly EVIDENCE_ROOT="${NET9_ROOT}"
readonly EVIDENCE_VALIDATOR="${NETWORK_DIR}/scripts/verify-net9-evidence.py"
readonly SANITIZER="${NETWORK_DIR}/scripts/sanitize-pdc-evidence.py"

# shellcheck source=test/integration/endorsement-evidence-common.sh
source "${REPOSITORY_ROOT}/test/integration/endorsement-evidence-common.sh"

register_request() {
  local serial="$1"
  local expiry="$2"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" --arg expiry "${expiry}" '
    {gtin:$gtin,numeroSerie:$serial,lote:"NET9-EVIDENCE",fechaVencimiento:$expiry}
  '
}

unit_ref_request() {
  local serial="$1"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" '{gtin:$gtin,numeroSerie:$serial}'
}

unit_event_request() {
  local serial="$1"
  local reason="$2"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" --arg reason "${reason}" '{gtin:$gtin,numeroSerie:$serial,motivo:$reason}'
}

authorize_request() {
  local serial="$1"
  local operation="$2"
  local expires_at="$3"
  local reason="$4"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" --arg laboratory "$(canonical_id LabMSP)" --arg operation "${operation}" --arg reason "${reason}" --arg expiresAt "${expires_at}" '
    {
      gtin:$gtin,
      numeroSerie:$serial,
      laboratorio:$laboratory,
      operacion:$operation,
      motivo:$reason,
      expiraEn:$expiresAt
    }
  '
}

revoke_request() {
  local serial="$1"
  local reason="$2"
  jq -cn --arg gtin "${GTIN}" --arg serial "${serial}" --arg reason "${reason}" '{gtin:$gtin,numeroSerie:$serial,motivo:$reason}'
}

query_lab_history() {
  local output_name="$1"
  local serial="$2"
  local ctor output
  ctor="$(read_ctor GetLabInterventionHistory "${GTIN}" "${serial}")"
  select_identity AnmatMSP User1
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor "${ctor}")"
  printf '%s\n' "${output}" >"${RUN_DIR}/${output_name}"
}

register_fixture() {
  local label="$1"
  local serial="$2"
  local expiry="$3"
  local request ctor
  request="$(register_request "${serial}" "${expiry}")"
  ctor="$(request_ctor RegisterUnit "${request}")"
  expect_valid "${label}-register" LabMSP "${ctor}" "" LabMSP
}

dispatch_fixture() {
  local label="$1"
  local serial="$2"
  local request ctor transient marker
  request="$(unit_ref_request "${serial}")"
  ctor="$(request_ctor DispatchTransfer "${request}")"
  marker="NET9-${label^^}-${RUN_TOKEN}"
  transient="$(dispatch_transient "$(canonical_id DrogueriaMSP)" "${marker}")"
  expect_valid "${label}-dispatch" LabMSP "${ctor}" "${transient}" LabMSP
  record_last_transaction_id "${label}-dispatch"
}

move_fixture_to_drugstore() {
  local label="$1"
  local serial="$2"
  local request ctor
  register_fixture "${label}" "${serial}" "${FUTURE_EXPIRY}"
  dispatch_fixture "${label}" "${serial}"
  request="$(unit_ref_request "${serial}")"
  ctor="$(request_ctor ReceiveTransfer "${request}")"
  expect_valid "${label}-receive" DrogueriaMSP "${ctor}" "" LabMSP DrogueriaMSP
  assert_unit_state "${label}-received" "${serial}" EN_CUSTODIA
}

decode_regulatory_marker() {
  local label="$1"
  decode_hashed_rwset "${label}" _implicit_org_AnmatMSP "${label}-marker-AnmatMSP.json"
}

decode_lab_markers() {
  local label="$1"
  decode_hashed_rwset "${label}" _implicit_org_LabMSP "${label}-marker-LabMSP.json"
  decode_regulatory_marker "${label}"
}

verify_extraordinary_implementations() {
  local function request ctor
  request='{"gtin":"","numeroSerie":"","motivo":""}'
  for function in Quarantine ReleaseQuarantine ReportExpired ReportStolen ReportLost ReportDamaged WithdrawFromMarket ProhibitProduct ReturnProduct Restock FinalDisposition; do
    ctor="$(request_ctor "${function}" "${request}")"
    assert_not_stub "${function}" AnmatMSP "${ctor}" INVALID_REQUEST
  done
  ctor="$(request_ctor AuthorizeLabIntervention '{"gtin":"","numeroSerie":"","laboratorio":"","operacion":"","motivo":"","expiraEn":""}')"
  assert_not_stub AuthorizeLabIntervention AnmatMSP "${ctor}" INVALID_REQUEST
  ctor="$(request_ctor RevokeLabIntervention "${request}")"
  assert_not_stub RevokeLabIntervention AnmatMSP "${ctor}" INVALID_REQUEST
  ctor="$(read_ctor GetLabInterventionHistory "" "")"
  assert_not_stub GetLabInterventionHistory AnmatMSP "${ctor}" INVALID_REQUEST
}

write_run_metadata() {
  local commit package_id
  commit="$(git -C "${REPOSITORY_ROOT}" rev-parse HEAD)"
  read_package_lock
  package_id="${LOCK_PACKAGE_ID}"
  jq -n --arg schemaVersion "1.0.0" --arg runToken "${RUN_TOKEN}" --arg commit "${commit}" --arg channel "${CHANNEL_NAME}" --arg chaincode "${CHAINCODE_NAME}" --arg packageID "${package_id}" '
    {
      schemaVersion:$schemaVersion,
      runToken:$runToken,
      repositoryCommit:$commit,
      channel:$channel,
      chaincode:$chaincode,
      packageID:$packageID
    }
  ' >"${RUN_DIR}/run-metadata.json"
}

verify_transit_closure() {
  local label="$1"
  local serial="$2"
  local expiry="$3"
  local function="$4"
  local expected_state="$5"
  local follow_up="${6:-}"
  local follow_up_state="${7:-}"
  local request ctor

  register_fixture "${label}" "${serial}" "${expiry}"
  dispatch_fixture "${label}" "${serial}"
  request="$(unit_event_request "${serial}" "Cierre extraordinario ${function} de evidencia NET-9")"
  ctor="$(request_ctor "${function}" "${request}")"

  if [[ "${label}" == "transit-quarantine" ]]; then
    expect_platform_rejection "${label}-missing-sender" AnmatMSP "${ctor}" "" AnmatMSP DrogueriaMSP
    expect_platform_rejection "${label}-missing-receiver" AnmatMSP "${ctor}" "" AnmatMSP LabMSP
    expect_platform_rejection "${label}-missing-regulator" AnmatMSP "${ctor}" "" LabMSP DrogueriaMSP
  fi

  expect_valid_with_block "${label}" AnmatMSP "${ctor}" "" LabMSP DrogueriaMSP AnmatMSP
  assert_block_sbe "${label}" LabMSP
  decode_hashed_rwset "${label}" "${TRANSFER_COLLECTION}" "${label}-transfer-hashed-rwset.json"
  decode_regulatory_marker "${label}"
  assert_unit_state "${label}" "${serial}" "${expected_state}"

  if [[ -n "${follow_up}" ]]; then
    ctor="$(request_ctor "${follow_up}" "${request}")"
    expect_valid "${label}-follow-up" LabMSP "${ctor}" "" LabMSP
    assert_unit_state "${label}-unlocked" "${serial}" "${follow_up_state}"
  fi
}

verify_transit_closures() {
  verify_transit_closure transit-quarantine "N9TQ${RUN_TOKEN}" "${FUTURE_EXPIRY}" Quarantine EN_CUARENTENA ReleaseQuarantine EN_CUSTODIA
  verify_transit_closure transit-expired "N9TE${RUN_TOKEN}" "${PAST_EXPIRY}" ReportExpired VENCIDO FinalDisposition DISPUESTO_FINAL
  verify_transit_closure transit-stolen "N9TS${RUN_TOKEN}" "${FUTURE_EXPIRY}" ReportStolen ROBADO
  verify_transit_closure transit-lost "N9TL${RUN_TOKEN}" "${FUTURE_EXPIRY}" ReportLost EXTRAVIADO
  verify_transit_closure transit-damaged "N9TD${RUN_TOKEN}" "${FUTURE_EXPIRY}" ReportDamaged DETERIORADO FinalDisposition DISPUESTO_FINAL

  sanitize_captured_block transit-quarantine _implicit_org_AnmatMSP transit-quarantine-regulator-marker-sanitized.json '{"operacion":"Quarantine","mspId":"AnmatMSP","timestamp":"'
  sanitize_captured_block transit-quarantine "${TRANSFER_COLLECTION}" transit-quarantine-transfer-sanitized.json "NET9-TRANSIT-QUARANTINE-${RUN_TOKEN}"
}

regulatory_event() {
  local label="$1"
  local function="$2"
  local serial="$3"
  local request ctor
  request="$(unit_event_request "${serial}" "Evento regulatorio ${function} de evidencia NET-9")"
  ctor="$(request_ctor "${function}" "${request}")"
  expect_valid_with_block "${label}" AnmatMSP "${ctor}" "" AnmatMSP LabMSP
  decode_regulatory_marker "${label}"
}

verify_regulatory_events() {
  local serial="N9RA${RUN_TOKEN}"
  local final_serial="N9RF${RUN_TOKEN}"
  local request ctor

  register_fixture regulatory-main "${serial}" "${FUTURE_EXPIRY}"
  request="$(unit_event_request "${serial}" "Cuarentena regulatoria de evidencia NET-9")"
  ctor="$(request_ctor Quarantine "${request}")"
  expect_platform_rejection regulatory-quarantine-missing-regulator AnmatMSP "${ctor}" "" LabMSP
  expect_platform_rejection regulatory-quarantine-missing-custodian AnmatMSP "${ctor}" "" AnmatMSP
  expect_valid_with_block regulatory-quarantine AnmatMSP "${ctor}" "" AnmatMSP LabMSP
  decode_regulatory_marker regulatory-quarantine

  regulatory_event regulatory-release ReleaseQuarantine "${serial}"
  regulatory_event regulatory-withdraw WithdrawFromMarket "${serial}"
  regulatory_event regulatory-restock Restock "${serial}"
  regulatory_event regulatory-prohibit ProhibitProduct "${serial}"
  regulatory_event regulatory-return ReturnProduct "${serial}"
  assert_unit_state regulatory-return "${serial}" DEVUELTO

  register_fixture regulatory-final "${final_serial}" "${FUTURE_EXPIRY}"
  regulatory_event regulatory-final-quarantine Quarantine "${final_serial}"
  regulatory_event regulatory-final FinalDisposition "${final_serial}"
  assert_unit_state regulatory-final "${final_serial}" DISPUESTO_FINAL
}

authorize_lab() {
  local label="$1"
  local serial="$2"
  local operation="$3"
  local expires_at="$4"
  local reason="$5"
  local request ctor
  request="$(authorize_request "${serial}" "${operation}" "${expires_at}" "${reason}")"
  ctor="$(request_ctor AuthorizeLabIntervention "${request}")"
  expect_valid_with_block "${label}" AnmatMSP "${ctor}" "" AnmatMSP
  assert_block_sbe "${label}" AnmatMSP
  decode_regulatory_marker "${label}"
}

lab_operation_matrix() {
  local label="$1"
  local function="$2"
  local serial="$3"
  local expected_state="$4"
  local request ctor
  request="$(unit_event_request "${serial}" "Intervencion ${function} de evidencia NET-9")"
  ctor="$(request_ctor "${function}" "${request}")"

  expect_platform_rejection "${label}-missing-lab" LabMSP "${ctor}" "" AnmatMSP DrogueriaMSP
  expect_platform_rejection "${label}-missing-regulator" LabMSP "${ctor}" "" LabMSP DrogueriaMSP
  expect_platform_rejection "${label}-missing-custodian" LabMSP "${ctor}" "" LabMSP AnmatMSP
  expect_valid_with_block "${label}" LabMSP "${ctor}" "" LabMSP AnmatMSP DrogueriaMSP
  decode_lab_markers "${label}"
  assert_unit_state "${label}" "${serial}" "${expected_state}"
}

wait_until_expired() {
  local expires_at="$1"
  local expires_epoch now
  expires_epoch="$(date -u -d "${expires_at}" +%s)"
  for _ in {1..60}; do
    now="$(date -u +%s)"
    if [[ "${now}" -gt "${expires_epoch}" ]]; then
      return
    fi
    sleep 1
  done
  fail "short LabIntervention did not expire within the bounded wait"
}

verify_lab_intervention() {
  local serial="N9LI${RUN_TOKEN}"
  local short_expiry long_expiry request ctor
  long_expiry="2035-12-31T23:59:59Z"

  move_fixture_to_drugstore lab-intervention "${serial}"

  short_expiry="$(date -u -d '+20 seconds' +%Y-%m-%dT%H:%M:%SZ)"
  authorize_lab lab-authorize-expired "${serial}" WITHDRAW_FROM_MARKET "${short_expiry}" "Autorizacion breve para demostrar vencimiento derivado"
  wait_until_expired "${short_expiry}"

  request="$(unit_event_request "${serial}" "Intento con autorizacion vencida")"
  ctor="$(request_ctor WithdrawFromMarket "${request}")"
  expect_logic_rejection lab-expired-operation LAB_INTERVENTION_REQUIRED LabMSP "${ctor}" "" LabMSP AnmatMSP DrogueriaMSP
  query_lab_history lab-history-expired.json "${serial}"
  jq -e '
    length == 1
    and .[0].isDelete == false
    and .[0].value.estado == "ACTIVA"
  ' "${RUN_DIR}/lab-history-expired.json" >/dev/null || fail "expired authorization created an unexpected history entry"

  authorize_lab lab-authorize-after-expiry "${serial}" WITHDRAW_FROM_MARKET "${long_expiry}" "Reemplazo posterior al vencimiento sin endoso del laboratorio anterior"

  request="$(revoke_request "${serial}" "Revocacion controlada de evidencia NET-9")"
  ctor="$(request_ctor RevokeLabIntervention "${request}")"
  expect_valid_with_block lab-revoke AnmatMSP "${ctor}" "" AnmatMSP
  decode_regulatory_marker lab-revoke

  authorize_lab lab-authorize-after-revoke "${serial}" WITHDRAW_FROM_MARKET "${long_expiry}" "Reemplazo posterior a revocacion sin endoso del laboratorio anterior"
  authorize_lab lab-authorize-replacement "${serial}" WITHDRAW_FROM_MARKET "${long_expiry}" "Reemplazo de autorizacion activa para demostrar clave unica"
  lab_operation_matrix lab-withdraw WithdrawFromMarket "${serial}" RETIRADO_MERCADO

  authorize_lab lab-authorize-restock "${serial}" RESTOCK "${long_expiry}" "Autorizacion de reingreso para laboratorio no custodio"
  lab_operation_matrix lab-restock Restock "${serial}" EN_CUSTODIA

  request="$(unit_event_request "${serial}" "Retiro regulatorio previo a disposicion final")"
  ctor="$(request_ctor WithdrawFromMarket "${request}")"
  expect_valid_with_block lab-prep-regulatory-withdraw AnmatMSP "${ctor}" "" AnmatMSP DrogueriaMSP
  decode_regulatory_marker lab-prep-regulatory-withdraw
  assert_unit_state lab-prep-regulatory-withdraw "${serial}" RETIRADO_MERCADO

  authorize_lab lab-authorize-final "${serial}" FINAL_DISPOSITION "${long_expiry}" "Autorizacion de disposicion final para laboratorio no custodio"
  lab_operation_matrix lab-final FinalDisposition "${serial}" DISPUESTO_FINAL

  query_lab_history lab-history-final.json "${serial}"
  sanitize_captured_block lab-withdraw _implicit_org_LabMSP lab-withdraw-lab-marker-sanitized.json '{"operacion":"WithdrawFromMarket","mspId":"LabMSP","timestamp":"'
  sanitize_captured_block lab-withdraw _implicit_org_AnmatMSP lab-withdraw-regulator-marker-sanitized.json '{"operacion":"WithdrawFromMarket","mspId":"LabMSP","timestamp":"'
}

verify_net6_prerequisite() {
  local -a results=()
  local run_dir
  mapfile -t results < <(find "${EVIDENCE_DIR}/net-6" -mindepth 2 -maxdepth 2 -type f -name result.json)
  [[ "${#results[@]}" -eq 1 ]] || fail "NET-9 requires exactly one verified NET-6 evidence run on this clean ledger"
  run_dir="$(dirname "${results[0]}")"
  python3 "${NETWORK_DIR}/scripts/verify-net6-evidence.py" run --directory "${run_dir}" --manifest "${run_dir}/artifacts.json" --channel "${CHANNEL_NAME}" >/dev/null
}

write_result() {
  local commit package_id
  commit="$(git -C "${REPOSITORY_ROOT}" rev-parse HEAD)"
  read_package_lock
  package_id="${LOCK_PACKAGE_ID}"
  jq -n --arg schemaVersion "1.0.0" --arg runToken "${RUN_TOKEN}" --arg commit "${commit}" --arg channel "${CHANNEL_NAME}" --arg chaincode "${CHAINCODE_NAME}" --arg packageID "${package_id}" '
    {
      schemaVersion:$schemaVersion,
      runToken:$runToken,
      repositoryCommit:$commit,
      artifactManifest:"artifacts.json",
      channel:$channel,
      chaincode:$chaincode,
      packageID:$packageID,
      assertions:{
        transitRoutesT09T13T14T15T16Verified:true,
        incompleteTransitEndorsementsRejected:true,
        restingSBERestoredToSender:true,
        activeTransferClosedAndHistoricalWritten:true,
        regulatoryMarkersVerifiedForEveryExtraordinaryOperation:true,
        missingRegulatorRejectedByPlatform:true,
        missingCustodianRejectedByPlatform:true,
        labAndRegulatorMarkersVerified:true,
        labRegulatorCustodianEndorsementsRequired:true,
        activeDerivedExpiredConsumedRevokedAndReplacedVerified:true,
        replacementAfterExpiryRequiresOnlyRegulator:true,
        replacementAfterRevocationRequiresOnlyRegulator:true,
        labInterventionHistoryVerified:true,
        platformAndLogicRejectionsSeparated:true,
        privatePayloadAbsentFromSanitizedEvidence:true,
        net6EvidenceReusedWithoutChaincodeRuleDuplication:true
      }
    }
  ' >"${RUN_DIR}/result.json"
}

main_net9() {
  require_command jq
  require_command python3
  require_command configtxlator
  require_command base64
  require_command git
  require_command sort
  require_command sed
  require_command date
  resolve_fabric_environment
  validate_sources
  ensure_network_running
  prepare_run_directory
  set_primary_orderer
  build_all_peer_targets
  verify_channel
  verify_lifecycle
  verify_net6_prerequisite
  write_run_metadata

  verify_extraordinary_implementations
  verify_transit_closures
  verify_regulatory_events
  verify_lab_intervention

  python3 "${EVIDENCE_VALIDATOR}" run --directory "${RUN_DIR}" --channel "${CHANNEL_NAME}" >"${RUN_DIR}/artifacts.json"
  write_result
  python3 "${EVIDENCE_VALIDATOR}" run --directory "${RUN_DIR}" --manifest "${RUN_DIR}/artifacts.json" --channel "${CHANNEL_NAME}" >/dev/null

  printf 'OK: NET-9 extraordinary endorsement evidence completed under %s\n' "${RUN_DIR}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main_net9 "$@"
fi
