#!/usr/bin/env bash
# Probe deliberadamente aislado para demostrar políticas y privacidad de
# colecciones sin duplicar reglas del contrato snt. La evidencia productiva de
# DispatchTransfer que complementa este probe vive en
# test/integration/endorsement-evidence.sh.
set -Eeuo pipefail

REPOSITORY_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=network/network.sh
source "${REPOSITORY_ROOT}/network/network.sh"

readonly PROBE_NAME="pdc-probe"
readonly PROBE_VERSION="1.0"
readonly PROBE_LABEL="pdc_probe_1.0"
readonly PROBE_SOURCE="${REPOSITORY_ROOT}/test/fixtures/pdc-probe"
readonly PROBE_BUILD="${REPOSITORY_ROOT}/build/test/pdc-probe"
readonly PROBE_STAGE="${PROBE_BUILD}/stage"
readonly PROBE_PACKAGE="${PROBE_BUILD}/${PROBE_LABEL}.tar.gz"
readonly PROBE_EVIDENCE="${EVIDENCE_DIR}/net-5"
readonly EXPLICIT_COLLECTION="transfer_DrogueriaMSP_FarmaciaMSP"
readonly PUBLIC_KEY="probe-dispatch-net5"
readonly PUBLIC_VALUE='{"operationId":"probe-dispatch-net5","status":"DISPATCHED"}'
readonly PRIVATE_VALUE='private-commercial-net5'

PROBE_PACKAGE_ID=""

