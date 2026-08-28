# Evidencia sanitizada de NET-5

Ejecucion local: 2026-08-28. Esta evidencia usa el chaincode descartable `pdc-probe`; valida semantica de Fabric y la configuracion de red, no implementa las transferencias de CC-3.

## Generacion

```bash
python3 network/scripts/generate-collections.py
python3 network/scripts/generate-collections.py --check
python3 -m unittest discover -s network/tests -v
```

Resultado: 10 colecciones explicitas deterministicas. Cada una representa un par no ordenado autorizado del dataset, incluye a ambas partes y ANMAT como miembros, y excluye a ANMAT de `endorsementPolicy`.

## Prueba de privacidad

```bash
./test/integration/pdc-evidence.sh
```

- Coleccion ejercitada: `transfer_DrogueriaMSP_FarmaciaMSP`.
- PackageID del probe: `pdc_probe_1.0:28383fca312b2e88bbb5d2246fd8ab712fc93aac39925a4fbc4f48b1b4a88e38`.
- El estado publico minimo fue consultado desde las siete organizaciones.
- Drogueria, farmacia y ANMAT leyeron el dato privado; `DistribuidorMSP` recibio denegacion de lectura por no pertenecer a la coleccion.
- La farmacia estuvo detenida durante el despacho. Tras reiniciarla obtuvo el dato privado mediante reconciliacion.
- En los bloques 32, 36 y 40 decodificados aparece el nombre de la coleccion en claro y no aparece el payload privado; los dos ultimos corresponden a reejecuciones posteriores a reinicios de la red.
- El extracto citable del bloque 40 se versiona en [`NET-5-block-40.json`](NET-5-block-40.json). Conserva numero y hashes de bloque, `txID`, timestamp, chaincode, nombre de coleccion, hashed rwset y hash del private rwset. SHA-256 del archivo: `4dba017c39ea34d9f8500a0bb18e66cad371e0cdaa74170d7399faf51b9acfe8`.
- Una escritura endosada solamente por ANMAT fue invalidada con `ENDORSEMENT_POLICY_FAILURE` y no modifico el estado publico.
- Las colecciones implicitas no aparecen en `collections_config.json`: una escritura del propietario fue valida, un tercero no pudo leerla y una escritura del no propietario fue invalidada con `ENDORSEMENT_POLICY_FAILURE`.

Los bloques binarios, el bloque decodificado, payloads y salidas completas se conservan exclusivamente en `build/evidence/net-5/`. El script produce ademas `sanitized-block-excerpt.json` mediante una lista cerrada de campos. El informe y el extracto versionado no contienen certificados, claves, secretos ni el valor privado utilizado.

## Onboarding

Agregar una organizacion custodial requiere actualizar el manifiesto y la configuracion de canal correspondiente, regenerar `collections_config.json`, reconstruir y bloquear el paquete, y aprobar una nueva secuencia lifecycle con la configuracion regenerada antes de registrar la organizacion en el ledger.
