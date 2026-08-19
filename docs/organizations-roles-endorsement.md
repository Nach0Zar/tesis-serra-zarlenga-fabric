# DES-6: Organizaciones, MSP, roles, ABAC y politicas de endoso

- **Estado**: Aceptado
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
4. `FinanciadorMSP` es una organizacion no custodial. Su comportamiento quedo
   definido por ADR-005: verificador de trazabilidad de solo lectura, posterior
   a la dispensa, sin escrituras ni custodia; no participa en la matriz de
   transferencias ordinarias.
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
| `FinanciadorMSP` | Organismo financiador | Lectura/verificacion de trazabilidad conforme ADR-005; sin transferencia ni custodia. |

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
- `FinanciadorMSP` no puede invocar operaciones de transferencia, dispensacion
  ni eventos extraordinarios; conforme ADR-005 opera exclusivamente con
  operaciones de lectura sobre el estado publico y el historial (implementacion
  en CC-8).
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
| Registro inicial de unidades | Organizacion activa con `agentType=LABORATORY` y rol `operator` | Laboratorio invocante. No hay contraparte previa capaz de verificar custodia; la integridad multiorganizacion se ejerce desde las transferencias posteriores. **Materializacion** (ADR-007, punto 6.g): la clave de la unidad no existe todavia, con lo que su primera escritura no puede quedar cubierta por endoso basado en estado. `RegisterUnit` escribe ademas un **marcador de participacion** en la coleccion implicita del laboratorio invocante, cuya politica de endoso pertenece a esa organizacion y rige desde el despliegue: con eso el endoso del peer del laboratorio queda exigido por la plataforma ya en la primera transaccion. Precision: la politica de chaincode sigue admitiendo endosantes adicionales, de modo que el laboratorio es **necesario**, no exclusivo; endosantes de mas solo agregan ejecuciones coincidentes de la misma logica determinista. |
| Transferencia ordinaria de custodia | Custodio actual activo, destino activo y par permitido por DES-3 | Origen y destino de la transferencia. Si DES-9 separa despacho y recepcion, la transaccion que confirma el cambio de custodia debe quedar cubierta por ambas partes. **Materializacion** (ADR-007, punto 6.b): el despacho fija sobre la clave de la unidad la politica `AND(emisor, receptor declarado)`, **sin rama alternativa**. Ninguna otra organizacion —tampoco `AnmatMSP`— puede sustituir a una de las dos partes en la recepcion, el rechazo o cualquier otra escritura durante el transito. |
| Dispensacion | Custodio actual con `agentType=PHARMACY` o `HEALTHCARE_FACILITY` y rol `operator` | Organizacion dispensadora. Validacion por financiador queda fuera de DES-6 y depende de DES-10. **Materializacion** (ADR-007, punto 6.a): la politica de reposo de la clave de la unidad es la organizacion del custodio actual, sin rama alternativa, con lo que la dispensacion solo puede endosarla el peer del dispensador. |
| Evento extraordinario informado por custodio | Custodio actual activo y evento admitido por ADR-001. Incluye `RETIRAR_MERCADO` desde `EN_LABORATORIO` cuando el laboratorio todavia es custodio actual. | Custodio actual. `AnmatMSP` solo se agrega cuando la operacion sea iniciada por ANMAT o requiera autorizacion regulatoria previa. **Salvedad durante `EN_TRANSITO`** (ADR-007, punto 6.b): mientras dura el transito, la clave de la unidad lleva la politica de endoso basado en estado `AND(emisor, receptor declarado)`, de modo que un evento extraordinario informado en esa ventana requiere el coendoso del receptor declarado. El endurecimiento es deliberado y acotado a la ventana de transito, donde ambas partes son interesados directos de la operacion pendiente; NET-6 debe validarlo empiricamente. |
| Retiro, recupero o disposicion final iniciado por laboratorio no custodio | Laboratorio activo con `agentType=LABORATORY` y rol `operator`, vinculado como titular, elaborador o importador de la unidad o lote segun el modelo y contrato vigentes. Cubre las transiciones ADR-001 donde `LABORATORY` actua sobre unidades fuera de su custodia; no habilita `PROHIBIR_PRODUCTO`. | Laboratorio invocante y `AnmatMSP`. El coendoso de ANMAT se exige solo en este caso puntual para validar la legitimidad del reclamo antes de mutar un asset bajo custodia ajena. **Materializacion** (ADR-007, punto 6.e): este par no puede imponerse con endoso basado en estado sobre la clave de la unidad, porque la politica de una clave se evalua contra el estado previo y no puede condicionarse a la operacion intentada. Se materializa en dos pasos: la autoridad regulatoria emite `AuthorizeLabIntervention` sobre la unidad, y la operacion posterior del laboratorio escribe **marcadores de participacion** en su propia coleccion implicita y en la regulatoria, con lo que la plataforma exige el endoso de ambos peers en esa misma transaccion. **Salvedad**: como esa transaccion escribe ademas la clave de la unidad, sujeta a la politica de reposo, el endoso efectivo es de tres organizaciones — laboratorio designado, organizacion regulatoria y custodio actual. Es un endurecimiento respecto de la letra de esta fila, coherente con su propia justificacion de no mutar un asset bajo custodia ajena sin validacion; NET-6 debe validarlo empiricamente. |
| Evento regulatorio iniciado por ANMAT | `AnmatMSP` con `snt.role=regulatory-admin` | `AnmatMSP` y, cuando la operacion afecte custodia o datos privados de un establecimiento, la organizacion custodia involucrada. **Materializacion** (ADR-007, punto 6.d): la politica de la clave de la unidad **no** incluye a `AnmatMSP` como rama alternativa —incluirla habilitaria a ANMAT a endosar tambien las operaciones ordinarias, porque la politica es de la clave y no de la funcion—. El coendoso de esta fila se obtiene por otro camino: la transaccion escribe un **marcador de participacion** en la coleccion implicita de la organizacion regulatoria, cuya politica de endoso le pertenece, de modo que la plataforma exige su endoso; y la politica de reposo exige el del custodio actual, o el de ambas partes durante el transito. Es un coendoso real de peers y no la mera firma de creador del regulador, que acredita identidad e iniciativa pero no prueba ejecucion. Consecuencia asumida: si el peer del custodio o el regulatorio no estan disponibles, el evento no puede confirmarse. La politica de endoso de la coleccion privada del par (`OR(org A, org B)`, ADR-006) opera como barrera adicional e independiente sobre las escrituras privadas. |
| Lecturas y auditoria | Segun MSP, rol y visibilidad ADR-002 | No generan endoso de escritura; se gobiernan por politicas de lectura del canal, PDC y contrato DES-5. |