select_organization() {
  local wanted_msp="$1"
  local identity="${2:-Admin}"
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

build_probe_package() {
  require_command go
  [[ "${PROBE_BUILD}" == "${REPOSITORY_ROOT}/build/"* ]] || fail "unsafe probe build path"
  rm -rf -- "${PROBE_BUILD}"
  mkdir -p "${PROBE_STAGE}"
  cp -- "${PROBE_SOURCE}/go.mod" "${PROBE_SOURCE}/go.sum" "${PROBE_SOURCE}/main.go" "${PROBE_STAGE}/"
  (cd "${PROBE_SOURCE}" && go mod vendor -o "${PROBE_STAGE}/vendor")
  (cd "${PROBE_STAGE}" && go build -mod=vendor ./...)
  peer lifecycle chaincode package "${PROBE_PACKAGE}" --path "${PROBE_STAGE}" --lang golang --label "${PROBE_LABEL}"
  PROBE_PACKAGE_ID="$(peer lifecycle chaincode calculatepackageid "${PROBE_PACKAGE}")"
  printf '%s\n' "${PROBE_PACKAGE_ID}" >"${PROBE_EVIDENCE}/probe-package-id.txt"
}

probe_committed() {
  local output approved
  select_organization AnmatMSP Admin
  output="$(peer lifecycle chaincode querycommitted --channelID "${CHANNEL_NAME}" --name "${PROBE_NAME}" --output json 2>/dev/null)" || return 1
  jq -e --arg version "${PROBE_VERSION}" '.sequence == 1 and .version == $version and (.init_required // false) == false' <<<"${output}" >/dev/null || return 1
  approved="$(peer lifecycle chaincode queryapproved --channelID "${CHANNEL_NAME}" --name "${PROBE_NAME}" --sequence 1 --output json)"
  document_package_id_matches "${approved}" "${PROBE_PACKAGE_ID}" || fail "committed probe uses a different package; reset the disposable test ledger"
}

install_probe() {
  local msp_id slug peer_hostname _rest
  local installed
  while IFS=$'\t' read -r msp_id slug peer_hostname _rest; do
    use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
    installed="$(peer lifecycle chaincode queryinstalled --output json)"
    if ! jq -e --arg package "${PROBE_PACKAGE_ID}" 'any(.installed_chaincodes[]?; .package_id == $package)' <<<"${installed}" >/dev/null; then
      peer lifecycle chaincode install "${PROBE_PACKAGE}"
      installed="$(peer lifecycle chaincode queryinstalled --output json)"
    fi
    jq -e --arg package "${PROBE_PACKAGE_ID}" 'any(.installed_chaincodes[]?; .package_id == $package)' <<<"${installed}" >/dev/null || fail "probe package missing on ${peer_hostname}"
  done < <(organization_rows)
}

approve_and_commit_probe() {
  local policy
  local msp_id slug peer_hostname agent_type id id_type active orderer_hostname
  local expected_count
  local -a flags
  policy="$(operational_policy)"
  flags=(
    --channelID "${CHANNEL_NAME}"
    --name "${PROBE_NAME}"
    --version "${PROBE_VERSION}"
    --sequence 1
    --signature-policy "${policy}"
    --collections-config "${COLLECTIONS_CONFIG}"
  )
  while IFS=$'\t' read -r msp_id slug peer_hostname agent_type id id_type active orderer_hostname; do
    use_organization "${msp_id}" "${slug}" "${peer_hostname}" Admin
    peer lifecycle chaincode approveformyorg "${ORDERER_ARGS[@]}" "${flags[@]}" --package-id "${PROBE_PACKAGE_ID}"
  done < <(organization_rows)
  select_organization AnmatMSP Admin
  peer lifecycle chaincode checkcommitreadiness "${flags[@]}" --output json >"${PROBE_EVIDENCE}/probe-readiness.json"
  expected_count="$(jq '.organizations | length' "${MANIFEST}")"
  jq -e --argjson expected "${expected_count}" '(.approvals | length) == $expected and all(.approvals[]; . == true)' "${PROBE_EVIDENCE}/probe-readiness.json" >/dev/null || fail "probe approvals are incomplete"
  peer lifecycle chaincode commit "${ORDERER_ARGS[@]}" "${flags[@]}" "${ALL_PEER_TARGETS[@]}"
  probe_committed || fail "pdc-probe definition was not committed"
}

deploy_probe() {
  build_probe_package
  if probe_committed; then
    info "pdc-probe is already committed"
    return
  fi
  info "Installing the test-only PDC probe on all peers"
  install_probe
  approve_and_commit_probe
}

probe_ctor() {
  local function="$1"
  shift
  jq -cn --arg function "${function}" --args '{function:$function,Args:$ARGS.positional}' -- "$@"
}

query_probe() {
  local msp_id="$1"
  local ctor="$2"
  select_organization "${msp_id}" User1
  peer chaincode query --channelID "${CHANNEL_NAME}" --name "${PROBE_NAME}" --ctor "${ctor}"
}

invoke_probe() {
  local msp_id="$1"
  local ctor="$2"
  local private_value="$3"
  local transient
  local -a args
  select_organization "${msp_id}" User1
  transient="$(jq -cn --arg value "${private_value}" '{private:($value|@base64)}')"
  args=(
    "${ORDERER_ARGS[@]}"
    --channelID "${CHANNEL_NAME}"
    --name "${PROBE_NAME}"
    --ctor "${ctor}"
    --transient "${transient}"
    --peerAddresses "${CORE_PEER_ADDRESS}"
    --tlsRootCertFiles "${CORE_PEER_TLS_ROOTCERT_FILE}"
    --waitForEvent
    --waitForEventTimeout "${SNT_COMMIT_TIMEOUT:-180s}"
  )
  peer chaincode invoke "${args[@]}"
}

channel_height() {
  local output
  select_organization DrogueriaMSP User1
  output="$(peer channel getinfo --channelID "${CHANNEL_NAME}")"
  jq -r '.height' <<<"${output#Blockchain info: }"
}

assert_query_value() {
  local msp_id="$1"
  local ctor="$2"
  local expected="$3"
  local output
  output="$(query_probe "${msp_id}" "${ctor}")"
  [[ "${output}" == "${expected}" ]] || fail "unexpected probe value from ${msp_id}: ${output}"
}

assert_query_rejected() {
  local msp_id="$1"
  local ctor="$2"
  local output status
  set +e
  output="$(query_probe "${msp_id}" "${ctor}" 2>&1)"
  status=$?
  set -e
  [[ "${status}" -ne 0 ]] || fail "${msp_id} unexpectedly read private data"
  printf '%s\n' "${output}"
}

restart_receiver() {
  compose start peer0.farmacia.snt.local >/dev/null 2>&1 || true
  compose up --detach --wait --wait-timeout "${SNT_START_TIMEOUT:-180}" >/dev/null 2>&1 || true
}

wait_for_receiver_private_data() {
  local ctor="$1"
  local output status
  for _ in {1..45}; do
    set +e
    output="$(query_probe FarmaciaMSP "${ctor}" 2>&1)"
    status=$?
    set -e
    if [[ "${status}" -eq 0 && "${output}" == "${PRIVATE_VALUE}" ]]; then
      printf '%s\n' "${output}" >"${PROBE_EVIDENCE}/receiver-reconciliation.txt"
      return
    fi
    sleep 2
  done
  fail "FarmaciaMSP did not reconcile private data after restart"
}

verify_explicit_collection() {
  local block_number dispatch_key
  local put_ctor public_ctor private_ctor output
  local raw_block decoded_block
  block_number="$(channel_height)"
  dispatch_key="${PUBLIC_KEY}-${block_number}"
  put_ctor="$(probe_ctor Put "${EXPLICIT_COLLECTION}" "${dispatch_key}" "${PUBLIC_VALUE}")"
  public_ctor="$(probe_ctor GetPublic "${dispatch_key}")"
  private_ctor="$(probe_ctor GetPrivate "${EXPLICIT_COLLECTION}" "${dispatch_key}")"

  info "Stopping the receiver to force private-data reconciliation"
  compose stop peer0.farmacia.snt.local >/dev/null
  trap restart_receiver EXIT
  output="$(invoke_probe DrogueriaMSP "${put_ctor}" "${PRIVATE_VALUE}" 2>&1)"
  printf '%s\n' "${output}" >"${PROBE_EVIDENCE}/explicit-dispatch.txt"
  restart_receiver
  trap - EXIT

  wait_for_receiver_private_data "${private_ctor}"
  assert_query_value DrogueriaMSP "${private_ctor}" "${PRIVATE_VALUE}"
  assert_query_value AnmatMSP "${private_ctor}" "${PRIVATE_VALUE}"
  assert_query_rejected DistribuidorMSP "${private_ctor}" >"${PROBE_EVIDENCE}/nonmember-read.txt"

  local msp_id _slug _peer_hostname _rest
  while IFS=$'\t' read -r msp_id _slug _peer_hostname _rest; do
    assert_query_value "${msp_id}" "${public_ctor}" "${PUBLIC_VALUE}"
  done < <(organization_rows)

  raw_block="${PROBE_EVIDENCE}/explicit-block-${block_number}.pb"
  decoded_block="${PROBE_EVIDENCE}/explicit-block-${block_number}.json"
  select_organization DrogueriaMSP Admin
  peer channel fetch "${block_number}" "${raw_block}" --channelID "${CHANNEL_NAME}" "${ORDERER_ARGS[@]}" >/dev/null
  configtxlator proto_decode --input "${raw_block}" --type common.Block --output "${decoded_block}"
  grep -q "${EXPLICIT_COLLECTION}" "${decoded_block}" || fail "decoded block does not expose the collection name"
  if grep -q "${PRIVATE_VALUE}" "${decoded_block}"; then
    fail "decoded block leaked the private payload"
  fi
  python3 "${NETWORK_DIR}/scripts/sanitize-pdc-evidence.py" \
    --input "${decoded_block}" \
    --collection "${EXPLICIT_COLLECTION}" \
    --forbidden-value "${PRIVATE_VALUE}" \
    --output "${PROBE_EVIDENCE}/sanitized-block-excerpt.json"
  printf '%s\n' "${block_number}" >"${PROBE_EVIDENCE}/explicit-block-number.txt"
}

verify_anmat_cannot_endorse_explicit_write() {
  local key ctor output status public_ctor public_output public_status
  key="probe-anmat-only-$(channel_height)"
  ctor="$(probe_ctor Put "${EXPLICIT_COLLECTION}" "${key}" '{"status":"INVALID"}')"
  set +e
  output="$(invoke_probe AnmatMSP "${ctor}" "anmat-only-private" 2>&1)"
  status=$?
  set -e
  printf '%s\n' "${output}" >"${PROBE_EVIDENCE}/anmat-only-write.txt"
  [[ "${status}" -ne 0 ]] || fail "ANMAT-only explicit write unexpectedly committed"
  grep -q 'ENDORSEMENT_POLICY_FAILURE' <<<"${output}" || fail "ANMAT-only write did not fail because of the collection endorsement policy"
  public_ctor="$(probe_ctor GetPublic "${key}")"
  set +e
  public_output="$(query_probe AnmatMSP "${public_ctor}" 2>&1)"
  public_status=$?
  set -e
  [[ "${public_status}" -ne 0 ]] || fail "invalid ANMAT-only transaction changed public state"
  printf '%s\n' "${public_output}" >"${PROBE_EVIDENCE}/anmat-only-public-check.txt"
}

verify_implicit_collections() {
  local owner_key invalid_key owner_ctor owner_query invalid_ctor
  local output status
  if grep -q '_implicit_org_' "${COLLECTIONS_CONFIG}"; then
    fail "implicit collections must not appear in collections_config.json"
  fi
  owner_key="implicit-owner-$(channel_height)"
  owner_ctor="$(probe_ctor PutImplicit DrogueriaMSP "${owner_key}")"
  owner_query="$(probe_ctor GetImplicit DrogueriaMSP "${owner_key}")"
  invoke_probe DrogueriaMSP "${owner_ctor}" "implicit-owner-value" >"${PROBE_EVIDENCE}/implicit-owner-write.txt" 2>&1
  assert_query_value DrogueriaMSP "${owner_query}" "implicit-owner-value"
  assert_query_rejected DistribuidorMSP "${owner_query}" >"${PROBE_EVIDENCE}/implicit-third-party-read.txt"

  invalid_key="implicit-nonowner-$(channel_height)"
  invalid_ctor="$(probe_ctor PutImplicit DrogueriaMSP "${invalid_key}")"
  set +e
  output="$(invoke_probe DistribuidorMSP "${invalid_ctor}" "implicit-invalid-value" 2>&1)"
  status=$?
  set -e
  printf '%s\n' "${output}" >"${PROBE_EVIDENCE}/implicit-nonowner-write.txt"
  [[ "${status}" -ne 0 ]] || fail "non-owner implicit write unexpectedly committed"
  grep -q 'ENDORSEMENT_POLICY_FAILURE' <<<"${output}" || fail "non-owner implicit write did not fail because of the implicit collection endorsement policy"
}

write_sanitized_result() {
  local block_number
  block_number="$(<"${PROBE_EVIDENCE}/explicit-block-number.txt")"
  jq -n --arg channel "${CHANNEL_NAME}" --arg probePackageID "${PROBE_PACKAGE_ID}" --arg collection "${EXPLICIT_COLLECTION}" --argjson blockNumber "${block_number}" '
      {
        channel: $channel,
        probePackageID: $probePackageID,
        explicitCollection: $collection,
        explicitBlockNumber: $blockNumber,
        publicReadableByAllOrganizations: true,
        privateReadableByPairAndRegulator: true,
        privateRejectedForNonMember: true,
        receiverReconciliationVerified: true,
        collectionNameVisibleInBlock: true,
        privatePayloadAbsentFromBlock: true,
        regulatorOnlyWriteRejected: true,
        implicitOwnerWriteVerified: true,
        implicitThirdPartyReadRejected: true,
        implicitNonOwnerWriteRejected: true
      }
    ' >"${PROBE_EVIDENCE}/result.json"
}

main() {
  require_command docker
  require_command jq
  resolve_fabric_environment
  require_command configtxlator
  validate_sources
  ensure_network_running
  mkdir -p "${PROBE_EVIDENCE}"
  set_primary_orderer
  build_all_peer_targets
  deploy_probe
  verify_explicit_collection
  verify_anmat_cannot_endorse_explicit_write
  verify_implicit_collections
  write_sanitized_result
  printf 'OK: NET-5 PDC evidence completed; raw artifacts are under %s\n' "${PROBE_EVIDENCE}"
}

main "$@"
