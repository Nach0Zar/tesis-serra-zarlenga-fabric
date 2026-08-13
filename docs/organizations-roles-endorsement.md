# DES-6: Organizaciones, MSP, roles, ABAC y politicas de endoso

- **Estado**: Propuesto
- **Fecha**: 2026-08-09
- **Autores**: Serra, Zarlenga

---

## Contexto

DES-6 define como deben mapearse las organizaciones Fabric, las identidades MSP,
los roles intra-organizacion, las reglas ABAC y las politicas de endoso del
prototipo SNT. Es una decision de diseno: no implementa `configtx.yaml`, material
criptografico, colecciones privadas, chaincode, baseline ni contrato publico.

La issue #12 fue creada antes de ADR-003 y lista siete MSP de referencia:
`LabMSP`, `DrogueriaMSP`, `FarmaciaMSP`, `CentroMedicoMSP`,
`DistribuidorMSP`, `FinanciadorMSP` y `AnmatMSP`. Esa lista se conserva como
dataset minimo de ejemplo para el prototipo, pero no como modelo de una MSP
compartida por categoria. ADR-003 gobierna esta decision y establece que cada
establecimiento identificado por GLN o CUFE debe operar como una organizacion
Fabric independiente, con su propia MSP.

Esta decision debe mantenerse compatible con:

- [ADR-001](adr/001-maquina-estados-medicamento.md), que define actores logicos y transiciones de estado, pero no MSP ni
  autorizacion tecnica.
- [ADR-002](adr/002-topologia-canales.md), que define canal unico, estado minimo visible en el canal y datos
  comerciales o documentales en Private Data Collections.
- [ADR-003](adr/003-establishment-identity-gln-cufe.md), que define identidad de establecimientos mediante una organizacion
  Fabric por establecimiento y custodio persistido como GLN/CUFE canonico.
- [DES-3](../domain/authorized-transfers.json), que define la matriz origen -> destino para transferencias ordinarias,
  sin permisos, MSP ni politicas de endoso.

## Decision

Se adopta el siguiente modelo:

1. Cada establecimiento custodial habilitado por el SNT se representa como una
   organizacion Fabric independiente y posee una MSP propia.
2. Los nombres `LabMSP`, `DrogueriaMSP`, `FarmaciaMSP`, `CentroMedicoMSP` y
   `DistribuidorMSP` son ejemplos del dataset minimo, no MSP compartidas por
   todos los laboratorios, droguerias, farmacias, centros medicos o
   distribuidores.
3. `AnmatMSP` es una organizacion regulatoria diferenciada. No representa un
   custodio ordinario de medicamentos; su alcance es auditoria, administracion
   regulatoria del prototipo y eventos extraordinarios.
4. `FinanciadorMSP` es una organizacion no custodial. Su comportamiento concreto
   queda reservado para DES-10 y CC-8; DES-6 solo fija que no participa en la
   matriz de transferencias ordinarias ni puede actuar como custodio.
5. La identidad de establecimiento se obtiene desde `cid.GetMSPID()` y se
   resuelve contra el registro organizacion-establecimiento de ADR-003. DES-6 no
   usa atributos X.509 para resolver GLN/CUFE.
6. El unico atributo ABAC definido por DES-6 es `snt.role`, utilizado para roles
   intra-organizacion. Cuando una clase de operacion exige rol, la ausencia o un
   valor desconocido del atributo debe rechazar la autorizacion.
7. La politica de endoso se define como dinamica por participantes: las
   operaciones que modifican custodia deben involucrar a las organizaciones que
   tienen capacidad independiente de validar la operacion; ANMAT se suma en
   eventos regulatorios o extraordinarios, pero no como coendosante obligatorio
   de toda transaccion.

## Organizaciones y categorias

El registro organizacion-establecimiento de ADR-003 es el contrato de diseno para
las organizaciones custodiales. Sus campos minimos son:

| Campo | Uso |
|---|---|
| `mspId` | Identificador interno de la organizacion Fabric del establecimiento. |
| `id` | Identificador regulatorio del establecimiento. |
| `idType` | `GLN` o `CUFE`. |
| `agentType` | Categoria normativa del establecimiento. |
| `active` | Indica si el establecimiento puede operar como actor habilitado. |

