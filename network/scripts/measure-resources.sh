#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd -- "${script_dir}/../.." && pwd)"
compose_file="${repository_root}/network/compose.yaml"

"${script_dir}/validate-compose-prerequisites.sh" --measurement

mapfile -t services < <(docker compose -f "${compose_file}" config --services)

if [[ "${#services[@]}" -ne 11 ]]; then
  echo "ERROR: se esperaban 11 servicios y se encontraron ${#services[@]}." >&2
  exit 1
fi

invalid_run=0
declare -a container_ids=()
declare -A container_id_by_service=()
for service in "${services[@]}"; do
  mapfile -t service_container_ids < <(docker compose -f "${compose_file}" ps -q "${service}")
  if [[ "${#service_container_ids[@]}" -ne 1 ]]; then
    echo "ERROR: ${service} debe tener exactamente un contenedor en ejecución; se encontraron ${#service_container_ids[@]}." >&2
    invalid_run=1
    continue
  fi

  container_id="${service_container_ids[0]}"
  inspection="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing-healthcheck{{end}} {{.RestartCount}} {{.State.OOMKilled}}' "${container_id}" 2>/dev/null || true)"
  if [[ -z "${inspection}" ]]; then
    echo "ERROR: no se pudo inspeccionar el contenedor de ${service}." >&2
    invalid_run=1
    continue
  fi

  read -r status restart_count oom_killed <<< "${inspection}"
  container_ids+=("${container_id}")
  container_id_by_service["${service}"]="${container_id}"

  if [[ "${status}" != "healthy" ]]; then
    echo "ERROR: ${service} está ${status:-ausente}." >&2
    invalid_run=1
  fi
  if [[ "${restart_count}" -ne 0 ]]; then
    echo "ERROR: ${service} registra ${restart_count} reinicio(s); la medición no es válida." >&2
    invalid_run=1
  fi
  if [[ "${oom_killed}" != "false" ]]; then
    echo "ERROR: ${service} registra OOMKilled=${oom_killed}; la medición no es válida." >&2
    invalid_run=1
  fi
done

if [[ "${invalid_run}" -ne 0 ]]; then
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
echo "container,health,restart_count,oom_killed"
for service in "${services[@]}"; do
  docker inspect --format '{{.Name}},{{.State.Health.Status}},{{.RestartCount}},{{.State.OOMKilled}}' "${container_id_by_service[${service}]}" | sed 's#^/##'
done

echo
echo "container,cpu_percent,memory_usage,memory_percent,pids"
docker stats --no-stream \
  --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}},{{.PIDs}}' \
  "${container_ids[@]}"

echo
echo "container,persistent_kib"
for service in "${services[@]}"; do
  container_id="${container_id_by_service[${service}]}"
  case "${service}" in
    peer0.*)
      size="$(docker exec "${container_id}" du -sk /var/hyperledger/production 2>/dev/null | awk '{print $1}')"
      ;;
    orderer.*)
      size="$(docker exec "${container_id}" du -sk /var/hyperledger/production/orderer 2>/dev/null | awk '{print $1}')"
      ;;
    fabric-ca.*)
      size="$(docker exec "${container_id}" du -sk /etc/hyperledger/fabric-ca-server 2>/dev/null | awk '{print $1}')"
      ;;
    *)
      size="unknown"
      ;;
  esac
  echo "${service},${size:-unknown}"
done
