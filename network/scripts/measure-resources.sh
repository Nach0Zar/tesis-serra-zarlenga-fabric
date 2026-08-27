#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd -- "${script_dir}/../.." && pwd)"
compose_file="${repository_root}/network/compose.yaml"

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker no está disponible en PATH." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "ERROR: el daemon Docker no está disponible." >&2
  exit 1
fi

mapfile -t services < <(docker compose -f "${compose_file}" config --services)

if [[ "${#services[@]}" -ne 11 ]]; then
  echo "ERROR: se esperaban 11 servicios y se encontraron ${#services[@]}." >&2
  exit 1
fi

unhealthy=0
for service in "${services[@]}"; do
  status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing-healthcheck{{end}}' "${service}" 2>/dev/null || true)"
  if [[ "${status}" != "healthy" ]]; then
    echo "ERROR: ${service} está ${status:-ausente}." >&2
    unhealthy=1
  fi
done

if [[ "${unhealthy}" -ne 0 ]]; then
  exit 1
fi

echo "timestamp_utc=$(date --utc +%Y-%m-%dT%H:%M:%SZ)"
echo "repository_commit=$(git -C "${repository_root}" rev-parse HEAD)"
echo "kernel=$(uname -srmo)"
echo "processors=$(nproc)"
echo "memory_bytes=$(awk '/MemTotal:/ {printf "%.0f\n", $2 * 1024}' /proc/meminfo)"
echo "swap_bytes=$(awk '/SwapTotal:/ {printf "%.0f\n", $2 * 1024}' /proc/meminfo)"
echo "docker_version=$(docker version --format '{{.Server.Version}}')"
echo "compose_version=$(docker compose version --short)"
echo "service_count=${#services[@]}"

echo
echo "container,health"
for service in "${services[@]}"; do
  docker inspect --format '{{.Name}},{{.State.Health.Status}}' "${service}" | sed 's#^/##'
done

echo
echo "container,cpu_percent,memory_usage,memory_percent,pids"
docker stats --no-stream \
  --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}},{{.PIDs}}' \
  "${services[@]}"

echo
echo "container,persistent_kib"
for service in "${services[@]}"; do
  case "${service}" in
    peer0.*)
      size="$(docker exec "${service}" du -sk /var/hyperledger/production 2>/dev/null | awk '{print $1}')"
      ;;
    orderer.*)
      size="$(docker exec "${service}" du -sk /var/hyperledger/production/orderer 2>/dev/null | awk '{print $1}')"
      ;;
    fabric-ca.*)
      size="$(docker exec "${service}" du -sk /etc/hyperledger/fabric-ca-server 2>/dev/null | awk '{print $1}')"
      ;;
    *)
      size="unknown"
      ;;
  esac
  echo "${service},${size:-unknown}"
done
