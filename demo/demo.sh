#!/usr/bin/env bash
# Arranque en un comando de la demo de la defensa (DEMO-4, #146).
#
# Levanta la red con network.sh (idempotente: conserva el ledger), compila
# snt-client, inicia demo-server en segundo plano y espera a que el escenario
# esté listo. No borra volúmenes ni modifica la red.
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPOSITORY_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly REPOSITORY_ROOT
readonly NETWORK_SCRIPT="${REPOSITORY_ROOT}/network/network.sh"
readonly STATE_DIR="${SNT_DEMO_DIR:-${REPOSITORY_ROOT}/build/demo}"
readonly BINARY="${STATE_DIR}/snt-client"
readonly PID_FILE="${STATE_DIR}/demo-server.pid"
readonly LOG_FILE="${STATE_DIR}/demo-server.log"
readonly ADDRESS="${SNT_DEMO_ADDR:-127.0.0.1:8080}"
readonly URL="http://${ADDRESS}/"
readonly READY_TIMEOUT="${SNT_DEMO_READY_TIMEOUT:-120}"
readonly MIN_CPUS=6
readonly MIN_MEMORY_KB=7500000

FAILURES=0

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '\n==> %s\n' "$*"
}

check_ok() {
  printf '  [OK]    %s\n' "$*"
}

check_warn() {
  printf '  [AVISO] %s\n' "$*"
}

check_fail() {
  printf '  [FALLA] %s\n' "$*"
  FAILURES=$((FAILURES + 1))
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

server_pid() {
  local pid
  [[ -f "${PID_FILE}" ]] || return 1
  pid="$(<"${PID_FILE}")"
  [[ "${pid}" =~ ^[0-9]+$ ]] || return 1
  # El PID sólo cuenta si sigue siendo nuestro demo-server.
  [[ -r "/proc/${pid}/cmdline" ]] || return 1
  tr '\0' ' ' <"/proc/${pid}/cmdline" | grep -q "demo-server" || return 1
  printf '%s\n' "${pid}"
}

api_state() {
  curl --silent --fail --max-time 5 "${URL}api/state"
}

port_in_use() {
  local port="${ADDRESS##*:}"
  (exec 3<>"/dev/tcp/127.0.0.1/${port}") 2>/dev/null
}

wait_until_ready() {
  local deadline=$((SECONDS + READY_TIMEOUT))
  local preparation=""
  while ((SECONDS < deadline)); do
    preparation="$(api_state 2>/dev/null | jq -r '.scenario.preparation' 2>/dev/null || true)"
    case "${preparation}" in
      LISTA) return 0 ;;
      FALLIDA)
        api_state | jq -r '.scenario.preparationError.message // "sin detalle"' >&2
        fail "the demo scenario could not be prepared; see ${LOG_FILE}"
        ;;
    esac
    if ! server_pid >/dev/null; then
      tail -n 20 "${LOG_FILE}" >&2 || true
      fail "demo-server exited; see ${LOG_FILE}"
    fi
    sleep 1
  done
  fail "the demo was not ready after ${READY_TIMEOUT}s (last state: ${preparation:-sin respuesta})"
}

open_browser() {
  if command -v wslview >/dev/null 2>&1; then
    wslview "${URL}" >/dev/null 2>&1 &
  elif command -v cmd.exe >/dev/null 2>&1; then
    cmd.exe /c start "" "${URL}" >/dev/null 2>&1 || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "${URL}" >/dev/null 2>&1 &
  else
    printf 'Abrí %s en el navegador.\n' "${URL}"
  fi
}

# demo-server rechaza una dirección que no sea loopback; se valida antes de
# levantar la red para no esperar en vano.
require_loopback() {
  case "${ADDRESS%:*}" in
    localhost | 127.* | "[::1]") ;;
    *) fail "SNT_DEMO_ADDR must be a loopback address, got ${ADDRESS}" ;;
  esac
}

# --- Comandos ------------------------------------------------------------------

