COMPOSE := docker compose -f $(CURDIR)/compose.yaml
SHELLCHECK ?= shellcheck

.PHONY: check-env db-up db-down migrate-up migrate-down migrate-version lint test

check-env:
	@test -n "$$SNT_BASELINE_DB_PASSWORD" || { \
		echo "ERROR: SNT_BASELINE_DB_PASSWORD must be set" >&2; exit 1; \
	}

db-up: check-env
	$(COMPOSE) up -d --wait postgres

db-down:
	SNT_BASELINE_DB_PASSWORD="$${SNT_BASELINE_DB_PASSWORD:-unused}" $(COMPOSE) down

migrate-up: check-env
	./scripts/migrate.sh up

migrate-down: check-env
	./scripts/migrate.sh down

migrate-version: check-env
	./scripts/migrate.sh version

lint:
	$(SHELLCHECK) scripts/migrate.sh tests/schema-integration.sh

test: lint
	./tests/schema-integration.sh