Para custodios, `agentType` debe reutilizar el catalogo vigente de DES-3:

| `agentType` | Actor de dominio | Ejemplo de MSP en dataset minimo |
|---|---|---|
| `LABORATORY` | Laboratorio | `LabMSP` |
| `DISTRIBUTOR` | Distribuidora | `DistribuidorMSP` |
| `LOGISTICS_OPERATOR` | Operador logistico | Sin ejemplo obligatorio en la lista original de #12. Si se incluye, debe ser otra MSP de establecimiento. |
| `DRUGSTORE` | Drogueria | `DrogueriaMSP` |
| `PHARMACY` | Farmacia | `FarmaciaMSP` |
| `HEALTHCARE_FACILITY` | Establecimiento asistencial | `CentroMedicoMSP` |

Las organizaciones no custodiales no deben resolverse como custodios de assets:

| MSP | Tipo | Alcance DES-6 |
|---|---|---|
| `AnmatMSP` | Autoridad regulatoria | Auditoria, administracion regulatoria y eventos extraordinarios. |
| `FinanciadorMSP` | Organismo financiador | Reserva de lectura/verificacion futura para DES-10/CC-8; sin transferencia ni custodia. |

DES-6 no define una forma permanente de nombrar MSP productivas. El nombre MSP es
un identificador tecnico de Fabric; el identificador persistido en assets y
eventos de dominio debe ser el GLN/CUFE canonico definido por ADR-003.

## Roles y ABAC

El atributo `snt.role` debe emitirse en el enrollment certificate cuando se use
para autorizar una clase de operacion. Los valores definidos son:

| `snt.role` | Organizaciones aplicables | Permisos de diseno |
|---|---|---|
| `operator` | Organizaciones custodiales activas | Operaciones ordinarias permitidas por `agentType`, custodia actual, estado ADR-001 y matriz DES-3. |
| `auditor` | `AnmatMSP` y, si corresponde, identidades internas de consulta | Lecturas y auditoria segun visibilidad ADR-002, sin modificar estado. |
| `regulatory-admin` | `AnmatMSP` | Alta/baja logica del registro, cambios de habilitacion y eventos regulatorios o extraordinarios. |
| `financier-auditor` | `FinanciadorMSP` | Reserva para verificacion de trazabilidad por financiador; no habilita escrituras ni custodia. |

Reglas de autorizacion:

- La identidad de establecimiento siempre se resuelve con `cid.GetMSPID()` y el
  registro de ADR-003.
- `snt.role` no reemplaza a `agentType`: un usuario con rol `operator` solo puede
  operar dentro de las capacidades de la organizacion a la que pertenece.
- Una organizacion custodial debe existir en el registro y tener `active=true`
  para operar.
- `AnmatMSP` puede auditar y ejecutar operaciones regulatorias, pero no se
  convierte por eso en custodio ordinario.
- `FinanciadorMSP` no puede invocar operaciones de transferencia, dispensacion o
  eventos extraordinarios hasta que DES-10/CC-8 definan su comportamiento.
- DES-6 no define nombres de funciones, requests, responses ni errores. Esas
  decisiones pertenecen a DES-5.

## Politicas de endoso

La politica de endoso es parte central del argumento de integridad del PFI:
multiples organizaciones independientes ejecutan la misma logica deterministica
antes de que una escritura sea valida. La red no debe depender de una aceptacion
unilateral del custodio saliente ni convertir a ANMAT en cuello de botella de
todas las escrituras.

La politica de diseno es:

