#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BASELINE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${BASELINE_DIR}/compose.yaml"
DB_NAME="${SNT_BASELINE_DB_NAME:-snt_baseline}"
DB_USER="${SNT_BASELINE_DB_USER:-snt_baseline}"

: "${SNT_BASELINE_DB_PASSWORD:?SNT_BASELINE_DB_PASSWORD must be set}"

compose() {
    docker compose -f "${COMPOSE_FILE}" "$@"
}

psql_in_container() {
    compose exec -T postgres psql \
        --username "${DB_USER}" \
        --dbname "${DB_NAME}" \
        -v ON_ERROR_STOP=1 \
        "$@"
}

ensure_metadata() {
    psql_in_container >/dev/null <<'SQL'
CREATE SCHEMA IF NOT EXISTS baseline_meta;
CREATE TABLE IF NOT EXISTS baseline_meta.schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
SQL
}

current_version() {
    psql_in_container --tuples-only --no-align \
        --command 'SELECT COALESCE(MAX(version), 0) FROM baseline_meta.schema_migrations;'
}

migrate_up() {
    local file base raw_version version applied
    local -a files

    ensure_metadata
    shopt -s nullglob
    files=("${BASELINE_DIR}"/migrations/*.up.sql)
    shopt -u nullglob
    if (("${#files[@]}" == 0)); then
        echo "ERROR: no up migrations found" >&2
        exit 1
    fi

    for file in "${files[@]}"; do
        base="$(basename -- "${file}")"
        raw_version="${base%%_*}"
        if [[ ! "${raw_version}" =~ ^[0-9]+$ ]]; then
            echo "ERROR: invalid migration filename: ${base}" >&2
            exit 1
        fi
        version=$((10#${raw_version}))
        applied="$(psql_in_container --tuples-only --no-align \
            --command "SELECT EXISTS (SELECT 1 FROM baseline_meta.schema_migrations WHERE version = ${version});")"
        if [[ "${applied}" == "t" ]]; then
            continue
        fi

        psql_in_container --single-transaction \
            --file "/migrations/${base}" \
            --command "INSERT INTO baseline_meta.schema_migrations(version) VALUES (${version});"
        echo "applied migration ${base}"
    done
}

migrate_down() {
    local version prefix base
    local -a matches

    ensure_metadata
    version="$(current_version)"
    if [[ "${version}" == "0" ]]; then
        echo "no migration to revert"
        return
    fi

    printf -v prefix '%06d' "${version}"
    shopt -s nullglob
    matches=("${BASELINE_DIR}"/migrations/"${prefix}"_*.down.sql)
    shopt -u nullglob
    if (("${#matches[@]}" != 1)); then
        echo "ERROR: expected one down migration for version ${version}" >&2
        exit 1
    fi

    base="$(basename -- "${matches[0]}")"
    psql_in_container --single-transaction \
        --file "/migrations/${base}" \
        --command "DELETE FROM baseline_meta.schema_migrations WHERE version = ${version};"
    echo "reverted migration ${base}"
}

case "${1:-}" in
    up)
        migrate_up
        ;;
    down)
        migrate_down
        ;;
    version)
        ensure_metadata
        current_version
        ;;
    *)
        echo "usage: $0 {up|down|version}" >&2
        exit 2
        ;;
esac
