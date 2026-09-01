# Evidencia NET-6 — políticas de endoso del Core

## Estado

**Ejecución final completada el 2026-09-01.**

La corrida citable se realizó desde un ledger limpio después de integrar las
implementaciones CC del Core y regenerar el paquete. Los resultados crudos se
encuentran bajo `build/evidence/net-6/run-0901151321/`, ignorado por Git.

## Defecto detectado y resuelto durante la integración

La red limpia desplegó correctamente el paquete
`snt_1.0:7606f16248eaa46c63263fe1953b5898e130cbbfa6909847388a509ca2bafb43`
y completó NET-5. La corrida oficial `run-0901144336` se detuvo en el gate de
CC-7: `VerifyUnit` devolvió `SECUENCIA_INVALIDA` para una unidad legítima en
`EN_TRANSITO`.

La causa está aislada y no pertenece a NET. Desde Fabric 2.0,
`GetHistoryForKey` entrega las modificaciones desde la más nueva hacia la más
antigua. El mock de CC-7 las acumula en orden cronológico y
`verifyCustodyChain` interpreta el primer elemento como el alta. En la red
real, por lo tanto, interpreta `EN_TRANSITO` como origen en lugar de
`EN_LABORATORIO`. El contrato de la plataforma documenta ese orden en
[`ChaincodeStubInterface.GetHistoryForKey`](https://github.com/hyperledger/fabric-chaincode-go/blob/main/shim/interfaces.go).

La corrida diagnóstica `run-0901144819` permitió continuar después de registrar
ese veredicto y completó los demás escenarios NET-6, incluida la divergencia de
matriz y la restauración del paquete canónico. No generó `result.json` y no es
evidencia aprobatoria. Reinicio, `createChannel`, `deployCC` y `verify` fueron
idempotentes después de la corrida.

Con autorización explícita, la integración corrigió el defecto en el carril CC:
`readUnitHistory` normaliza el orden real de Fabric a cronológico, el mock ahora
reproduce el orden de Fabric y una prueba de regresión protege ambas
semánticas. La corrección compatible quedó documentada como contrato `2.7.1`,
sin cambiar firmas, tipos, schemas ni códigos públicos. Luego se regeneró el
paquete y se repitió el procedimiento completo desde un ledger limpio.

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
- veredicto positivo de `VerifyUnit` antes de la recepción e historial
  completo como gates de CC-7 y CC-5.

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
| Metadatos SBE de alta, tránsito, recepción y rechazo | Principales MSP, rol `PEER` y umbral exactos | Validación por clave | `*-sbe.json` |
| `RegisterUnit` creado por Lab y endosado solo por ANMAT | Inválida en bloque, código 10 | Colección implícita de Lab | `register-unit-regulator-only-*` |
| Alta duplicada | `UNIT_ALREADY_EXISTS`, sin bloque nuevo | Lógica CC | `register-unit-duplicate-*` |
| Recepción/rechazo con una sola parte | Inválida en bloque, código 10 | SBE de tránsito | `receive-one-party-*`, `reject-one-party-*` |
| Recepción con receptor + ANMAT, sin emisor | Inválida en bloque, código 10 | SBE de tránsito | `receive-without-sender-*` |
| Dispensación creada por farmacia y endosada solo por ANMAT | Inválida en bloque, código 10 | SBE de reposo | `dispense-regulator-only-*` |
| Despacho productivo | Nombre de PDC y hashes visibles; payload ausente | Hash de PDC en bloque | `dispatch-lab-drugstore-sanitized.json` |
| Escrituras PDC del probe | Rechazos y visibilidad esperados | Políticas explícitas e implícitas | `build/evidence/net-5/` |
| Recepción con matriz alterada en el receptor | No forma transacción; altura sin cambios | Simulación/endosos divergentes | `matrix-divergent-*` |
| Autenticidad previa a recepción | `VerifyUnit.autentica=true` en `EN_TRANSITO` | Gate general CC-7; no define endoso | `core-unit-verdict.json` |
| Historial del flujo completo | Seis o más snapshots confirmados | Gate general CC-5; no define endoso | `core-history.json` |

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

## Resultado final

- estado del merge ejecutado: `HEAD d1cc9700ba56bde26962e0b2fae0cd1712430ae9`
  más `MERGE_HEAD 06ebf1c` y la corrección CC-7 aún no commiteada;
- `packageID`:
  `snt_1.0:d6659e4907274adff5695c9de33d26a02e4f790f62494334c8f67018aef705aa`;
- corrida: `run-0901151321`;
- resultado: las 20 aserciones del harness son verdaderas;
- autenticidad: `autentica=true`, estado `EN_TRANSITO` y cuatro checks `OK`;
- NET-5 se repitió previamente sobre el mismo ledger limpio, incluido el caso
  de reconciliación de datos privados.

El campo `repositoryCommit` del artefacto crudo identifica `HEAD`; como la
ejecución ocurrió durante un merge todavía abierto, este reporte registra
además `MERGE_HEAD` y el `packageID`, que identifica de forma determinística el
chaincode efectivamente instalado. La evidencia deberá repetirse si cambia el
árbol antes del commit final.