| Clase de operacion | Autorizacion de negocio | Endoso requerido por diseno |
|---|---|---|
| Alta o cambio del registro organizacion-establecimiento | `AnmatMSP` con `snt.role=regulatory-admin` | `AnmatMSP`. La incorporacion tecnica de la organizacion al canal queda para NET-2/NET-4. |
| Registro inicial de unidades | Organizacion activa con `agentType=LABORATORY` y rol `operator` | Laboratorio invocante. No hay contraparte previa capaz de verificar custodia; la integridad multiorganizacion se ejerce desde las transferencias posteriores. |
| Transferencia ordinaria de custodia | Custodio actual activo, destino activo y par permitido por DES-3 | Origen y destino de la transferencia. Si DES-9 separa despacho y recepcion, la transaccion que confirma el cambio de custodia debe quedar cubierta por ambas partes. |
| Dispensacion | Custodio actual con `agentType=PHARMACY` o `HEALTHCARE_FACILITY` y rol `operator` | Organizacion dispensadora. Validacion por financiador queda fuera de DES-6 y depende de DES-10. |
| Evento extraordinario informado por custodio | Custodio actual activo y evento admitido por ADR-001. Incluye `RETIRAR_MERCADO` desde `EN_LABORATORIO` cuando el laboratorio todavia es custodio actual. | Custodio actual. `AnmatMSP` solo se agrega cuando la operacion sea iniciada por ANMAT o requiera autorizacion regulatoria previa. |
| Retiro, recupero o disposicion final iniciado por laboratorio no custodio | Laboratorio activo con `agentType=LABORATORY` y rol `operator`, vinculado como titular, elaborador o importador de la unidad o lote segun el modelo y contrato vigentes. Cubre las transiciones ADR-001 donde `LABORATORY` actua sobre unidades fuera de su custodia; no habilita `PROHIBIR_PRODUCTO`. | Laboratorio invocante y `AnmatMSP`. El coendoso de ANMAT se exige solo en este caso puntual para validar la legitimidad del reclamo antes de mutar un asset bajo custodia ajena. |
| Evento regulatorio iniciado por ANMAT | `AnmatMSP` con `snt.role=regulatory-admin` | `AnmatMSP` y, cuando la operacion afecte custodia o datos privados de un establecimiento, la organizacion custodia involucrada. |
| Lecturas y auditoria | Segun MSP, rol y visibilidad ADR-002 | No generan endoso de escritura; se gobiernan por politicas de lectura del canal, PDC y contrato DES-5. |

La materializacion concreta queda fuera de DES-6. NET-2, NET-5 y NET-6 deben
decidir como expresarla en configuracion Fabric, por ejemplo mediante politicas
generadas por organizacion, endorsement policies de chaincode, state-based
endorsement o politicas de colecciones privadas. Cualquiera sea el mecanismo,
debe preservar estas propiedades:

- no usar una unica MSP de categoria para representar establecimientos distintos;
- no exigir `AnmatMSP` como coendosante de toda escritura ordinaria;
- exigir `AnmatMSP` como coendosante cuando un laboratorio no custodio inicia un
  retiro, recupero o disposicion final sobre una unidad bajo custodia ajena;
- no permitir que una transferencia ordinaria cambie custodia solo con endoso del
  origen;
- no exponer datos privados a organizaciones no participantes, en linea con
  ADR-002;
- mantener paridad funcional con la baseline cuando se implemente la logica de
  autorizacion equivalente.

## Contexto utilizado

- [ADR-001](adr/001-maquina-estados-medicamento.md): maquina de estados del medicamento, actores logicos y transiciones.
- [ADR-002](adr/002-topologia-canales.md): topologia de canal unico, estado minimo de trazabilidad visible y PDC
  para informacion comercial o documental.
- [ADR-003](adr/003-establishment-identity-gln-cufe.md): identidad de establecimientos mediante GLN/CUFE y organizacion Fabric
  por establecimiento.
- [`domain/authorized-transfers.json`](../domain/authorized-transfers.json): catalogo de `agentType`,
  usado solo para reutilizar categorias de agente.
- Paper del proyecto, secciones 3.3 a 3.5: red Hyperledger Fabric permisionada,
  chaincode como validacion de reglas, privacidad por canales/PDC y excepcion de
  auditoria para ANMAT.
- Avance de tesis, secciones 1.1, 1.2.2, 2.1.3.3 y 2.1.5: alcance del prototipo,
  procesos core SNT, actores regulados, GLN/CUFE, MSP y canales.
- Hyperledger Fabric: documentacion oficial sobre MSP, politicas y endorsement
  policies.
- Hyperledger Fabric CA: documentacion oficial sobre atributos en certificados de
  enrolamiento.
- Paquete Go `github.com/hyperledger/fabric-chaincode-go/pkg/cid`: funciones
  `GetMSPID` y lectura de atributos de identidad.
