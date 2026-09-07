# Cliente Fabric Gateway

Cliente CLI genérico para invocar y consultar el chaincode `snt` mediante
[Fabric Gateway](https://github.com/hyperledger/fabric-gateway). Implementa el alcance
de CLI-1 (#34) contra el contrato público congelado en
[`docs/api-contract.md`](../docs/api-contract.md) (v2.7.1).

No incluye comandos de negocio ni el recorrido de demostración, que corresponden a
CLI-2 (#35).

## Requisitos

- Go 1.23 o posterior.
- Red levantada, canal `snt-channel` creado y chaincode `snt` desplegado.
- Material criptográfico generado bajo `network/organizations/`.
- Identidades `User1` enroladas por NET-1.

El módulo usa `fabric-gateway` v1.8.0, compatible con la versión de Go
declarada por el repositorio.

## Construcción y pruebas

Desde `client/`:

```bash
go build ./...
go test -race ./...
go vet ./...
```

## Uso

```text
snt-client query  --org <org> --function <name> [--arg <value> ...]
snt-client invoke --org <org> --function <name> [--arg <value> ...]
```

`--arg` es repetible y conserva el orden exigido por la función pública.
Los valores complejos se pasan como un único argumento JSON.

Ejemplos desde `client/`:

```bash
go run ./cmd/snt-client query \
  --org anmat \
  --function QueryUnitsByGTIN \
  --arg 07791234567898

go run ./cmd/snt-client query \
  --org farmacia \
  --function ReadUnit \
  --arg 07791234567898 \
  --arg SN-0001-ABCD

go run ./cmd/snt-client invoke \
  --org lab \
  --function RegisterUnit \
  --arg '{"gtin":"07791234567898","numeroSerie":"SN-0001-ABCD","lote":"L2026-014","fechaVencimiento":"2027-12-31"}'
```

Las respuestas se escriben en stdout. Si el payload es JSON, se indenta sin
modificar su contenido.

## Organizaciones e identidades

`--org` admite las cuatro identidades requeridas por CLI-1:

| Valor | MSP | Gateway local | Identidad |
|---|---|---|---|
| `anmat` | `AnmatMSP` | `localhost:7051` | `User1@anmat.snt.local` |
| `lab` | `LabMSP` | `localhost:8051` | `User1@lab.snt.local` |
| `drogueria` | `DrogueriaMSP` | `localhost:9051` | `User1@drogueria.snt.local` |
| `farmacia` | `FarmaciaMSP` | `localhost:11051` | `User1@farmacia.snt.local` |

El MSP y el hostname del peer se leen de
`network/organizations-manifest.json`. El certificado, la clave privada
y la CA TLS se resuelven desde el layout generado por NET-1. La conexión valida
TLS contra la CA del peer de la organización seleccionada.

El Gateway descubre y coordina los endosos requeridos por las políticas vigentes.
Para entornos que no usan los puertos locales se pueden sobrescribir
`--gateway-endpoint` y `--tls-server-name`. También están
disponibles `--channel`, `--chaincode`, `--timeout`
y `--repo-root`.

## Transient data

Las claves privadas definidas por DES-5 (`commercial`,
`destinatario` y `devolucion`) se suministran mediante un
objeto JSON en un archivo:

```json
{
  "destinatario": {
    "gln": "7791234500048"
  }
}
```

```bash
go run ./cmd/snt-client invoke \
  --org drogueria \
  --function DispatchTransfer \
  --arg '{"gtin":"07791234567898","numeroSerie":"SN-0001-ABCD"}' \
  --transient-file /ruta/segura/transient.json
```

El valor `--transient-file -` lee el objeto desde stdin. No existe una
opción para incluir transient data directamente como argumento de proceso.

## Errores

Los errores del chaincode se conservan en stderr con el envelope de DES-5:

```json
{
  "code": "UNIT_NOT_FOUND",
  "message": "La unidad no existe.",
  "details": {
    "key": "..."
  }
}
```

Los fallos de transporte o plataforma no clasificables se expresan como
`INTERNAL_ERROR`. En esos casos, `details` incluye la etapa,
una clasificación operativa y la causa original normalizada y truncada a 1024
caracteres. Los rechazos de validación de Fabric incluyen además
`validationCode` y `transactionId`.

Los códigos de salida son `0` para éxito,
`1` para fallos de ejecución y `2` para uso inválido de la CLI.
