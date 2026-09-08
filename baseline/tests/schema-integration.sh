#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BASELINE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${BASELINE_DIR}/compose.yaml"

export COMPOSE_PROJECT_NAME="snt_baseline_test_$$"
export SNT_BASELINE_DB_NAME="snt_baseline_test"
export SNT_BASELINE_DB_USER="snt_baseline_test"
SNT_BASELINE_DB_PASSWORD="$(openssl rand -hex 32)"
export SNT_BASELINE_DB_PASSWORD
export SNT_BASELINE_DB_PORT=0

compose() {
    docker compose -f "${COMPOSE_FILE}" "$@"
}

cleanup() {
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
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
compose up -d --wait postgres

"${BASELINE_DIR}/scripts/migrate.sh" up
test "$("${BASELINE_DIR}/scripts/migrate.sh" version)" = "2"

database_address="$(compose port postgres 5432)"
database_port="${database_address##*:}"
export SNT_BASELINE_TEST_DSN="postgres://${SNT_BASELINE_DB_USER}:${SNT_BASELINE_DB_PASSWORD}@127.0.0.1:${database_port}/${SNT_BASELINE_DB_NAME}?sslmode=disable"
(cd "${BASELINE_DIR}" && go test -p 1 ./...)

psql_in_container --file - < "${SCRIPT_DIR}/schema_test.sql"

"${BASELINE_DIR}/scripts/migrate.sh" down
test "$("${BASELINE_DIR}/scripts/migrate.sh" version)" = "1"

"${BASELINE_DIR}/scripts/migrate.sh" down
test "$("${BASELINE_DIR}/scripts/migrate.sh" version)" = "0"
test "$(psql_in_container --tuples-only --no-align --command \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';")" = "0"

"${BASELINE_DIR}/scripts/migrate.sh" up
test "$("${BASELINE_DIR}/scripts/migrate.sh" version)" = "2"
psql_in_container --file - < "${SCRIPT_DIR}/schema_test.sql"

echo "baseline schema integration test passed"