check() {
  local cpus memory_kb running
  info "Checklist de la demo"

  if docker info >/dev/null 2>&1; then
    check_ok "Docker responde"
  else
    check_fail "Docker no responde: encender Docker Desktop"
  fi

  cpus="$(nproc)"
  memory_kb="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
  if ((cpus >= MIN_CPUS && memory_kb >= MIN_MEMORY_KB)); then
    check_ok "Recursos: ${cpus} CPU y $((memory_kb / 1024 / 1024)) GB"
  else
    check_warn "Recursos: ${cpus} CPU y $((memory_kb / 1024 / 1024)) GB; se recomiendan ${MIN_CPUS} CPU y 8 GB (network/wslconfig.example)"
  fi

  if [[ -n "${SNT_FABRIC_BIN_DIR:-}" ]]; then
    PATH="${SNT_FABRIC_BIN_DIR}:${PATH}"
  fi
  if command -v peer >/dev/null 2>&1; then
    check_ok "Binarios de Fabric: $(command -v peer)"
  else
    check_fail "Binarios de Fabric no encontrados: exportar SNT_FABRIC_BIN_DIR y SNT_FABRIC_CFG_PATH"
  fi

  for tool in go jq curl; do
    if command -v "${tool}" >/dev/null 2>&1; then
      check_ok "${tool} disponible"
    else
      check_fail "${tool} no está instalado"
    fi
  done

  if server_pid >/dev/null; then
    check_ok "demo-server ya está corriendo en ${URL}"
  elif port_in_use; then
    check_fail "El puerto de ${ADDRESS} está ocupado por otro proceso (cambiar con SNT_DEMO_ADDR)"
  else
    check_ok "Puerto libre: ${ADDRESS}"
  fi

  running="$(docker compose -f "${REPOSITORY_ROOT}/network/compose.yaml" ps --status running --services 2>/dev/null | wc -l)"
  if ((running >= 10)); then
    if "${NETWORK_SCRIPT}" verify >/dev/null 2>&1; then
      check_ok "Red verificada (network.sh verify)"
    else
      check_fail "La red está levantada pero network.sh verify falla"
    fi
  else
    check_warn "La red no está levantada: demo.sh up la levanta"
  fi

  if ((FAILURES > 0)); then
    printf '\n%d punto(s) a resolver antes de la demo.\n' "${FAILURES}"
    return 1
  fi
  printf '\nChecklist completo.\n'
}

up() {
  local open=false
  for argument in "$@"; do
    case "${argument}" in
      --open) open=true ;;
      *) fail "unknown option for up: ${argument}" ;;
    esac
  done
  require_command docker
  require_command go
  require_command curl
  require_command jq
  require_loopback
  local started=${SECONDS}

  if server_pid >/dev/null; then
    info "demo-server ya está corriendo"
  else
    if port_in_use; then
      fail "${ADDRESS} is already in use; set SNT_DEMO_ADDR to another loopback port"
    fi
    info "Levantando la red (conserva el ledger)"
    "${NETWORK_SCRIPT}" up
    "${NETWORK_SCRIPT}" createChannel
    "${NETWORK_SCRIPT}" deployCC
    "${NETWORK_SCRIPT}" verify

    info "Compilando snt-client"
    mkdir -p "${STATE_DIR}"
    (cd "${REPOSITORY_ROOT}/client" && go build -o "${BINARY}" ./cmd/snt-client)

    info "Iniciando demo-server en ${URL}"
    nohup "${BINARY}" demo-server --addr "${ADDRESS}" --repo-root "${REPOSITORY_ROOT}" \
      >"${LOG_FILE}" 2>&1 </dev/null &
    printf '%s\n' "$!" >"${PID_FILE}"
  fi

  info "Esperando el escenario"
  wait_until_ready
  printf '\nDemo lista en %s (%ss).\n' "${URL}" "$((SECONDS - started))"
  if [[ "${open}" == true ]]; then
    open_browser
  fi
}

reset() {
  require_command curl
  require_command jq
  server_pid >/dev/null || fail "demo-server is not running; execute demo/demo.sh up"
  local started=${SECONDS}
  info "Empezando un ensayo nuevo"
  curl --silent --fail --max-time 10 -X POST "${URL}api/reset" >/dev/null
  wait_until_ready
  printf 'Ensayo nuevo listo en %ss: %s\n' "$((SECONDS - started))" \
    "$(api_state | jq -r '.scenario.serial')"
}

status() {
  local pid
  if ! pid="$(server_pid)"; then
    printf 'demo-server: detenido\n'
    return 1
  fi
  printf 'demo-server: PID %s en %s\n' "${pid}" "${URL}"
  api_state | jq -r '"red: \(if .connected then "en línea" else "sin conexión" end)
escenario: \(.scenario.preparation), serie \(.scenario.serial), paso siguiente \(.scenario.nextStep)"'
}

stop() {
  local pid
  if ! pid="$(server_pid)"; then
    printf 'demo-server no está corriendo.\n'
    rm -f "${PID_FILE}"
    return 0
  fi
  kill "${pid}"
  for _ in $(seq 1 20); do
    kill -0 "${pid}" 2>/dev/null || break
    sleep 0.25
  done
  rm -f "${PID_FILE}"
  printf 'demo-server detenido. La red sigue levantada (network/network.sh down la detiene).\n'
}

usage() {
  cat <<'USAGE'
Usage: demo/demo.sh <command>

Commands:
  check        checklist previo: Docker, recursos, binarios, puerto y red
  up [--open]  levanta la red, compila snt-client e inicia demo-server
  reset        empieza un ensayo nuevo con series nuevas
  status       estado de demo-server y del escenario
  stop         detiene demo-server (la red sigue levantada)

Variables: SNT_DEMO_ADDR (127.0.0.1:8080), SNT_FABRIC_BIN_DIR, SNT_FABRIC_CFG_PATH.
USAGE
}

main() {
  local command="${1:-}"
  shift || true
  case "${command}" in
    check) check ;;
    up) up "$@" ;;
    reset) reset ;;
    status) status ;;
    stop) stop ;;
    -h|--help|help|"") usage ;;
    *) usage >&2; fail "unknown command: ${command}" ;;
  esac
}

main "$@"