La materializacion concreta queda fuera de DES-6. NET-2, NET-5 y NET-6 deben
decidir como expresarla en configuracion Fabric, por ejemplo mediante politicas
generadas por organizacion, endorsement policies de chaincode, state-based
endorsement o politicas de colecciones privadas. ADR-007 (punto 6) fija esa
materializacion y deja registrado un limite de plataforma que DES-6 no habia
considerado: una politica de endoso basada en estado se evalua contra el estado
**anterior** a la transaccion, por lo que solo puede expresar requisitos
derivables del estado ya confirmado y nunca condicionarse a la operacion que la
transaccion intenta. Las filas de esta tabla cuyo conjunto de endosantes depende
del invocador se materializan por los mecanismos complementarios indicados en
cada fila, y las condiciones que dependen exclusivamente del invocador se
validan en la logica del chaincode. Cualquiera sea el mecanismo, debe preservar
estas propiedades:

- no usar una unica MSP de categoria para representar establecimientos distintos;
- no exigir `AnmatMSP` como coendosante de toda escritura ordinaria;
- exigir `AnmatMSP` como coendosante cuando un laboratorio no custodio inicia un
  retiro, recupero o disposicion final sobre una unidad bajo custodia ajena;
- no permitir que una transferencia ordinaria cambie custodia solo con endoso del
  origen;
- durante `EN_TRANSITO`, admitir la salvedad de coendoso que fija ADR-007 para los
  eventos extraordinarios informados por el custodio;
- restablecer la politica de endoso de la clave de la unidad en **toda** salida de
  `EN_TRANSITO` (recepcion, rechazo y evento extraordinario que cierre el
  transito), para que la unidad no quede bloqueada bajo una politica que exige a
  la contraparte de un despacho ya resuelto;
- distinguir, en la evidencia que se presente, las reglas impuestas por la
  plataforma (rechazo por politica de endoso) de las validadas por la logica del
  chaincode (error tipificado del contrato), sin atribuir a la primera garantias
  que solo provee la segunda;
- no admitir a `AnmatMSP` —ni a ninguna otra organizacion ajena a la operacion—
  como rama alternativa de la politica de endoso de la clave de una unidad: una
  rama disyuntiva agregada para un caso excepcional habilita con la misma fuerza
  todos los casos ordinarios, porque la politica es de la clave y no de la
  funcion invocada;
- exigir el coendoso de una organizacion mediante un mecanismo que la plataforma
  imponga —endoso basado en estado o politica de coleccion—, nunca apoyandolo en
  la firma de creador de la transaccion: esa firma acredita quien invoca, no que
  un peer de esa organizacion haya ejecutado la logica;
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
- [ADR-007](adr/007-network-topology.md): materializacion de estas politicas mediante
  endoso basado en estado, salvedad de coendoso durante `EN_TRANSITO`, limite de
  SBE respecto de la operacion intentada y de la primera escritura de una clave, y
  marcador de participacion en la coleccion implicita de una organizacion como
  forma nativa de exigir su coendoso.
- [ADR-010](adr/010-non-custodial-identity.md): la identidad de `AnmatMSP` y de los
  financiadores se resuelve por el registro (`agentType` `REGULATOR`/`FINANCIER`), no
  por el nombre de la MSP.
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
