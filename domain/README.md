# Paquete compartido de reglas (`domain`)

Módulo Go `github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain`: la **única** implementación de las reglas de negocio que el chaincode y la baseline centralizada deben aplicar por igual. La paridad funcional queda garantizada por construcción y no por disciplina — no hay dos implementaciones que mantener consistentes, hay una sola que ambos binarios importan ([ADR-008](../docs/adr/008-transfer-matrix-distribution.md) punto 2, [ADR-012](../docs/adr/012-baseline-design.md) sección 1).

| Archivo | Contenido | Fuente de verdad |
|---|---|---|
| `authorized-transfers.json` | Matriz regulatoria de transferencias (embebida por `go:embed`). | Este archivo |
| `transfers.go` | `DecideTransfer(origen, destino)`: el algoritmo de decisión documentado más abajo. | `authorized-transfers.json` |
| `states.go` | Máquina de estados 1.0.0: catálogo de estados, eventos, actores y las 33 transiciones. | [ADR-001](../docs/adr/001-maquina-estados-medicamento.md) |
| `manifest/` | Manifiesto fundacional embebido que `Init` consume en el bootstrap. | `network/organizations-manifest.json` |

`states.go` **reproduce** la tabla de ADR-001, no la interpreta: `TestTransitionTableMatchesADR001` contrasta cada fila contra el Markdown del propio ADR y falla si divergen. Lo mismo hace `manifest/sync_test.go` con la copia embebida del manifiesto.

El layout del workspace Go (módulos separados con `replace`, y por qué el empaquetado del chaincode exige `go mod vendor`) está documentado en [`chaincode/README.md`](../chaincode/README.md).

---

# Matriz regulatoria de transferencias autorizadas — PFI SNT

Este directorio contiene la fuente única de verdad para validar pares **origen → destino** en transferencias ordinarias de custodia del Sistema Nacional de Trazabilidad de Medicamentos.

```text
authorized-transfers.json
authorized-transfers.schema.json
README.md
```

La matriz debe ser consumida tanto por el **chaincode** como por el **baseline centralizado**, evitando implementar reglas equivalentes mediante condicionales independientes.

## Alcance

El campo:

```json
"transferScope": "ORDINARY_CUSTODY_TRANSFER"
```

indica que estos archivos contemplan únicamente:

- tipos de agentes involucrados en transferencias ordinarias;
- pares origen → destino autorizados;
- referencias normativas que fundamentan cada par;
- casos prohibidos representativos;
- rechazo predeterminado de pares no declarados.

Quedan fuera del alcance de DES-3:

- estados del medicamento;
- precondiciones de custodia o habilitación;
- despacho y confirmación de recepción;
- devoluciones;
- dispensación al paciente;
- eventos extraordinarios;
- permisos, MSP y políticas de endoso.

Estas responsabilidades se resuelven en sus issues correspondientes.

## Archivos

### `authorized-transfers.json`

Contiene la matriz regulatoria.

Sus secciones principales son:

| Campo | Descripción |
|---|---|
| `schemaVersion` | Versión del formato y contenido de la matriz. |
| `rulesetId` | Identificador estable del conjunto de reglas. |
| `transferScope` | Limita el uso a transferencias ordinarias de custodia. |
| `regulatorySnapshot` | Fecha de corte del relevamiento normativo. |
| `defaultDecision` | Decisión aplicada a pares no autorizados expresamente. |
| `agentTypes` | Catálogo de agentes admitidos. |
| `normativeReferences` | Catálogo de normas citadas por las reglas. |
| `authorizedTransfers` | Pares origen → destino permitidos. |
| `prohibitedTransfers` | Casos prohibidos documentados explícitamente. |

### `authorized-transfers.schema.json`

JSON Schema Draft 2020-12 que valida la estructura de la matriz.

Controla:

- versión `1.0.0`;
- identificador del ruleset;
- alcance `ORDINARY_CUSTODY_TRANSFER`;
- códigos válidos de agentes;
- forma de las referencias normativas;
- forma de las reglas permitidas y prohibidas;
- obligatoriedad de al menos una referencia normativa por regla;
- rechazo de propiedades no declaradas.

## Agentes

