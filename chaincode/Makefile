# Tareas de desarrollo y empaquetado del chaincode `snt`.
#
# El layout del workspace Go y el motivo del paso de vendoring estan explicados
# en chaincode/README.md, "Layout del workspace Go".

REPO_ROOT := $(abspath $(CURDIR)/..)
CANONICAL_MANIFEST := $(REPO_ROOT)/network/organizations-manifest.json
EMBEDDED_MANIFEST := $(REPO_ROOT)/domain/manifest/organizations-manifest.json

CC_NAME ?= snt
CC_VERSION ?= 1.0
CC_LABEL ?= $(CC_NAME)_$(CC_VERSION)
PACKAGE_OUT ?= $(CURDIR)/$(CC_LABEL).tar.gz

.PHONY: all build test cover lint vendor package sync-manifest verify-manifest clean

all: verify-manifest build test

build:
	go build ./...
	cd $(REPO_ROOT)/domain && go build ./...

test:
	go test -race ./...
	cd $(REPO_ROOT)/domain && go test -race ./...

cover:
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run --timeout=5m
	cd $(REPO_ROOT)/domain && golangci-lint run --timeout=5m

## sync-manifest refresca la copia embebida del manifiesto fundacional a partir
## del archivo canonico que fija NET-2 (ADR-006 punto 2, ADR-010 punto 4).
sync-manifest:
	cp $(CANONICAL_MANIFEST) $(EMBEDDED_MANIFEST)
	@echo "manifiesto embebido sincronizado desde $(CANONICAL_MANIFEST)"

## verify-manifest falla si la copia embebida divergio del archivo canonico.
verify-manifest:
	cd $(REPO_ROOT)/domain && go test ./manifest/...

## vendor materializa el paquete compartido `domain` dentro del modulo del
## chaincode. Es OBLIGATORIO antes de empaquetar: el `replace ../domain` de
## go.mod apunta fuera del directorio que el peer empaqueta.
vendor:
	go mod vendor
	@test -f vendor/github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/authorized-transfers.json \
		|| { echo "ERROR: la matriz de DES-3 no quedo vendorizada"; exit 1; }
	@test -f vendor/github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/manifest/organizations-manifest.json \
		|| { echo "ERROR: el manifiesto fundacional no quedo vendorizado"; exit 1; }
	go build -mod=vendor ./...

## package produce el artefacto de chaincode. ADR-008 (punto 5) y ADR-010
## (punto 4) exigen construirlo UNA sola vez y distribuirlo identico; el
## packageID y el checksum resultantes se versionan en el repositorio y NET-4
## (#23) los verifica con queryinstalled/queryapproved antes de invocar Init.
package: verify-manifest vendor
	peer lifecycle chaincode package $(PACKAGE_OUT) \
		--path $(CURDIR) \
		--lang golang \
		--label $(CC_LABEL)
	@echo "--- checksum del artefacto ---"
	@sha256sum $(PACKAGE_OUT)
	@peer lifecycle chaincode calculatepackageid $(PACKAGE_OUT)

clean:
	rm -rf vendor coverage.out $(CC_LABEL).tar.gz
