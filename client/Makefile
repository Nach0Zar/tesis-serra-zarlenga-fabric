UNITS ?= 50000
OUTPUT_DIR ?= ../build/dataset
DEMO_ARGS ?=

.PHONY: build test test-race vet demo-core generate-dataset

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

generate-dataset:
	go run ./cmd/datasetgen --units $(UNITS) --output-dir $(OUTPUT_DIR)
