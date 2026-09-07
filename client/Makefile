UNITS ?= 50000
OUTPUT_DIR ?= ../build/dataset

.PHONY: build test test-race vet generate-dataset

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

generate-dataset:
	go run ./cmd/datasetgen --units $(UNITS) --output-dir $(OUTPUT_DIR)
