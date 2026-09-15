#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BASELINE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
REPOSITORY_DIR="$(cd -- "${BASELINE_DIR}/.." && pwd)"
COMPOSE_FILE="${BASELINE_DIR}/compose.yaml"
DATASET_DIR="$(mktemp -d)"

export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-snt_baseline_packaging_$$}"
export SNT_BASELINE_DB_NAME="snt_baseline_packaging"
export SNT_BASELINE_DB_USER="snt_baseline_packaging"
SNT_BASELINE_DB_PASSWORD="$(openssl rand -hex 32)"
export SNT_BASELINE_DB_PASSWORD
export SNT_BASELINE_DB_PORT=0
export SNT_BASELINE_API_PORT=0
API_KEY="$(openssl rand -hex 32)"
export SNT_BASELINE_API_KEYS="[{\"key\":\"${API_KEY}\",\"mspId\":\"LabMSP\",\"role\":\"operator\"}]"
export SNT_BASELINE_DATASET_DIR="${DATASET_DIR}"
export SNT_BASELINE_IMAGE="snt-baseline-test:$$"

compose() {
    docker compose -f "${COMPOSE_FILE}" "$@"
}

cleanup() {
    compose --profile seed down --volumes --remove-orphans >/dev/null 2>&1 || true
    rm -rf -- "${DATASET_DIR}"
}
trap cleanup EXIT

psql_in_container() {
    compose exec -T postgres psql \
        --username "${SNT_BASELINE_DB_USER}" \
        --dbname "${SNT_BASELINE_DB_NAME}" \
        -v ON_ERROR_STOP=1 \
        "$@"
}

compose config --quiet
compose up -d --build --wait postgres api
make -C "${REPOSITORY_DIR}/client" generate-dataset UNITS=50000 OUTPUT_DIR="${DATASET_DIR}"
chmod 0755 "${DATASET_DIR}"
chmod 0444 \
    "${DATASET_DIR}/dataset.json" \
    "${DATASET_DIR}/dataset.sha256" \
    "${DATASET_DIR}/manifest.json"
compose --profile seed run --rm seed

psql_in_container --file - <<'SQL'
DO $$
BEGIN
    IF (SELECT count(*) FROM public.organizations) <> 7 THEN
        RAISE EXCEPTION 'expected 7 organizations';
    END IF;
    IF (SELECT count(*) FROM public.medication_units) <> 50000 THEN
        RAISE EXCEPTION 'expected 50000 medication units';
    END IF;
    IF (SELECT count(*) FROM public.unit_events) <> 50000 THEN
        RAISE EXCEPTION 'expected 50000 unit events';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.medication_units
        WHERE estado <> 'EN_LABORATORIO' OR custodio_actual <> 'GLN:7791234500017'
    ) THEN
        RAISE EXCEPTION 'snapshot contains a non-initial unit';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.unit_events
        WHERE operation <> 'RegisterUnit'
           OR invoker_msp_id <> 'LabMSP'
           OR event_sequence <> 1
    ) THEN
        RAISE EXCEPTION 'snapshot contains a non-registration event';
    END IF;
    IF (SELECT count(*) FROM public.transfer_operations) <> 0
       OR (SELECT count(*) FROM public.return_operations) <> 0
       OR (SELECT count(*) FROM public.lab_interventions) <> 0 THEN
        RAISE EXCEPTION 'workload operations were executed during seed';
    END IF;
END
$$;
SQL

read -r first_gtin first_serial < <(
    psql_in_container --tuples-only --no-align --field-separator ' ' \
        --command 'SELECT gtin, numero_serie FROM public.medication_units ORDER BY gtin, numero_serie LIMIT 1;'
)
api_address="$(compose port api 8080)"
api_port="${api_address##*:}"
curl --fail --silent --show-error \
    "http://127.0.0.1:${api_port}/v1/units/${first_gtin}/${first_serial}" |
    grep --fixed-strings --quiet "\"numeroSerie\":\"${first_serial}\""

before_counts="$(psql_in_container --tuples-only --no-align --command \
    "SELECT (SELECT count(*) FROM public.organizations) || ':' ||
            (SELECT count(*) FROM public.medication_units) || ':' ||
            (SELECT count(*) FROM public.unit_events);")"
if compose --profile seed run --rm seed; then
    echo "ERROR: a second seed unexpectedly succeeded" >&2
    exit 1
fi
after_counts="$(psql_in_container --tuples-only --no-align --command \
    "SELECT (SELECT count(*) FROM public.organizations) || ':' ||
            (SELECT count(*) FROM public.medication_units) || ':' ||
            (SELECT count(*) FROM public.unit_events);")"
test "${before_counts}" = "${after_counts}"

echo "baseline packaging and seed integration test passed"
