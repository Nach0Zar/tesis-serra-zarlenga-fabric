#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd -- "${script_dir}/../.." && pwd)"
compose_file="${repository_root}/network/compose.yaml"
measurement_profile=false

usage() {
  cat <<'EOF'
Usage: validate-compose-prerequisites.sh [--measurement]

Validates the generated NET-1 material and the NET-3 Docker Compose file.
With --measurement, it also requires the homogeneous WSL2 profile defined in
network/wslconfig.example (6 processors, 8 GiB memory and 4 GiB swap).
EOF
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

case "${1:-}" in
  "") ;;
  --measurement) measurement_profile=true ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

(( $# <= 1 )) || {
  usage >&2
  exit 2
}

command -v git >/dev/null 2>&1 || fail "git no está disponible en PATH."
command -v docker >/dev/null 2>&1 || fail "docker no está disponible en PATH."
docker info >/dev/null 2>&1 || fail "el daemon Docker no está disponible."
docker compose version >/dev/null 2>&1 || fail "Docker Compose no está disponible."

if ! "${script_dir}/verify-crypto.sh"; then
  printf '%s\n' \
    "ERROR: el material local de NET-1 no está completo." \
    "Ejecute ./network/scripts/generate-crypto.sh y repita la validación." >&2
  exit 1
fi

docker compose -f "${compose_file}" config --quiet
mapfile -t services < <(docker compose -f "${compose_file}" config --services)
(( ${#services[@]} == 11 )) \
  || fail "se esperaban 11 servicios y se encontraron ${#services[@]}."

if [[ "${measurement_profile}" == true ]]; then
  grep -qi microsoft /proc/sys/kernel/osrelease \
    || fail "--measurement requiere ejecutar la prueba dentro de WSL2."
  measurement_files=(
    network/compose.yaml
    network/wslconfig.example
    network/organizations-manifest.json
    network/scripts/generate-crypto.sh
    network/scripts/measure-resources.sh
    network/scripts/validate-compose-prerequisites.sh
    network/scripts/verify-crypto.sh
  )
  for measurement_file in "${measurement_files[@]}"; do
    git -C "${repository_root}" ls-files --error-unmatch "${measurement_file}" \
      >/dev/null 2>&1 \
      || fail "${measurement_file} debe estar versionado antes de medir."
  done
  git -C "${repository_root}" diff --quiet -- "${measurement_files[@]}" \
    || fail "hay cambios sin commit en archivos que afectan la medición."
  git -C "${repository_root}" diff --cached --quiet -- "${measurement_files[@]}" \
    || fail "hay cambios staged sin commit en archivos que afectan la medición."

  processors="$(nproc)"
  memory_bytes="$(awk '/MemTotal:/ {printf "%.0f\n", $2 * 1024}' /proc/meminfo)"
  swap_bytes="$(awk '/SwapTotal:/ {printf "%.0f\n", $2 * 1024}' /proc/meminfo)"
  readonly expected_processors=6
  readonly minimum_memory_bytes=8053063680
  readonly maximum_memory_bytes=8589934592
  readonly expected_swap_bytes=4294967296

  [[ "${processors}" -eq "${expected_processors}" ]] \
    || fail "el perfil homogéneo requiere 6 procesadores; se detectaron ${processors}."
  (( memory_bytes >= minimum_memory_bytes && memory_bytes <= maximum_memory_bytes )) \
    || fail "el perfil homogéneo requiere memory=8GB; WSL informó ${memory_bytes} bytes."
  [[ "${swap_bytes}" -eq "${expected_swap_bytes}" ]] \
    || fail "el perfil homogéneo requiere swap=4GB; WSL informó ${swap_bytes} bytes."

  printf 'OK: perfil homogéneo WSL2: processors=%s memory_bytes=%s swap_bytes=%s\n' \
    "${processors}" "${memory_bytes}" "${swap_bytes}"
fi

printf 'OK: material NET-1 verificado y Compose NET-3 válido con 11 servicios.\n'
