#!/usr/bin/env sh
set -eu

: "${SNT_BASELINE_DATABASE_URL:?SNT_BASELINE_DATABASE_URL must be set}"

export SNT_BASELINE_MIGRATION_URL="${SNT_BASELINE_DATABASE_URL}"
export SNT_BASELINE_MIGRATIONS_DIR="/app/migrations"

/app/scripts/migrate.sh up
exec /usr/local/bin/snt-baseline
