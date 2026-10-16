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

## El artefacto y el arbol que se empaqueta viven FUERA de chaincode/. Si el
## tar.gz se generara dentro del directorio que `--path` empaqueta, la segunda
## ejecucion de `make package` incluiria el artefacto de la primera y cambiaria
## el checksum y el packageID sin que el codigo hubiera cambiado -- justo lo
## contrario de lo que exigen ADR-008 (punto 5) y ADR-010 (punto 4). Por el
## mismo motivo no se empaqueta $(CURDIR) directamente: arrastraria coverage.out
## y cualquier archivo suelto del arbol de trabajo.
BUILD_DIR ?= $(REPO_ROOT)/build/chaincode
STAGE_DIR := $(BUILD_DIR)/$(CC_LABEL)
PACKAGE_OUT ?= $(BUILD_DIR)/$(CC_LABEL).tar.gz

## Contenido exacto que entra en el paquete. Es una lista explicita, no un `cp`
## del arbol entero, para que el artefacto sea funcion del codigo y de nada mas.
PACKAGE_SOURCES := go.mod go.sum main.go internal vendor

.PHONY: all build test cover lint vendor stage package package-reproducible \
	sync-manifest verify-manifest clean

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

## stage arma, desde cero, el arbol limpio que se empaqueta. Se reconstruye
## entero en cada corrida: un `stage` incremental reintroduciria por la ventana
## la no-reproducibilidad que este target existe para cerrar.
##
## El `go build -mod=vendor` final es la red de seguridad de la lista explicita
## de PACKAGE_SOURCES: si una fuente nueva quedara afuera, el build del arbol
## staged falla aca y no en el builder del peer. El `-o` apunta FUERA del arbol
## staged: un binario compilado adentro terminaria dentro del paquete.
stage: vendor
	rm -rf $(STAGE_DIR) $(BUILD_DIR)/stage-check
	mkdir -p $(STAGE_DIR) $(BUILD_DIR)/stage-check
	cp -R $(PACKAGE_SOURCES) $(STAGE_DIR)/
	cd $(STAGE_DIR) && go build -mod=vendor -o $(BUILD_DIR)/stage-check/ ./...
	rm -rf $(BUILD_DIR)/stage-check

## package produce el artefacto de chaincode. ADR-008 (punto 5) y ADR-010
## (punto 4) exigen construirlo UNA sola vez y distribuirlo identico; el
## packageID y el checksum resultantes se versionan en el repositorio y NET-4
## (#23) los verifica con queryinstalled/queryapproved antes de invocar Init.
package: verify-manifest stage
	mkdir -p $(BUILD_DIR)
	rm -f $(PACKAGE_OUT)
	peer lifecycle chaincode package $(PACKAGE_OUT) \
		--path $(STAGE_DIR) \
		--lang golang \
		--label $(CC_LABEL)
	@echo "--- checksum del artefacto ---"
	@sha256sum $(PACKAGE_OUT)
	@peer lifecycle chaincode calculatepackageid $(PACKAGE_OUT)

## package-reproducible empaqueta dos veces y falla si el packageID cambia.
## Es la comprobacion mecanica de la propiedad que ADR-008 punto 5 y ADR-010
## punto 4 dan por sentada al versionar el packageID en el repositorio.
package-reproducible:
	$(MAKE) package
	@cp $(PACKAGE_OUT) $(BUILD_DIR)/first.tar.gz
	@set -e; \
	first=$$(peer lifecycle chaincode calculatepackageid $(BUILD_DIR)/first.tar.gz); \
	$(MAKE) package >/dev/null; \
	second=$$(peer lifecycle chaincode calculatepackageid $(PACKAGE_OUT)); \
	rm -f $(BUILD_DIR)/first.tar.gz; \
	if [ "$$first" != "$$second" ]; then \
		echo "ERROR: el packageID no es reproducible: $$first != $$second"; exit 1; \
	fi; \
	echo "packageID reproducible: $$second"

clean:
	rm -rf vendor coverage.out $(BUILD_DIR)
