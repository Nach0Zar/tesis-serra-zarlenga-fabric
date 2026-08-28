# Evidencia NET-6 — políticas de endoso del Core

## Estado

**Procedimiento implementado; ejecución final pendiente.**

Este documento no afirma todavía que las pruebas hayan pasado. La corrida
citable debe realizarse desde una red limpia después de integrar en `develop`
las implementaciones CC del Core y de actualizar en esta rama el paquete y su
lock. Los resultados crudos se escribirán bajo
`build/evidence/net-6/run-<token>/`, que está ignorado por Git.

La evidencia de operaciones extraordinarias fue separada a
[NET-9](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/97).

## Alcance Core

El harness `test/integration/endorsement-evidence.sh` cubre:

- bootstrap bajo unanimidad y rechazo de un creador no regulatorio;
- alta con marcador implícito del laboratorio;
- marcadores regulatorios reales del registro, además del marcador del alta;
- SBE de reposo del custodio;
- SBE de tránsito `AND(emisor,receptor)`;
- restauración al receptor después de `ReceiveTransfer`;
- restauración al emisor después de `RejectTransfer`;
- imposibilidad de sustituir al emisor por el regulador;
- dispensación bajo la SBE de la farmacia;
- política explícita de PDC y políticas de colecciones implícitas;
- diferencia entre rechazo de aplicación y transacción inválida;
- rechazo de recepción cuando un peer ejecuta una matriz divergente;
- historial completo y veredicto positivo de `VerifyTrace` como gate Core.

No cubre T09/T13–T16, eventos extraordinarios ni intervención de un
laboratorio no custodio.

## Precondiciones

1. NET-4/NET-5 y las CC del Core están integradas en `develop`.
2. La rama actual incorporó esos cambios con `git pull origin develop`.
3. El lock corresponde al paquete actual:

   ```bash
   ./network/network.sh package-lock
   git diff -- network/chaincode-package.lock
   ```

4. El material criptográfico fue generado y verificado.
5. La corrida comienza con un ledger descartable limpio. El borrado de
   volúmenes es deliberado y no forma parte de los scripts idempotentes.

## Ejecución

Desde la raíz del repositorio:

```bash
./network/scripts/generate-crypto.sh
./network/network.sh up
./network/network.sh createChannel
./network/network.sh deployCC
./test/integration/pdc-evidence.sh
./test/integration/endorsement-evidence.sh
```

`deployCC` produce primero la evidencia de bootstrap, porque esas pruebas solo
pueden ejecutarse mientras la secuencia 1 continúa sin inicializar. El harness
principal exige esos archivos y aborta si el ledger fue inicializado con una
versión anterior del script.

## Escenarios y capa que decide

| Escenario | Resultado esperado | Capa | Evidencia cruda |
|---|---|---|---|
| `Init` con solo ANMAT | Inválida en bloque, código Fabric 10 | Política de chaincode de secuencia 1 | `net-6/bootstrap/init-insufficient-*` |
| `Init` creado por otra organización | `REGULATORY_ONLY`, sin bloque nuevo | Lógica CC | `net-6/bootstrap/init-wrong-creator*` |
| `RegisterOrganization` solo por ANMAT | Válida | Política operativa + marcador regulatorio | `registry-regulator-only-*` |
| `RegisterOrganization` creado por Lab | `REGULATORY_ONLY`, sin bloque nuevo | Lógica CC | `registry-wrong-creator-*` |
| Marcadores reales de registro y alta | Hash de la colección implícita presente; payload ausente | PDC implícita | `registry-marker-sanitized.json`, `register-unit-marker-sanitized.json` |
| Metadatos SBE de alta, tránsito, recepción y rechazo | Principales MSP y umbral exactos | Validación por clave | `*-sbe.json`, `*-sbe-role-*.pb` |
| `RegisterUnit` creado por Lab y endosado solo por ANMAT | Inválida en bloque, código 10 | Colección implícita de Lab | `register-unit-regulator-only-*` |
| Alta duplicada | `UNIT_ALREADY_EXISTS`, sin bloque nuevo | Lógica CC | `register-unit-duplicate-*` |
| Recepción/rechazo con una sola parte | Inválida en bloque, código 10 | SBE de tránsito | `receive-one-party-*`, `reject-one-party-*` |
| Recepción con receptor + ANMAT, sin emisor | Inválida en bloque, código 10 | SBE de tránsito | `receive-without-sender-*` |
| Dispensación creada por farmacia y endosada solo por ANMAT | Inválida en bloque, código 10 | SBE de reposo | `dispense-regulator-only-*` |
| Despacho productivo | Nombre de PDC y hashes visibles; payload ausente | Hash de PDC en bloque | `dispatch-lab-drugstore-sanitized.json` |
| Escrituras PDC del probe | Rechazos y visibilidad esperados | Políticas explícitas e implícitas | `build/evidence/net-5/` |
| Recepción con matriz alterada en el receptor | No forma transacción; altura sin cambios | Simulación/endosos divergentes | `matrix-divergent-*` |
| Consultas del flujo completo | Historial consistente y `VerifyTrace.legitima=true` | Gate general CC-5/CC-7; no define endoso | `core-history.json`, `core-trace-verdict.json` |

El código 10 se lee desde `TRANSACTIONS_FILTER` del bloque decodificado y
corresponde a `ENDORSEMENT_POLICY_FAILURE`. El texto de la CLI se conserva,
pero no es la única prueba.

## Paquete divergente

La prueba no modifica la matriz versionada. Construye bajo `build/test/` una
copia del paquete staged y cambia únicamente el `ruleId`
`LABORATORY_TO_DRUGSTORE`. La organización receptora aprueba localmente ese
`packageID` para la misma definición de secuencia 2.

La recepción debe fallar antes de llegar al ledger porque las simulaciones no
coinciden. Un `trap` restaura el `packageID` canónico incluso ante errores;
el harness verifica `queryapproved` y completa luego la recepción con el
paquete correcto.

## Resultado final pendiente

Después de la corrida final, esta sección debe incorporar:

- commit y `packageID` ejecutados;
- token y ruta de la corrida;
- tabla de escenarios con resultado real;
- hashes/extractos sanitizados citables;
- limitaciones observadas.

Hasta entonces, ningún criterio que diga “DEMOSTRADO” debe marcarse como
satisfecho en NET-6.