| Código | Agente |
|---|---|
| `LABORATORY` | Laboratorio |
| `DISTRIBUTOR` | Distribuidora |
| `LOGISTICS_OPERATOR` | Operador logístico |
| `DRUGSTORE` | Droguería |
| `PHARMACY` | Farmacia |
| `HEALTHCARE_FACILITY` | Establecimiento asistencial |

El operador logístico se mantiene como agente diferenciado porque puede ejercer custodia física durante una distribución, aunque actúe por cuenta y orden de una distribuidora.

## Pares autorizados

| Origen | Destino |
|---|---|
| Laboratorio | Distribuidora |
| Laboratorio | Operador logístico |
| Laboratorio | Droguería |
| Laboratorio | Farmacia |
| Laboratorio | Establecimiento asistencial |
| Distribuidora | Operador logístico |
| Distribuidora | Droguería |
| Distribuidora | Farmacia |
| Distribuidora | Establecimiento asistencial |
| Operador logístico | Droguería |
| Operador logístico | Farmacia |
| Operador logístico | Establecimiento asistencial |
| Droguería | Droguería |
| Droguería | Farmacia |
| Droguería | Establecimiento asistencial |
| Farmacia | Establecimiento asistencial |

Cada entrada de `authorizedTransfers` incluye:

```json
{
  "id": "DRUGSTORE_TO_PHARMACY",
  "origin": "DRUGSTORE",
  "destination": "PHARMACY",
  "allowed": true,
  "normativeReferences": [
    "DEC_1299_1997_ART_4",
    "DISP_3683_2011_ANNEX_II",
    "DISP_963_2015_ART_15"
  ],
  "rationale": "La farmacia puede adquirir medicamentos a una droguería habilitada."
}
```

Los valores de `normativeReferences` deben existir en el catálogo superior del mismo archivo.

## Casos prohibidos

Se documentan explícitamente los casos más relevantes de venta hacia un eslabón superior:

- farmacia → laboratorio, distribuidora, operador logístico o droguería;
- establecimiento asistencial → laboratorio, distribuidora, operador logístico, droguería o farmacia.

Ejemplo:

```json
{
  "id": "PHARMACY_TO_UPSTREAM_AGENT",
  "origins": ["PHARMACY"],
  "destinations": [
    "LABORATORY",
    "DISTRIBUTOR",
    "LOGISTICS_OPERATOR",
    "DRUGSTORE"
  ],
  "allowed": false,
  "normativeReferences": [
    "DEC_1299_1997_ART_4",
    "DISP_3683_2011_ART_9"
  ],
  "reason": "Una farmacia no puede realizar una transferencia comercial ordinaria hacia un eslabón superior."
}
```

## Política de decisión

La matriz aplica:

```json
"defaultDecision": "DENY"
```

El algoritmo de consumo debe ser:

```text
1. Buscar coincidencia exacta en authorizedTransfers.
2. Si existe, autorizar y devolver el id de la regla.
3. Si no existe, comprobar si coincide con una prohibición explícita.
4. En cualquier otro caso, rechazar por defaultDecision.
```

Las prohibiciones explícitas sirven para producir errores más descriptivos. No reemplazan la política general de denegación.

Pseudocódigo:

```text
rule = findAuthorizedPair(origin, destination)

if rule exists:
    return ALLOW(rule.id)

prohibition = findExplicitProhibition(origin, destination)

if prohibition exists:
    return DENY(prohibition.id)

return DENY("DEFAULT_DENY")
```

## Uso desde chaincode y baseline

Ambos prototipos deben obtener la decisión desde la misma matriz:

```text
authorized-transfers.json
          │
          ├── chaincode
          └── baseline
```

Ante el mismo par de agentes, ambos deben devolver:

- la misma decisión;
- el mismo identificador de regla cuando corresponda;
- una razón equivalente de rechazo.

No se deben duplicar los pares mediante `if` o `switch` mantenidos manualmente en cada implementación.

## Versionado

La versión inicial es:

```json
"schemaVersion": "1.0.0"
```

No se declara un estado explícito. La aprobación queda determinada por el proceso de revisión y merge del pull request.

Una modificación posterior sobre una versión ya integrada debe actualizar la versión según el impacto del cambio:

- `PATCH`: corrección sin modificar pares;
- `MINOR`: incorporación compatible de pares o referencias;
- `MAJOR`: cambio incompatible de estructura o interpretación.