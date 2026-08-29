#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
NETWORK_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly NETWORK_DIR
readonly MANIFEST="${SNT_ORGANIZATIONS_MANIFEST:-${NETWORK_DIR}/organizations-manifest.json}"
readonly SCHEMA="${SNT_ORGANIZATIONS_SCHEMA:-${NETWORK_DIR}/organizations-manifest.schema.json}"
readonly CONFIGTX="${SNT_CONFIGTX_FILE:-${NETWORK_DIR}/configtx.yaml}"
readonly OUTPUT_DIR_INPUT="${SNT_CRYPTO_OUTPUT_DIR:-${NETWORK_DIR}/organizations}"

(( $# == 0 )) || {
  printf 'Usage: %s\n' "$0" >&2
  exit 2
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

for required in jq openssl python3 sha256sum realpath awk find grep mktemp rm sort stat tr wc; do
  command -v "$required" >/dev/null 2>&1 || fail "required command not found: $required"
done

python3 "$SCRIPT_DIR/validate-organizations-manifest.py"   --manifest "$MANIFEST"   --schema "$SCHEMA"   --configtx "$CONFIGTX"

[[ -d "$OUTPUT_DIR_INPUT" ]] || fail "generated output does not exist: $OUTPUT_DIR_INPUT"
OUTPUT_DIR="$(realpath -- "$OUTPUT_DIR_INPUT")"
readonly OUTPUT_DIR
readonly MANIFEST_HASH_FILE="$OUTPUT_DIR/.state/manifest.sha256"
readonly SECRETS_FILE="$OUTPUT_DIR/.state/secrets.env"
CURRENT_MANIFEST_HASH="$(sha256sum "$MANIFEST" | awk '{print $1}')"
readonly CURRENT_MANIFEST_HASH

single_file() {
  local directory="$1"
  local files=()
  mapfile -d '' files < <(find "$directory" -maxdepth 1 -type f -print0)
  (( ${#files[@]} == 1 )) || fail "expected exactly one file in $directory, found ${#files[@]}"
  printf '%s' "${files[0]}"
}

certificate_has_ou() {
  local certificate="$1"
  local expected_ou="$2"
  openssl x509 -in "$certificate" -noout -subject -nameopt RFC2253 \
    | grep -F "OU=$expected_ou" >/dev/null
}

verify_node_ous() {
  local label="$1"
  local config_file="$2"
  local expected_ou
  [[ -f "$config_file" ]] || fail "$label is missing config.yaml"
  grep -F "Enable: true" "$config_file" >/dev/null \
    || fail "$label does not enable NodeOUs"
  for expected_ou in client peer admin orderer; do
    grep -F "OrganizationalUnitIdentifier: $expected_ou" "$config_file" >/dev/null \
      || fail "$label does not define the $expected_ou NodeOU"
  done
}

assert_no_snt_role() {
  local label="$1"
  local certificate="$2"
  if openssl x509 -in "$certificate" -noout -text | grep -F '"snt.role"' >/dev/null; then
    fail "$label certificate must not contain snt.role"
  fi
}

verify_msp_identity() {
  local label="$1"
  local msp_dir="$2"
  local ca_certificate="$3"
  local expected_ou="$4"
  verify_node_ous "$label" "$msp_dir/config.yaml"
  local sign_certificate private_key
  sign_certificate="$(single_file "$msp_dir/signcerts")"
  private_key="$(single_file "$msp_dir/keystore")"
  [[ -s "$private_key" ]] || fail "$label has an empty private key"
  openssl verify -CAfile "$ca_certificate" "$sign_certificate" >/dev/null     || fail "$label enrollment certificate does not verify"
  certificate_has_ou "$sign_certificate" "$expected_ou"     || fail "$label certificate does not contain OU=$expected_ou"
  printf '%s' "$sign_certificate"
}

verify_tls_identity() {
  local label="$1"
  local tls_dir="$2"
  local hostname="$3"
  [[ -s "$tls_dir/ca.crt" ]] || fail "$label is missing TLS CA certificate"
  [[ -s "$tls_dir/server.crt" ]] || fail "$label is missing TLS server certificate"
  [[ -s "$tls_dir/server.key" ]] || fail "$label is missing TLS private key"
  openssl verify -CAfile "$tls_dir/ca.crt" "$tls_dir/server.crt" >/dev/null     || fail "$label TLS certificate does not verify"
  openssl x509 -in "$tls_dir/server.crt" -noout -ext subjectAltName     | grep -F "DNS:$hostname" >/dev/null     || fail "$label TLS certificate does not include SAN DNS:$hostname"
}

[[ -f "$MANIFEST_HASH_FILE" ]] \
  || fail "manifest checksum is missing: $MANIFEST_HASH_FILE"
stored_manifest_hash="$(tr -d '[:space:]' < "$MANIFEST_HASH_FILE")"
[[ "$stored_manifest_hash" == "$CURRENT_MANIFEST_HASH" ]] \
  || fail "stored manifest checksum does not match $MANIFEST"

[[ -f "$SECRETS_FILE" ]] || fail "generated secrets file is missing"
[[ "$(stat -c '%a' "$SECRETS_FILE")" == "600" ]]   || fail "generated secrets file must have mode 0600"

fingerprints_file="$(mktemp)"
trap 'rm -f -- "$fingerprints_file"' EXIT
expected_orderer_count=0

while IFS=$'\t' read -r msp_id slug client_role peer_hostname orderer_hostname; do
  org_dir="$OUTPUT_DIR/$slug"
  org_msp="$org_dir/msp"
  org_ca="$org_msp/cacerts/ca.crt"
  org_tls_ca="$org_msp/tlscacerts/tlsca.crt"
  verify_node_ous "$msp_id organization MSP" "$org_msp/config.yaml"
  [[ -s "$org_ca" ]] || fail "$msp_id organization MSP is missing its CA root"
  [[ -s "$org_tls_ca" ]] || fail "$msp_id organization MSP is missing its TLS root"
  openssl x509 -in "$org_ca" -noout -fingerprint -sha256     | awk -F= '{print $2}' >> "$fingerprints_file"

  peer_dir="$org_dir/peers/$peer_hostname"
  peer_certificate="$(verify_msp_identity "$msp_id peer" "$peer_dir/msp" "$org_ca" peer)"
  assert_no_snt_role "$msp_id peer" "$peer_certificate"
  verify_tls_identity "$msp_id peer" "$peer_dir/tls" "$peer_hostname"

  admin_msp="$org_dir/users/Admin@$slug.snt.local/msp"
  admin_certificate="$(verify_msp_identity "$msp_id admin" "$admin_msp" "$org_ca" admin)"
  assert_no_snt_role "$msp_id admin" "$admin_certificate"

  user_msp="$org_dir/users/User1@$slug.snt.local/msp"
  user_certificate="$(verify_msp_identity "$msp_id user" "$user_msp" "$org_ca" client)"
  openssl x509 -in "$user_certificate" -noout -text     | grep -F "\"snt.role\":\"$client_role\"" >/dev/null     || fail "$msp_id user certificate does not contain snt.role=$client_role"

  if [[ -n "$orderer_hostname" ]]; then
    expected_orderer_count=$((expected_orderer_count + 1))
    orderer_dir="$org_dir/orderers/$orderer_hostname"
    orderer_certificate="$(verify_msp_identity "$msp_id orderer" "$orderer_dir/msp" "$org_ca" orderer)"
    assert_no_snt_role "$msp_id orderer" "$orderer_certificate"
    verify_tls_identity "$msp_id orderer" "$orderer_dir/tls" "$orderer_hostname"
  elif [[ -d "$org_dir/orderers" ]]       && [[ -n "$(find "$org_dir/orderers" -mindepth 1 -maxdepth 1 -type d -print -quit)" ]]; then
    fail "$msp_id has unexpected orderer material"
  fi
done < <(
  jq -r '.organizations[] | [.mspId, .slug, .clientRole, .peerHostname, (.ordererHostname // "")] | @tsv' "$MANIFEST"
)

organization_count="$(jq '.organizations | length' "$MANIFEST")"
unique_fingerprints="$(sort -u "$fingerprints_file" | wc -l | tr -d '[:space:]')"
[[ "$unique_fingerprints" == "$organization_count" ]]   || fail "expected $organization_count distinct organization CA roots, found $unique_fingerprints"

actual_orderer_count="$(
  find "$OUTPUT_DIR" -path '*/orderers/*/tls/server.crt' -type f | wc -l | tr -d '[:space:]'
)"
[[ "$actual_orderer_count" == "$expected_orderer_count" ]]   || fail "expected $expected_orderer_count orderers, found $actual_orderer_count"

printf 'OK: verified %s independent organization MSPs and %s orderers\n'   "$organization_count" "$expected_orderer_count"
