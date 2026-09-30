#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=test/integration/pdc-evidence.sh
source "${REPOSITORY_ROOT}/test/integration/pdc-evidence.sh"

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
trap 'rm -rf -- "${TEST_DIR}"' EXIT

sleep() {
  return 0
}

attempt_file="${TEST_DIR}/attempt-count.txt"
evidence_file="${TEST_DIR}/retry-evidence.txt"

invoke_probe() {
  local attempt=0
  if [[ -f "${attempt_file}" ]]; then
    read -r attempt <"${attempt_file}"
  fi
  attempt=$((attempt + 1))
  printf '%d\n' "${attempt}" >"${attempt_file}"
  if [[ "${attempt}" -lt 3 ]]; then
    printf 'failed to distribute private collection: Failed disseminating 1 out of 2 private dissemination plans\n' >&2
    return 1
  fi
  printf 'dispatch committed\n'
}

output="$(invoke_probe_with_dissemination_retry TestMSP '{}' private "${evidence_file}")"
[[ "${output}" == "dispatch committed" ]]
[[ "$(<"${attempt_file}")" == "3" ]]
grep -q '^attempt=1 status=1$' "${evidence_file}"
grep -q '^attempt=3 status=0$' "${evidence_file}"

invoke_probe() {
  printf 'fatal endorsement error\n' >&2
  return 42
}

set +e
output="$(invoke_probe_with_dissemination_retry TestMSP '{}' private "${evidence_file}" 2>&1)"
status=$?
set -e
[[ "${status}" -eq 42 ]]
[[ "${output}" == "fatal endorsement error" ]]
grep -q '^attempt=1 status=42$' "${evidence_file}"
if grep -q '^attempt=2 ' "${evidence_file}"; then
  printf 'non-dissemination failure was retried unexpectedly\n' >&2
  exit 1
fi

printf 'OK: bounded dissemination retry and diagnostic persistence verified\n'
