UNITS ?= 50000
OUTPUT_DIR ?= ../build/dataset
RUN_METADATA ?= $(wildcard ../benchmarks/examples/*.json)
DEMO_ARGS ?=
LISTENER_ARGS ?=
DEMO_SERVER_ARGS ?=

.PHONY: build test test-race vet demo-core demo-server listen-anmat generate-dataset validate-run-metadata

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

demo-core:
	go run ./cmd/snt-client demo-core $(DEMO_ARGS)

demo-server:
	go run ./cmd/snt-client demo-server $(DEMO_SERVER_ARGS)

listen-anmat:
	go run ./cmd/snt-client listen-anmat $(LISTENER_ARGS)

generate-dataset:
	go run ./cmd/datasetgen --units $(UNITS) --output-dir $(OUTPUT_DIR)

validate-run-metadata:
	go run ./cmd/runmeta $(RUN_METADATA)
