#!/usr/bin/env bash
# Regresiones del harness sin Docker: peer observador atrasado, tx exacta y token.
set -Eeuo pipefail
REPOSITORY_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "${TEST_DIR}"' EXIT
export SNT_EVIDENCE_DIR="${TEST_DIR}"
export SNT_NET6_RUN_TOKEN="regression"
# shellcheck source=test/integration/endorsement-evidence.sh
source "${REPOSITORY_ROOT}/test/integration/endorsement-evidence.sh"

prepare_run_directory
printf 'preserve\n' >"${RUN_DIR}/sentinel"
if (prepare_run_directory) 2>/dev/null; then
  fail "existing run directory was accepted"
fi
[[ "$(<"${RUN_DIR}/sentinel")" == preserve ]]

select_identity() { :; }
sleep() { :; }
attempts=0
txid="$(printf '%064d' 1)"
other="$(printf '%064d' 2)"
LAST_OUTPUT="txid [${txid}] committed with status (ENDORSEMENT_POLICY_FAILURE)
txid [${txid}] committed with status (ENDORSEMENT_POLICY_FAILURE)"
fetch_peer_block_from_ledger() {
  [[ "$1" == "${txid}" && "$4" == GetBlockByTxID ]] || fail "wrong QSCC query"
  attempts=$((attempts + 1))
  if [[ "${attempts}" -lt 3 ]]; then
    return 1
  fi
  jq -n --arg txid "${txid}" --arg other "${other}" '{
    header:{number:"42"},metadata:{metadata:["","","AAo="]},
    data:{data:[$other,$txid] | map({payload:{header:{channel_header:{tx_id:.,channel_id:"snt-channel"}}}})}
  }' >"$2"
}
configtxlator() {
  [[ "$1" == proto_decode && "$2" == --input && "$6" == --output ]]
  cp -- "$3" "$7"
}
capture_block observer-lag 10
[[ "${attempts}" -eq 3 ]]
jq -e '.transactionIndex == 1 and .validationCode == 10 and .blockNumber == 42' \
  "${RUN_DIR}/observer-lag-transaction.json" >/dev/null

# Un 10 de OTRA transaccion no demuestra que la nuestra sea invalida.
jq '.metadata.metadata[2] = "CgA="' "${RUN_DIR}/observer-lag-block.json" >"${TEST_DIR}/wrong-code.json"
if python3 "${NETWORK_DIR}/scripts/verify-net6-evidence.py" transaction \
  --block "${TEST_DIR}/wrong-code.json" --txid "${txid}" --code 10 >/dev/null 2>&1; then
  fail "accepted another transaction's validation code"
fi

LAST_OUTPUT="txid [${txid}] committed
txid [${other}] committed"
if (capture_block ambiguous 10) 2>/dev/null; then
  fail "ambiguous transaction IDs were accepted"
fi
LAST_OUTPUT="txid [${txid}] committed"
fetch_peer_block_from_ledger() { return 1; }
if (capture_block never-confirmed 10) 2>/dev/null; then
  fail "unconfirmed transaction was accepted"
fi
printf 'OK: observer lag, exact transaction, bounded retry and run isolation verified\n'
