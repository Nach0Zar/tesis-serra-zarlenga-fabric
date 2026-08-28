# Evidencia sanitizada de NET-4

Ejecucion local: 2026-08-28. Commit base: `08c84c2366cf57a27b4c70856f027d70abd7b23a`.

## Entorno

- Hyperledger Fabric CLI y nodos: `2.5.16`.
- Fabric CA: `1.5.17`.
- Docker Engine: `29.7.2`.
- Docker Compose: `5.4.0`.
- Canal: `snt-channel`; chaincode: `snt`.

## Comandos ejecutados

```bash
python3 network/scripts/validate-organizations-manifest.py
python3 network/scripts/generate-collections.py --check
./network/network.sh down
./network/network.sh up
./network/network.sh createChannel
./network/network.sh deployCC
./network/network.sh verify
./network/network.sh restart
./network/network.sh createChannel
./network/network.sh deployCC
./network/network.sh verify
./network/network.sh down
```

Los binarios se resolvieron con `SNT_FABRIC_BIN_DIR` y el `core.yaml` de la misma distribucion mediante `SNT_FABRIC_CFG_PATH`.

## Resultados

- Los orderers de `AnmatMSP`, `LabMSP` y `DrogueriaMSP` informaron `consensusRelation=consenter` y `status=active`. La obtencion del bloque mas reciente por el ordering service fue exitosa.
- Los siete peers se unieron al canal y respondieron `peer channel getinfo`.
- El paquete se construyo una sola vez para el despliegue. Los siete `queryinstalled` devolvieron el mismo valor:

  `snt_1.0:194b7b60cff60fdb2d428d583711a1b2e30760a66891fd1ff40380feca625570`
- El SHA-256 del tar coincide con la parte hash del `packageID` y con `network/chaincode-package.lock`.
- Secuencia 1: version `1.0`, `init_required=true`, 10 colecciones, politica `AND` de las siete organizaciones. Las siete aprobaciones fueron consultadas antes de `Init`.
- `Init` fue invocado sin argumentos por `AnmatMSP/User1`, cuya identidad tiene `snt.role=regulatory-admin`. Una simulacion posterior devolvio `ALREADY_INITIALIZED` para `AnmatMSP`.
- Secuencia 2: mismo paquete y version, `sequence=2`, sin `init-required`, con la politica operacional derivada del manifiesto y la matriz.
- `RegisterOrganization` registro laboratorio, drogueria, distribuidor, farmacia, centro medico y financiador. El regulador fue sembrado por `Init`.
- El ciclo `down -> up` preservo el canal, las instalaciones y el registro mediante los 10 volumenes nombrados. La reejecucion posterior omitio el estado ya alcanzado y acepto solamente los duplicados tipificados.
- Tras `restart`, los tres orderers y los siete peers volvieron a estado saludable; `createChannel`, `deployCC` y `verify` pasaron nuevamente. El `down` final dejo cero servicios Compose y conservo los 10 volumenes.
- La revision posterior de findings incorporo una comparacion semantica de la definicion: propiedades y politicas de las 10 colecciones, mas la politica de endoso del chaincode derivada de manifiesto y matriz. El comparador paso contra `querycommitted` de la secuencia 2 preservada y sus ocho pruebas unitarias cubrieron drift de escalares, membresia, endoso y orden.
- El flujo limpio ahora guarda `pre-init-gate.txt` y exige que `ReadUnit` falle especificamente por `--init-required` antes de invocar `Init`. El ledger local preservado ya estaba inicializado al incorporar este control; su ejecucion limpia queda cubierta por el job de integracion que crea la red desde cero.

Los JSON completos de `queryinstalled`, `queryapproved`, readiness, politicas decodificadas y definiciones, junto con bloques y logs, se conservan solamente en `build/evidence/net-4/`.

## Recuperacion

Si se confirma la secuencia 1 con un paquete incorrecto antes de `Init`, se debe construir y versionar un nuevo paquete, aprobarlo en la secuencia siguiente disponible y repetir el bootstrap antes de pasar a la definicion operacional. Si un `Init` incorrecto ya fue confirmado, el script se detiene: la recuperacion requiere reiniciar el ledger descartable o una decision de gobernanza; no sustituye silenciosamente al regulador.
