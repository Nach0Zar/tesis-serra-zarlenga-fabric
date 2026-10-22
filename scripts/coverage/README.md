# Cobertura de tests

Cada CI exige **85 % de cobertura** sobre la lógica de su proyecto. El mínimo
se aplica a la lógica de negocio y de procesamiento, no al código que sólo
opera contra la red Fabric o Docker: ese código se valida con los tests de
integración y con las corridas reales, cuya evidencia queda publicada.

| Proyecto | Alcance medido | Fuera del alcance | Dónde se verifica |
|---|---|---|---|
| `chaincode` | `internal/...` | `main.go`, que sólo arranca el servidor | `chaincode-ci.yml` |
| `domain` | todo el módulo | — | `chaincode-ci.yml` |
| `client` | `internal/app`, `internal/config`, `internal/contracterr`, `internal/runmeta` | el adaptador gRPC `internal/fabric` y los `main` de `cmd/` | `client-ci.yml` |
| `baseline` | `internal/...`, con los tests de integración contra PostgreSQL | los `main` de `cmd/` | `baseline/tests/schema-integration.sh` (`baseline-ci.yml`) |
| `benchmarks/caliper` | `coverage.include` de su `package.json` | lo listado en `coverage.excluded`, con su motivo | `caliper-ci.yml` |
| `benchmarks/baseline` | `coverage.include` de su `package.json` | lo listado en `coverage.excluded`, con su motivo | `baseline-benchmark-ci.yml` |
| `benchmarks/analysis` | todo `src/` | — | `analysis-ci.yml` |

Para medirla localmente:

```bash
cd benchmarks/caliper && npm run test:coverage
```

```bash
cd client && go test -coverpkg=./internal/app/...,./internal/config/...,./internal/contracterr/...,./internal/runmeta/... -coverprofile=coverage.out ./... && ../scripts/coverage/check-go.sh coverage.out 85
```

`check-go.sh` lee un perfil de `go test` y `check-lcov.js` lee el lcov de
`node --test`. Los dos fallan si la cobertura queda por debajo del mínimo.

## El paquete del chaincode

`chaincode/internal` se copia completo al paquete del chaincode, tests
incluidos. Lo mismo pasa con lo que el chaincode importa de `domain`, que se
vendoriza: el paquete raíz y `domain/manifest`. Un cambio en esos archivos,
aunque sea un test, cambia el package ID versionado en
`network/chaincode-package.lock`. Ese package ID ancla los snapshots y la
evidencia de las mediciones. Los tests nuevos de esos paquetes requieren
reempaquetar el contrato y van en su propio cambio.
