COMPOSE := docker compose -f $(CURDIR)/compose.yaml
SHELLCHECK ?= shellcheck
UNITS ?= 50000
DATASET_DIR ?= ../build/dataset

.PHONY: check-env check-api-env build up down db-up db-down migrate-up migrate-down migrate-version generate-dataset seed format-check lint test

check-env:
	@test -n "$$SNT_BASELINE_DB_PASSWORD" || { \
		echo "ERROR: SNT_BASELINE_DB_PASSWORD must be set" >&2; exit 1; \
	}

check-api-env: check-env
	@test -n "$$SNT_BASELINE_API_KEYS" || { \
		echo "ERROR: SNT_BASELINE_API_KEYS must be set" >&2; exit 1; \
	}

build: check-api-env
	$(COMPOSE) build api

up: check-api-env
	$(COMPOSE) up -d --build --wait postgres api

down:
	SNT_BASELINE_DB_PASSWORD="$${SNT_BASELINE_DB_PASSWORD:-unused}" \
	SNT_BASELINE_API_KEYS="$${SNT_BASELINE_API_KEYS:-[]}" \
	$(COMPOSE) down --remove-orphans

db-up: check-env
	SNT_BASELINE_API_KEYS="$${SNT_BASELINE_API_KEYS:-[]}" \
	$(COMPOSE) up -d --wait postgres

db-down:
	SNT_BASELINE_DB_PASSWORD="$${SNT_BASELINE_DB_PASSWORD:-unused}" \
	SNT_BASELINE_API_KEYS="$${SNT_BASELINE_API_KEYS:-[]}" \
	$(COMPOSE) down

migrate-up: check-env
	./scripts/migrate.sh up

migrate-down: check-env
	./scripts/migrate.sh down

migrate-version: check-env
	./scripts/migrate.sh version

generate-dataset:
	$(MAKE) -C ../client generate-dataset UNITS=$(UNITS) OUTPUT_DIR="$(abspath $(DATASET_DIR))"

seed: check-api-env
	SNT_BASELINE_DATASET_DIR="$(abspath $(DATASET_DIR))" \
	$(COMPOSE) --profile seed run --rm seed

format-check:
	@test -z "$$(gofmt -l cmd internal tests)" || { gofmt -l cmd internal tests; exit 1; }

lint: format-check
	$(SHELLCHECK) scripts/migrate.sh scripts/container-entrypoint.sh tests/schema-integration.sh tests/packaging-integration.sh
	go vet ./...

test: lint
	./tests/schema-integration.sh
	./tests/packaging-integration.sh
