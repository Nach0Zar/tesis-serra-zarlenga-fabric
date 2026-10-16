#!/usr/bin/env bash
# Helpers compartidos por los arneses de evidencia NET-6 y NET-9. Este archivo
# no define escenarios ni reglas del chaincode: solamente encapsula invocacion,
# captura exacta de transacciones y decodificacion de evidencia de plataforma.

declare -a TARGET_ARGS=()
LAST_OUTPUT=""
LAST_STATUS=0
LAST_OBSERVER_MSP="AnmatMSP"

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
  jq -cn --arg function "${function}" --arg request "${request}" \
    '{function:$function,Args:[$request]}'
}

read_ctor() {
  local function="$1"
  local gtin="$2"
  local serial="$3"
  jq -cn --arg function "${function}" --arg gtin "${gtin}" --arg serial "${serial}" \
    '{function:$function,Args:[$gtin,$serial]}'
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
  [[ "$#" -gt 0 ]] || fail "invoke requires at least one target endorser"
  # Consultar después un peer cuyo evento de commit fue esperado evita depender
  # del índice de txID de una organización ajena a los endorsers del intento.
  LAST_OBSERVER_MSP="$1"
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

prepare_run_directory() {
  # mkdir sin -p para el directorio de corrida: nunca mezclar ni borrar
  # evidencia de una ejecucion anterior.
  mkdir -p "${EVIDENCE_ROOT}"
  mkdir "${RUN_DIR}" || fail "run directory already exists; choose a new evidence run token"
}

last_transaction_id() {
  local -a tx_ids=()
  mapfile -t tx_ids < <(sed -nE 's/.*txid \[([0-9a-f]{64})\].*/\1/p' <<<"${LAST_OUTPUT}" | sort -u)
  [[ "${#tx_ids[@]}" -eq 1 ]] || fail "invocation did not report exactly one transaction ID"
  printf '%s\n' "${tx_ids[0]}"
}

record_last_transaction_id() {
  local label="$1"
  last_transaction_id >"${RUN_DIR}/${label}-txid.txt"
}

capture_block() {
  local label="$1"
  local expected_code="$2"
  local raw="${RUN_DIR}/${label}-block.pb"
  local decoded="${RUN_DIR}/${label}-block.json"
  local tx_id fetched=false
  tx_id="$(last_transaction_id)"
  printf '%s\n' "${tx_id}" >"${RUN_DIR}/${label}-txid.txt"
  select_identity "${LAST_OBSERVER_MSP}" Admin
  : >"${RUN_DIR}/${label}-block-fetch.txt"
  # El peer observador puede estar atrasado: esperar ESTA transaccion, no una altura.
  for _ in {1..30}; do
    if fetch_peer_block_from_ledger "${tx_id}" "${raw}" "${RUN_DIR}/${label}-block-fetch.txt" GetBlockByTxID; then
      fetched=true
      break
    fi
    sleep 1
  done
  [[ "${fetched}" == true ]] || fail "${label} transaction was not available on the observer peer"
  # La consulta exitosa no escribe salida diagnostica. Evitar que un archivo
  # vacio se confunda con evidencia incompleta; si hubo reintentos, conservar
  # sus mensajes para auditoria.
  [[ -s "${RUN_DIR}/${label}-block-fetch.txt" ]] \
    || rm -- "${RUN_DIR}/${label}-block-fetch.txt"
  configtxlator proto_decode --input "${raw}" --type common.Block --output "${decoded}"
  python3 "${EVIDENCE_VALIDATOR}" transaction \
    --block "${decoded}" --txid "${tx_id}" --code "${expected_code}" --channel "${CHANNEL_NAME}" \
    >"${RUN_DIR}/${label}-transaction.json"
}

captured_block_json() {
  local block="${RUN_DIR}/$1-block.json"
  [[ -f "${block}" ]] || fail "decoded block is missing for $1"
  printf '%s\n' "${block}"
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
    --transaction-id "$(<"${RUN_DIR}/${label}-txid.txt")" \
    --output "${RUN_DIR}/${output_name}" \
    --forbidden-value "${forbidden_value}"
}

decode_hashed_rwset() {
  local label="$1"
  local collection="$2"
  local output_name="$3"
  local decoded encoded raw
  decoded="$(captured_block_json "${label}")"
  raw="${RUN_DIR}/${output_name%.json}.pb"
  encoded="$(jq -er --arg namespace "${CHAINCODE_NAME}" \
    --arg collection "${collection}" \
    --arg txid "$(<"${RUN_DIR}/${label}-txid.txt")" '
    [
      .data.data[]?
      | select(.payload.header.channel_header.tx_id == $txid)
      | .payload.data.actions[]?.payload.action.proposal_response_payload.extension.results.ns_rwset[]?
      | select(.namespace == $namespace)
      | .collection_hashed_rwset[]?
      | select(.collection_name == $collection)
      | .hashed_rwset
    ]
    | if length == 1 then .[0] else error("expected exactly one hashed rwset") end
  ' "${decoded}")"
  printf '%s' "${encoded}" | base64 --decode >"${raw}"
  python3 "${EVIDENCE_VALIDATOR}" hashed-rwset \
    --input "${raw}" \
    --output "${RUN_DIR}/${output_name}"
}

assert_block_sbe() {
  local label="$1"
  shift
  local decoded policy_value policy_pb policy_json expected_sorted actual_sorted
  local -a expected=("$@")
  local -a actual=()
  [[ "${#expected[@]}" -gt 0 ]] || fail "assert_block_sbe requires at least one MSP"
  decoded="$(captured_block_json "${label}")"
  policy_pb="${RUN_DIR}/${label}-sbe.pb"
  policy_json="${RUN_DIR}/${label}-sbe.json"
  policy_value="$(jq -er --arg namespace "${CHAINCODE_NAME}" \
    --arg txid "$(<"${RUN_DIR}/${label}-txid.txt")" '
    [
      .data.data[]?
      | select(.payload.header.channel_header.tx_id == $txid)
      | .payload.data.actions[]?.payload.action.proposal_response_payload.extension.results.ns_rwset[]?
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
  mapfile -t actual < <(jq -er '.identities[].principal.msp_identifier' "${policy_json}")
  jq -e 'all(.identities[];
    .principal_classification == "ROLE" and .principal.role == "PEER")' "${policy_json}" >/dev/null \
    || fail "${label} SBE contains a principal other than an MSP peer role"
  [[ "${#actual[@]}" -eq "${#expected[@]}" ]] \
    || fail "${label} SBE has an unexpected number of principals"
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
  expect_valid "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  capture_block "${label}" 0
}

expect_platform_rejection() {
  local label="$1"
  local creator_msp="$2"
  local ctor="$3"
  local transient="$4"
  shift 4
  run_invoke "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  [[ "${LAST_STATUS}" -ne 0 ]] || fail "${label} unexpectedly committed"
  grep -q 'ENDORSEMENT_POLICY_FAILURE' <<<"${LAST_OUTPUT}" \
    || fail "${label} did not report ENDORSEMENT_POLICY_FAILURE"
  capture_block "${label}" 10
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
  before="$(network_channel_height)"
  run_invoke "${label}" "${creator_msp}" "${ctor}" "${transient}" "$@"
  [[ "${LAST_STATUS}" -ne 0 ]] || fail "${label} unexpectedly committed"
  grep -q "${expected_code}" <<<"${LAST_OUTPUT}" \
    || fail "${label} did not return application code ${expected_code}"
  sleep 2
  after="$(network_channel_height)"
  [[ "${after}" -eq "${before}" ]] \
    || fail "${label} reached the ledger despite being rejected during proposal simulation"
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
    fail "${operation} is still a chaincode stub; merge the required implementation issue first"
  fi
  grep -q "${expected_code}" <<<"${output}" \
    || fail "${operation} preflight did not return ${expected_code}"
}

assert_unit_state() {
  local label="$1"
  local serial="$2"
  local expected_state="$3"
  local ctor output
  # GTIN is a required global supplied by each suite before this file is sourced.
  # shellcheck disable=SC2153
  ctor="$(read_ctor ReadUnit "${GTIN}" "${serial}")"
  select_identity AnmatMSP User1
  output="$(peer chaincode query --channelID "${CHANNEL_NAME}" --name "${CHAINCODE_NAME}" --ctor "${ctor}")"
  printf '%s\n' "${output}" >"${RUN_DIR}/${label}-unit.json"
  jq -e --arg state "${expected_state}" '.estado == $state' <<<"${output}" >/dev/null \
    || fail "${label} expected state ${expected_state}"
}
