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
readonly CA_PORT="${SNT_CA_PORT:-7054}"
readonly REQUIRED_CA_VERSION="${SNT_FABRIC_CA_VERSION:-1.5.17}"

CA_PID=""

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '==> %s\n' "$*"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

fabric_ca_version() {
  "$1" version 2>&1 | awk '$1 == "Version:" { sub(/^v/, "", $2); print $2; exit }'
}

check_fabric_ca_version() {
  local command_name="$1"
  local actual_version
  actual_version="$(fabric_ca_version "$command_name")"
  [[ "$actual_version" == "$REQUIRED_CA_VERSION" ]] || fail     "$command_name version $REQUIRED_CA_VERSION is required; found ${actual_version:-unknown}"
}

cleanup() {
  if [[ -n "$CA_PID" ]] && kill -0 "$CA_PID" >/dev/null 2>&1; then
    kill "$CA_PID" >/dev/null 2>&1 || true
    wait "$CA_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

for required in bash jq openssl python3 sha256sum realpath awk basename cat find cp chmod mkdir mv rm seq sleep sort wc tr; do
  require_command "$required"
done
require_command fabric-ca-server
require_command fabric-ca-client
check_fabric_ca_version fabric-ca-server
check_fabric_ca_version fabric-ca-client

[[ "$CA_PORT" =~ ^[0-9]+$ ]] || fail "SNT_CA_PORT must be numeric"
(( CA_PORT >= 1024 && CA_PORT <= 65535 )) || fail "SNT_CA_PORT must be between 1024 and 65535"

python3 "$SCRIPT_DIR/validate-organizations-manifest.py"   --manifest "$MANIFEST"   --schema "$SCHEMA"   --configtx "$CONFIGTX"

mkdir -p -- "$OUTPUT_DIR_INPUT"
OUTPUT_DIR="$(realpath -- "$OUTPUT_DIR_INPUT")"
readonly OUTPUT_DIR
readonly STATE_DIR="$OUTPUT_DIR/.state"
readonly SERVER_DIR="$OUTPUT_DIR/.fabric-ca-server"
readonly SERVER_TLS_DIR="$SERVER_DIR/tls"
readonly SERVER_TLS_CERT="$SERVER_TLS_DIR/server-tls-cert.pem"
readonly SERVER_TLS_KEY="$SERVER_TLS_DIR/server-tls-key.pem"
readonly SECRETS_FILE="$STATE_DIR/secrets.env"
readonly MANIFEST_HASH_FILE="$STATE_DIR/manifest.sha256"
CURRENT_MANIFEST_HASH="$(sha256sum "$MANIFEST" | awk '{print $1}')"
readonly CURRENT_MANIFEST_HASH
readonly CA_URL="https://localhost:$CA_PORT"

mkdir -p -- "$STATE_DIR/logs" "$SERVER_DIR/cas" "$SERVER_TLS_DIR"
umask 077
touch "$SECRETS_FILE"
chmod 600 "$SECRETS_FILE"

if [[ -f "$MANIFEST_HASH_FILE" ]]; then
  previous_manifest_hash="$(tr -d '[:space:]' < "$MANIFEST_HASH_FILE")"
  [[ "$previous_manifest_hash" == "$CURRENT_MANIFEST_HASH" ]] || fail \
    "the manifest changed after material was generated; preserve or remove $OUTPUT_DIR manually before regenerating"
else
  manifest_hash_tmp="$MANIFEST_HASH_FILE.tmp"
  printf '%s\n' "$CURRENT_MANIFEST_HASH" > "$manifest_hash_tmp"
  chmod 600 "$manifest_hash_tmp"
  mv -- "$manifest_hash_tmp" "$MANIFEST_HASH_FILE"
fi

if python3 -c 'import socket, sys; sock = socket.socket(); sock.settimeout(0.25); status = sock.connect_ex(("127.0.0.1", int(sys.argv[1]))); sock.close(); raise SystemExit(0 if status == 0 else 1)' "$CA_PORT"; then
  fail "TCP port $CA_PORT is already in use"
fi

secret_value() {
  local key="$1"
  awk -F= -v expected="$key" '
    $1 == expected {
      print substr($0, index($0, "=") + 1)
      found = 1
      exit
    }
    END { if (!found) exit 1 }
  ' "$SECRETS_FILE"
}

get_or_create_secret() {
  local key="$1"
  local value
  if value="$(secret_value "$key" 2>/dev/null)"; then
    printf '%s' "$value"
    return
  fi
  value="$(openssl rand -hex 24)"
  printf '%s=%s\n' "$key" "$value" >> "$SECRETS_FILE"
  chmod 600 "$SECRETS_FILE"
  printf '%s' "$value"
}

assert_generated_target() {
  local target
  target="$(realpath -m -- "$1")"
  case "$target" in
    "$OUTPUT_DIR"/*) ;;
    *) fail "refusing to modify path outside generated output: $target" ;;
  esac
}

reset_generated_dir() {
  local target="$1"
  assert_generated_target "$target"
  if [[ -e "$target" ]]; then
    rm -rf -- "$target"
  fi
}

write_ca_name() {
  local config_file="$1"
  local ca_name="$2"
  local config_tmp="$config_file.tmp"
  awk -v ca_name="$ca_name" '
    /^ca:$/ {
      in_ca = 1
      print
      next
    }
    in_ca && /^[[:space:]]+name:/ {
      print "  name: " ca_name
      in_ca = 0
      next
    }
    { print }
  ' "$config_file" > "$config_tmp"
  mv -- "$config_tmp" "$config_file"
}

single_file() {
  local directory="$1"
  local files=()
  mapfile -d '' files < <(find "$directory" -maxdepth 1 -type f -print0)
  (( ${#files[@]} == 1 )) || fail "expected exactly one file in $directory, found ${#files[@]}"
  printf '%s' "${files[0]}"
}

msp_complete() {
  local msp_dir="$1"
  [[ -f "$msp_dir/config.yaml" ]]     && [[ -n "$(find "$msp_dir/signcerts" -maxdepth 1 -type f -print -quit 2>/dev/null)" ]]     && [[ -n "$(find "$msp_dir/keystore" -maxdepth 1 -type f -print -quit 2>/dev/null)" ]]     && [[ -n "$(find "$msp_dir/cacerts" -maxdepth 1 -type f -print -quit 2>/dev/null)" ]]
}

tls_complete() {
  local tls_dir="$1"
  [[ -f "$tls_dir/ca.crt" && -f "$tls_dir/server.crt" && -f "$tls_dir/server.key" ]]
}

org_msp_complete() {
  local msp_dir="$1"
  [[ -f "$msp_dir/config.yaml" ]]     && [[ -f "$msp_dir/cacerts/ca.crt" ]]     && [[ -f "$msp_dir/tlscacerts/tlsca.crt" ]]
}

write_node_ous() {
  local msp_dir="$1"
  local ca_certificate
  ca_certificate="$(single_file "$msp_dir/cacerts")"
  ca_certificate="$(basename -- "$ca_certificate")"
  cat > "$msp_dir/config.yaml" <<EOF
NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/$ca_certificate
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/$ca_certificate
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/$ca_certificate
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/$ca_certificate
    OrganizationalUnitIdentifier: orderer
EOF
}

write_org_node_ous() {
  local msp_dir="$1"
  cat > "$msp_dir/config.yaml" <<'EOF'
NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: orderer
EOF
}

normalize_tls_material() {
  local tls_dir="$1"
  local ca_certificate sign_certificate private_key
  ca_certificate="$(single_file "$tls_dir/tlscacerts")"
  sign_certificate="$(single_file "$tls_dir/signcerts")"
  private_key="$(single_file "$tls_dir/keystore")"
  cp -- "$ca_certificate" "$tls_dir/ca.crt"
  cp -- "$sign_certificate" "$tls_dir/server.crt"
  cp -- "$private_key" "$tls_dir/server.key"
  chmod 600 "$tls_dir/server.key"
}

client_base_args() {
  local ca_name="$1"
  local client_home="$2"
  printf '%s\0'     --home "$client_home"     --url "$CA_URL"     --caname "$ca_name"     --tls.certfiles "$SERVER_TLS_CERT"     --loglevel error
}

ensure_ca_admin() {
  local slug="$1"
  local ca_name="$2"
  local enrollment_id="$3"
  local enrollment_secret="$4"
  local admin_home="$STATE_DIR/ca-admins/$slug"
  if [[ -n "$(find "$admin_home/msp/signcerts" -maxdepth 1 -type f -print -quit 2>/dev/null)" ]]; then
    printf '%s' "$admin_home"
    return
  fi
  reset_generated_dir "$admin_home"
  mkdir -p -- "$admin_home"
  if ! fabric-ca-client enroll       --home "$admin_home"       --url "https://$enrollment_id:$enrollment_secret@localhost:$CA_PORT"       --caname "$ca_name"       --tls.certfiles "$SERVER_TLS_CERT"       --loglevel error       >"$STATE_DIR/logs/enroll-$slug-ca-admin.log" 2>&1; then
    fail "failed to enroll CA administrator for $slug; see $STATE_DIR/logs/enroll-$slug-ca-admin.log"
  fi
  printf '%s' "$admin_home"
}

identity_exists() {
  local admin_home="$1"
  local ca_name="$2"
  local enrollment_id="$3"
  fabric-ca-client identity list     --home "$admin_home"     --url "$CA_URL"     --caname "$ca_name"     --tls.certfiles "$SERVER_TLS_CERT"     --loglevel error     --id "$enrollment_id"     >/dev/null 2>&1
}

ensure_identity() {
  local admin_home="$1"
  local ca_name="$2"
  local enrollment_id="$3"
  local enrollment_secret="$4"
  local identity_type="$5"
  local attribute="${6:-}"
  if identity_exists "$admin_home" "$ca_name" "$enrollment_id"; then
    return
  fi
  local args=(
    register
    --home "$admin_home"
    --url "$CA_URL"
    --caname "$ca_name"
    --tls.certfiles "$SERVER_TLS_CERT"
    --loglevel error
    --id.name "$enrollment_id"
    --id.secret "$enrollment_secret"
    --id.type "$identity_type"
    --id.affiliation "."
  )
  if [[ -n "$attribute" ]]; then
    args+=(--id.attrs "$attribute")
  fi
  if ! fabric-ca-client "${args[@]}"       >"$STATE_DIR/logs/register-$enrollment_id.log" 2>&1; then
    fail "failed to register $enrollment_id; see $STATE_DIR/logs/register-$enrollment_id.log"
  fi
}

enroll_msp() {
  local slug="$1"
  local ca_name="$2"
  local enrollment_id="$3"
  local enrollment_secret="$4"
  local target_msp="$5"
  if msp_complete "$target_msp"; then
    return
  fi
  reset_generated_dir "$target_msp"
  mkdir -p -- "$(dirname -- "$target_msp")"
  local client_home="$STATE_DIR/enrollment-clients/$slug/$enrollment_id"
  mkdir -p -- "$client_home"
  if ! fabric-ca-client enroll       --home "$client_home"       --url "https://$enrollment_id:$enrollment_secret@localhost:$CA_PORT"       --caname "$ca_name"       --tls.certfiles "$SERVER_TLS_CERT"       --loglevel error       --mspdir "$target_msp"       >"$STATE_DIR/logs/enroll-$enrollment_id-msp.log" 2>&1; then
    fail "failed to enroll MSP for $enrollment_id; see $STATE_DIR/logs/enroll-$enrollment_id-msp.log"
  fi
  write_node_ous "$target_msp"
}

enroll_tls() {
  local slug="$1"
  local ca_name="$2"
  local enrollment_id="$3"
  local enrollment_secret="$4"
  local hostname="$5"
  local target_tls="$6"
  if tls_complete "$target_tls"; then
    return
  fi
  reset_generated_dir "$target_tls"
  mkdir -p -- "$(dirname -- "$target_tls")"
  local client_home="$STATE_DIR/enrollment-clients/$slug/$enrollment_id-tls"
  mkdir -p -- "$client_home"
  if ! fabric-ca-client enroll       --home "$client_home"       --url "https://$enrollment_id:$enrollment_secret@localhost:$CA_PORT"       --caname "$ca_name"       --tls.certfiles "$SERVER_TLS_CERT"       --loglevel error       --enrollment.profile tls       --csr.hosts "$hostname,localhost,127.0.0.1"       --mspdir "$target_tls"       >"$STATE_DIR/logs/enroll-$enrollment_id-tls.log" 2>&1; then
    fail "failed to enroll TLS material for $enrollment_id; see $STATE_DIR/logs/enroll-$enrollment_id-tls.log"
  fi
  normalize_tls_material "$target_tls"
}

create_org_msp() {
  local slug="$1"
  local source_msp="$2"
  local source_tls="$3"
  local target_msp="$OUTPUT_DIR/$slug/msp"
  if org_msp_complete "$target_msp"; then
    return
  fi
  reset_generated_dir "$target_msp"
  mkdir -p -- "$target_msp/cacerts" "$target_msp/tlscacerts"
  cp -- "$(single_file "$source_msp/cacerts")" "$target_msp/cacerts/ca.crt"
  cp -- "$source_tls/ca.crt" "$target_msp/tlscacerts/tlsca.crt"
  write_org_node_ous "$target_msp"
}

if [[ ! -f "$SERVER_TLS_CERT" || ! -f "$SERVER_TLS_KEY" ]]; then
  info "Generating local TLS certificate for the shared Fabric CA server"
  openssl req -x509 -newkey rsa:2048 -sha256 -nodes     -keyout "$SERVER_TLS_KEY"     -out "$SERVER_TLS_CERT"     -days 3650     -subj "/CN=localhost"     -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"     >/dev/null 2>&1
  chmod 600 "$SERVER_TLS_KEY"
fi

mapfile -t ORG_ROWS < <(
  jq -r '.organizations[] | [.mspId, .slug, .clientRole, .peerHostname, (.ordererHostname // "")] | @tsv' "$MANIFEST"
)
(( ${#ORG_ROWS[@]} >= 7 )) || fail "manifest has fewer than seven organizations"

PRIMARY_SLUG="$(jq -r '.organizations[] | select(.agentType == "REGULATOR") | .slug' "$MANIFEST")"
PRIMARY_CA_NAME="ca-$PRIMARY_SLUG"
PRIMARY_CA_HOME="$SERVER_DIR/cas/$PRIMARY_SLUG"
PRIMARY_CA_ADMIN_ID="$PRIMARY_SLUG-ca-admin"
PRIMARY_SECRET_PREFIX="$(printf '%s' "$PRIMARY_SLUG" | tr '[:lower:]-' '[:upper:]_')"
PRIMARY_CA_ADMIN_SECRET="$(get_or_create_secret "CA_${PRIMARY_SECRET_PREFIX}_ADMIN_SECRET")"

info "Initializing one logical CA per organization"
for row in "${ORG_ROWS[@]}"; do
  IFS=$'\t' read -r _msp_id slug client_role peer_hostname orderer_hostname <<< "$row"
  ca_name="ca-$slug"
  ca_home="$SERVER_DIR/cas/$slug"
  ca_admin_id="$slug-ca-admin"
  secret_prefix="$(printf '%s' "$slug" | tr '[:lower:]-' '[:upper:]_')"
  ca_admin_secret="$(get_or_create_secret "CA_${secret_prefix}_ADMIN_SECRET")"
  if [[ ! -f "$ca_home/fabric-ca-server-config.yaml" ]]; then
    mkdir -p -- "$ca_home"
    if ! fabric-ca-server init         --home "$ca_home"         --boot "$ca_admin_id:$ca_admin_secret"         --ca.name "$ca_name"         --csr.cn "$ca_name"         >"$STATE_DIR/logs/init-$slug-ca.log" 2>&1; then
      fail "failed to initialize logical CA $ca_name; see $STATE_DIR/logs/init-$slug-ca.log"
    fi
  fi
  write_ca_name "$ca_home/fabric-ca-server-config.yaml" "$ca_name"
done

server_args=(
  start
  --home "$PRIMARY_CA_HOME"
  --boot "$PRIMARY_CA_ADMIN_ID:$PRIMARY_CA_ADMIN_SECRET"
  --port "$CA_PORT"
  --tls.enabled
  --tls.certfile "$SERVER_TLS_CERT"
  --tls.keyfile "$SERVER_TLS_KEY"
  --loglevel warning
)
for row in "${ORG_ROWS[@]}"; do
  IFS=$'\t' read -r _msp_id slug client_role peer_hostname orderer_hostname <<< "$row"
  if [[ "$slug" != "$PRIMARY_SLUG" ]]; then
    server_args+=(--cafiles "$SERVER_DIR/cas/$slug/fabric-ca-server-config.yaml")
  fi
done

info "Starting the shared Fabric CA server with seven independent signing CAs"
fabric-ca-server "${server_args[@]}" >"$STATE_DIR/logs/fabric-ca-server.log" 2>&1 &
CA_PID="$!"

server_ready=false
for _ in $(seq 1 30); do
  if ! kill -0 "$CA_PID" >/dev/null 2>&1; then
    fail "Fabric CA server stopped during startup; see $STATE_DIR/logs/fabric-ca-server.log"
  fi
  if fabric-ca-client getcainfo       --home "$STATE_DIR/health-client"       --url "$CA_URL"       --caname "$PRIMARY_CA_NAME"       --tls.certfiles "$SERVER_TLS_CERT"       --loglevel error       >/dev/null 2>&1; then
    server_ready=true
    break
  fi
  sleep 1
done
[[ "$server_ready" == true ]] || fail "Fabric CA server did not become ready on $CA_URL"

info "Registering and enrolling peer, admin, user and orderer identities"
for row in "${ORG_ROWS[@]}"; do
  IFS=$'\t' read -r _msp_id slug client_role peer_hostname orderer_hostname <<< "$row"
  ca_name="ca-$slug"
  ca_admin_id="$slug-ca-admin"
  secret_prefix="$(printf '%s' "$slug" | tr '[:lower:]-' '[:upper:]_')"
  ca_admin_secret="$(get_or_create_secret "CA_${secret_prefix}_ADMIN_SECRET")"
  admin_home="$(ensure_ca_admin "$slug" "$ca_name" "$ca_admin_id" "$ca_admin_secret")"

  peer_id="$slug-peer0"
  org_admin_id="$slug-admin"
  user_id="$slug-user1"
  peer_secret="$(get_or_create_secret "ORG_${secret_prefix}_PEER_SECRET")"
  org_admin_secret="$(get_or_create_secret "ORG_${secret_prefix}_ADMIN_SECRET")"
  user_secret="$(get_or_create_secret "ORG_${secret_prefix}_USER_SECRET")"

  ensure_identity "$admin_home" "$ca_name" "$peer_id" "$peer_secret" peer
  ensure_identity "$admin_home" "$ca_name" "$org_admin_id" "$org_admin_secret" admin
  ensure_identity "$admin_home" "$ca_name" "$user_id" "$user_secret" client     "snt.role=$client_role:ecert"

  org_dir="$OUTPUT_DIR/$slug"
  peer_dir="$org_dir/peers/$peer_hostname"
  admin_dir="$org_dir/users/Admin@$slug.snt.local"
  user_dir="$org_dir/users/User1@$slug.snt.local"

  enroll_msp "$slug" "$ca_name" "$peer_id" "$peer_secret" "$peer_dir/msp"
  enroll_tls "$slug" "$ca_name" "$peer_id" "$peer_secret" "$peer_hostname" "$peer_dir/tls"
  enroll_msp "$slug" "$ca_name" "$org_admin_id" "$org_admin_secret" "$admin_dir/msp"
  enroll_msp "$slug" "$ca_name" "$user_id" "$user_secret" "$user_dir/msp"

  if [[ -n "$orderer_hostname" ]]; then
    orderer_id="$slug-orderer"
    orderer_secret="$(get_or_create_secret "ORG_${secret_prefix}_ORDERER_SECRET")"
    ensure_identity "$admin_home" "$ca_name" "$orderer_id" "$orderer_secret" orderer
    orderer_dir="$org_dir/orderers/$orderer_hostname"
    enroll_msp "$slug" "$ca_name" "$orderer_id" "$orderer_secret" "$orderer_dir/msp"
    enroll_tls "$slug" "$ca_name" "$orderer_id" "$orderer_secret" "$orderer_hostname" "$orderer_dir/tls"
  fi

  create_org_msp "$slug" "$peer_dir/msp" "$peer_dir/tls"
done

SNT_ORGANIZATIONS_MANIFEST="$MANIFEST" \
  SNT_ORGANIZATIONS_SCHEMA="$SCHEMA" \
  SNT_CONFIGTX_FILE="$CONFIGTX" \
  SNT_CRYPTO_OUTPUT_DIR="$OUTPUT_DIR" \
  "$SCRIPT_DIR/verify-crypto.sh"

info "Cryptographic material is ready under $OUTPUT_DIR"
