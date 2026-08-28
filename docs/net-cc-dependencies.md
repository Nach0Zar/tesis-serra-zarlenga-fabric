# Dependencias entre las issues NET y CC

Este documento registra qué evidencia o artefacto de red depende de una
implementación del chaincode. Su objetivo es impedir que una issue NET simule
comportamiento todavía no integrado o modifique código perteneciente al carril
CC para poder cerrar una prueba.

Fuentes de alcance: issues NET-1 a NET-9, CC-1 a CC-8, ADR-006, ADR-007,
ADR-008 y ADR-010. La issue [NET-6](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/25)
cubre el flujo Core; [NET-9](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/97)
concentra la evidencia que solo existe después de las operaciones EXT.

## Clasificación

| Código | Relación |
|---|---|
| `F` | Dependencia funcional: el criterio NET no puede ejecutarse sin esa implementación CC. |
| `I` | Integración o empaquetado: el cambio CC altera el artefacto desplegado, sus comandos o su `packageID`. |
| `E` | Evidencia: la configuración existe sin CC, pero la afirmación requiere ejercitar el chaincode real. |
| `Q` | Gate de calidad: no aporta comportamiento nuevo, pero debe estar verde antes de la integración final. |
| `R` | Relación inversa: CC consume una decisión NET; no bloquea la implementación de NET. |
| `—` | Sin dependencia directa. |

## Matriz exhaustiva

| NET | CC-1 | CC-2 | CC-3 | CC-4 | CC-5 | CC-6 | CC-7 | CC-8 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| NET-1 | I | — | — | — | — | — | — | — |
| NET-2 | — | — | — | — | I | — | — | — |
| NET-3 | E | E | — | — | — | — | — | — |
| NET-4 | F | I | I | I | I | I | I | I¹ |
| NET-5 | E | E | F/E | — | — | — | R | R |
| NET-6 | F/E | F/E | F/E | F/E | Q | Q | Q | — |
| NET-7 | I | I | I | I | I | I | I | — |
| NET-8 | — | I² | — | — | — | — | — | — |
| NET-9 | F/E | — | — | — | F³ | Q | — | — |

1. CC-8 cambia el paquete cuando se integre, pero pertenece a una milestone
   posterior y no bloquea NET-6/NET-7.
2. CC-2 aporta el patrón de emisión ya disponible. Las irregularidades que
   consume NET-8 dependen realmente de las operaciones EXT y de su catálogo de
   eventos, no de una ampliación de CC-2.
3. El contrato actual solo expone historial de unidades. La consulta aprobada
   del historial de `LabIntervention` debe resolverse en CC-5/DES-5 antes de
   ejecutar NET-9; una issue NET no puede agregarla.

## Dependencias por issue NET

| Issue | Dependencia y artefacto consumido | Criterio o evidencia afectada | Acción de integración |
|---|---|---|---|
| NET-1 | CC-1 resuelve `cid.GetMSPID()` y `snt.role`. | Los ECert de `User1` deben presentar el atributo que consume el chaincode. La generación criptográfica no necesita compilar CC. | Revalidar una invocación por cada rol después del merge de CC-1. |
| NET-2 | CC-5 usa claves compuestas y no requiere rich queries. | Confirma que LevelDB satisface la superficie Core; `configtx.yaml` se genera independientemente. | Repetir consultas Core, sin cambiar de state database desde NET. |
| NET-3 | CC-1 define el esquema del marcador y CC-2 lo escribe en el alta. | Presupuesto del costo privado y del hash público. El arranque y la medición en reposo no dependen de CC. | Medir carga de marcadores en el perfil que corresponda, sin atribuirla a la corrida de reposo. |
| NET-4 | CC-1 aporta módulo Go, manifiesto/matriz embebidos, `Init` y registro. Toda CC posterior modifica el paquete. | Lifecycle en dos secuencias, seed regulatorio, checksum y `packageID` comunes. | Tras cada integración CC legítima, ejecutar `package-lock` y repetir `queryinstalled`, `queryapproved` y `querycommitted`. |
| NET-5 | CC-1/CC-2 aportan marcadores implícitos; CC-3 crea y cierra `TransferOp` en la PDC explícita. | Privacidad real de la transferencia y rechazo de una escritura privada endosada solo por el regulador. | Mantener el probe para aislar propiedades de Fabric y añadir la transacción productiva de CC-3 como evidencia complementaria. |
| NET-6 | CC-1: helpers, `Init` y registro; CC-2: alta/SBE/marcador; CC-3: despacho, recepción, rechazo, PDC y restauración; CC-4: dispensación. | Todos los casos de endoso del flujo Core. CC-5 y CC-7 permiten observar historial y autenticidad, y CC-6 aporta el gate unitario; se consumen como gate general (`Q`) sin atribuirles políticas de NET-6. | Esperar los merges Core, traer `develop`, regenerar paquete y ejecutar la matriz de endosos sobre una red limpia. |
| NET-7 | La documentación operacional debe reflejar el paquete y la superficie Core definitivos. | README, comandos de demo y resultados reproducibles. | Actualizar ejemplos después del último merge CC Core; no documentar stubs como funciones disponibles. |
| NET-8 | El listener necesita eventos confirmados y bloques inválidos. CC-2 solo demuestra el patrón de emisión. | La demo de irregularidades depende de EXT-1…EXT-3 y del catálogo de eventos acordado con DES-5. | No bloquear NET-6/NET-7 ni prometer eventos extraordinarios antes de EXT. |
| NET-9 | CC-1 aporta autorización/SBE/marcadores y CC-5/DES-5 debe aportar historial; las operaciones que consumen esos mecanismos son EXT. | Restauración T09/T13–T16, eventos regulatorios e intervención de laboratorio no custodio. | Ejecutar únicamente después de integrar las EXT relacionadas y extender el harness de NET-6 sin copiar lógica del chaincode. |

## Estado informativo al 2026-08-28

- CC-1 y CC-2 tienen sus implementaciones integradas en `develop`.
- CC-3 a CC-6 existen como ramas apiladas y todavía deben recorrer su revisión
  e integración.
- CC-7 permanece pendiente dentro del Core.
- CC-8 pertenece a la milestone posterior y no es gate de NET-6/NET-7.
- NET-9 es la issue #97 y pertenece a la misma milestone que EXT-1…EXT-8.

Este estado no reemplaza GitHub como fuente de verdad. Debe comprobarse otra
vez antes de actualizar la rama NET.

## Secuencia de integración y merge

1. Finalizar y mergear NET-4/NET-5.
2. Mergear en `develop` las CC del milestone Core.
3. Con el árbol limpio, ejecutar en `feature/politicas-endoso-documentacion`:

   ```bash
   git pull origin develop
   ```

4. Conservar el chaincode recibido desde `develop` como autoritativo. Los
   conflictos de lógica CC no se resuelven implementando parches desde NET.
5. Regenerar y revisar el lock del paquete, desplegar desde una red limpia y
   repetir toda la evidencia Core.
6. Mientras la PR de NET-4/NET-5 esté abierta, usarla como base de la PR
   apilada. Después de su merge, retargetear NET-6/NET-7 a `develop`.
7. Mergear la PR de NET-6/NET-7 al final del Core. NET-9 tendrá su propia rama
   y PR después de las implementaciones EXT.

Los resultados crudos viven bajo `build/evidence/` y no se versionan. Solo se
versionan el procedimiento, los resúmenes sanitizados y las conclusiones que
hayan sido ejecutadas realmente.
