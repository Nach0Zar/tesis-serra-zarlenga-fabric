# Evidencia NET-9 — operaciones extraordinarias

## Estado

**Procedimiento implementado y reproducido localmente sobre un ledger limpio el
2026-09-23.** La identificación de la corrida y la limitación de trazabilidad
del working tree se registran en «Resultado reproducido».

NET-9 extiende la evidencia de NET-6 sin modificar las reglas del chaincode.
Usa las operaciones EXT ya integradas y la consulta pública
`GetLabInterventionHistory` incorporada por CC-10 (#123).

## Alcance

El harness `test/integration/extraordinary-endorsement-evidence.sh` demuestra:

- las cinco salidas extraordinarias desde `EN_TRANSITO` (T09 y T13–T16);
- el cierre histórico y la eliminación de la clave privada activa de la
  transferencia;
- la restauración de la SBE de reposo al emisor;
- la participación del regulador en toda operación extraordinaria que inicia;
- las operaciones `WithdrawFromMarket`, `Restock` y `FinalDisposition` de un
  laboratorio no custodio;
- el ciclo de `LabIntervention`, incluido el vencimiento derivado y el historial
  confirmado de una única clave por unidad;
- la diferencia entre una propuesta rechazada por lógica y una transacción
  ordenada pero inválida por política de endoso.

No implementa operaciones EXT, no modifica el contrato público ni los ADRs, no
repite la matriz Core de NET-6 y no usa el listener de NET-8.

## Precondiciones y ejecución

La corrida debe empezar sobre un ledger descartable limpio y usar exactamente
el paquete bloqueado por el repositorio. NET-5 y NET-6 se ejecutan antes en el
mismo ledger; NET-9 exige que exista exactamente un `result.json` válido de
NET-6.

```bash
docker compose -f network/compose.yaml down --volumes
./network/scripts/generate-crypto.sh
./network/network.sh up
./network/network.sh createChannel
./network/network.sh deployCC
./test/integration/pdc-evidence.sh
./test/integration/endorsement-evidence.sh
./test/integration/extraordinary-endorsement-evidence.sh
```

Los resultados crudos quedan en
`build/evidence/net-9/run-<token>/`, ignorados por Git. El token puede fijarse
con `SNT_NET9_RUN_TOKEN`; no se permite reutilizarlo ni borrar silenciosamente
una corrida existente.

## Escenarios de tránsito

| Escenario | Resultado esperado | Evidencia |
|---|---|---|
| T09 cuarentena en tránsito | Válida con Lab + Droguería + ANMAT | `transit-quarantine-*` |
| T13 vencimiento en tránsito | Válida con Lab + Droguería + ANMAT | `transit-expired-*` |
| T14 robo en tránsito | Válida con Lab + Droguería + ANMAT | `transit-stolen-*` |
| T15 extravío en tránsito | Válida con Lab + Droguería + ANMAT | `transit-lost-*` |
| T16 deterioro en tránsito | Válida con Lab + Droguería + ANMAT | `transit-damaged-*` |
| T09 sin emisor, receptor o regulador | Inválida en bloque, código Fabric 10 | `transit-quarantine-missing-*-*` |

Para cada salida válida, el validador decodifica el `HashedRWSet` de la
colección del par. Exige una escritura histórica bajo
`TransferOp+[GTIN,serial,txIdDespacho]` y una eliminación bajo
`TransferOpActive+[GTIN,serial]`; no acepta solamente un resumen del harness.
También decodifica el metadata write de la clave pública y exige SBE de
`LabMSP`, el emisor y custodio registrado. Operaciones posteriores endosadas
solo por ese emisor demuestran que la unidad no quedó bloqueada por la antigua
política de tránsito.

## Escenarios regulatorios

Se ejercitan `Quarantine`, `ReleaseQuarantine`, `WithdrawFromMarket`,
`Restock`, `ProhibitProduct`, `ReturnProduct` y `FinalDisposition` iniciadas por
ANMAT. Para cada transacción, el hash esperado de
`Participacion+[Unidad,GTIN,serial,txID]` debe aparecer en la colección
implícita `_implicit_org_AnmatMSP`.

La cuarentena fuera de tránsito se intenta además sin el regulador y sin el
custodio. Ambos casos deben ser transacciones incluidas en un bloque con código
10 (`ENDORSEMENT_POLICY_FAILURE`), no errores de simulación.

## Intervención de laboratorio no custodio

Sobre una unidad transferida a Droguería, el escenario comprueba:

1. autorización breve y vencimiento sin escritura sintética;
2. rechazo de la operación vencida durante simulación con
   `LAB_INTERVENTION_REQUIRED` y altura de canal sin cambios;
3. reemplazo posterior al vencimiento endosado solo por ANMAT;
4. revocación y reemplazo posterior, también solo por ANMAT;
5. reemplazo activo y consumo mediante `WithdrawFromMarket`;
6. nuevas autorizaciones y consumos mediante `Restock` y
   `FinalDisposition`;
7. para cada consumo, rechazo en bloque cuando falta Lab, ANMAT o Droguería;
8. marcadores simultáneos en `_implicit_org_LabMSP` y
   `_implicit_org_AnmatMSP`.

`GetLabInterventionHistory` debe devolver diez snapshots confirmados, en orden
cronológico, con estados:

```text
ACTIVA, ACTIVA, REVOCADA, ACTIVA, ACTIVA, CONSUMIDA,
ACTIVA, CONSUMIDA, ACTIVA, CONSUMIDA
```

El vencimiento es una condición derivada del timestamp de transacción. Por eso
la consulta inmediatamente posterior conserva una única entrada `ACTIVA` y no
agrega un estado persistido artificial.

## Integridad y privacidad de la evidencia

Cada caso ordenado se vincula por su `txID` exacto con el índice exacto dentro
del bloque y con la posición correspondiente de `TRANSACTIONS_FILTER`. El
validador rechaza un código 10 perteneciente a otra transacción, IDs repetidos,
canal distinto o resúmenes que no coincidan con el bloque decodificado.

`artifacts.json` contiene tamaño y SHA-256 de todos los archivos validados;
`result.json` es solo el resumen fail-fast. Los extractos sanitizados conservan
identificadores de transacción, nombres de colección y hashes, y verifican que
el payload privado suministrado al despacho o a los marcadores no aparezca.
Los hashes permiten detectar alteraciones de archivos; no son una firma de
autenticidad externa.

## Resultado reproducido

La corrida local `run-0923133438`, ejecutada el 2026-09-23 desde un ledger
limpio, validó 395 artefactos (1.006.316 bytes) y todas las assertions de
`result.json`. Usó:

- canal `snt-channel`;
- chaincode `snt`, secuencia 2;
- package ID
  `snt_1.0:ae1f239352a112f03405d673a253c727a2d4d95ef2d14f1510580366829d56b4`;
- working tree basado en el commit
  `7d6537548a856868e33231a90e8cf5c5fda7519a`.

La corrida también fue revalidada contra `artifacts.json` después de generada.
Luego se comprobó el E2E funcional y el reinicio idempotente de la red, del
canal y del despliegue del mismo paquete.

El campo `repositoryCommit` identifica el commit base porque esta validación se
hizo antes de crear un commit con los cambios de NET-9. Por lo tanto, esta
corrida demuestra el working tree local pero no constituye por sí sola una
referencia inmutable. La evidencia citable definitiva será la corrida del
workflow de la PR; los artefactos crudos locales permanecen deliberadamente
fuera de Git.
