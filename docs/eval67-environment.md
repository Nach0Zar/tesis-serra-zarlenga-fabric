# Entorno de EVAL-6 y EVAL-7 (#54)

## Artefacto y alcance

Las series de rendimiento de #46/#47 alternan en un mismo host físico.
EVAL-4/EVAL-5 corren en otra máquina y constituyen una campaña distinta.
No se infiere equivalencia de recursos entre ambas campañas.

La preparación incorpora develop `cb1bbff9a2d5b6f7f3e458e282313aead68fcd19`,
PR #153 `f4717931fef69fd87a3ce8ff547c06f5c0aa8bfd` y PR #154
`01d075286643bad1898dc4f3ea95672ce7d83401`. Las PRs estaban abiertas al
seleccionar el código; su uso local no implica aprobación ni merge.
La campaña y una etiqueta local `eval67-<fechaUTC>` registran el commit
experimental final, después de terminar los cambios y antes del snapshot.

## Herramientas reproducibles

Node 22.23.3 y npm 11.6.2 se conservan bajo `build/tools/`, ignorado por Git.
Go es 1.23.5; Fabric 2.5.16; Fabric CA 1.5.17. Las dependencias provienen
de `npm ci`, sin actualizar locks. Caliper es 0.7.1 y el binding Fabric
Gateway es el fijado por su lockfile. No ejecutar `npm audit fix`.

Desde la raíz WSL:

```bash
export PATH="$PWD/build/tools/npm/node_modules/.bin:$PWD/build/tools/node-v22.23.3-linux-x64/bin:/usr/local/go/bin:/home/nacho/pfi/fabric-samples/bin:$PATH"
export SNT_FABRIC_BIN_DIR=/home/nacho/pfi/fabric-samples/bin
export SNT_FABRIC_CFG_PATH=/home/nacho/pfi/fabric-samples/config
export FABRIC_CFG_PATH="$SNT_FABRIC_CFG_PATH"
node --version
npm --version
go version
command -v node npm go peer configtxgen
npm ci --prefix benchmarks/caliper
npm ci --prefix benchmarks/baseline
npm ci --prefix benchmarks/analysis
```

En otro host deben resolverse y registrarse las rutas equivalentes. El paquete
instalado incluye tests de `chaincode/internal`: archivos adicionales allí
alteran su package ID. `deployCC` verifica el paquete contra
`network/chaincode-package.lock`. Ante diferencia, detenerse y reproducir
el empaquetado de CI desde fuentes limpias; no regenerar el lock para
resolverla ni alterar contrato/package ID.

## Recursos efectivos

Aplicar `network/wslconfig.example`: 6 CPU, memory=8GB, swap=4GB. Respaldar
el archivo previo, preservar otras opciones y coordinar el reinicio de WSL
si existen procesos ajenos activos. Luego:

```bash
./network/scripts/validate-compose-prerequisites.sh --measurement
docker info --format '{{.NCPU}} {{.MemTotal}}'
./network/scripts/measure-resources.sh
```

Conservar las cuotas originales: peer 0,4 CPU/384 MiB; orderer 0,25 CPU/192 MiB;
CA 0,2 CPU/128 MiB; chaincode 0,2 CPU/192 MiB. Registrar los límites efectivos
de API/PostgreSQL, que no tienen ese mismo esquema de cuotas individuales.
**No hay presupuesto agregado equivalente demostrado.** No usar el helper
privilegiado ni el cgroup padre descartados. Compartir host no demuestra
igualdad de presupuesto agregado: declarar esta limitación al interpretar.

Guardar sólo campos seleccionados de recursos de `docker inspect`; el
documento completo y Compose expandido pueden contener secretos. Registrar
host, Windows/WSL, kernel, CPU, memoria, swap, almacenamiento físico, versiones
Docker/Compose, imágenes/IDs, cuotas y rutas de herramientas.
Exigir 80 GiB físicos libres antes de construir y 20 GiB antes de cada intento.
Es un resguardo operativo; no borrar datos automáticamente. `df` del disco
virtual no demuestra espacio físico disponible en Windows.

## Estado inicial y calificación

Bundle: 50.000 unidades, seed 20260727, SHA-256
`de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d`.
El estado canónico registra 46.920 unidades, reserva 3.080 ausentes para medir
altas, prepara 22.103 y conserva 24.817 de relleno; participan 25.183.
La referencia académica a 50.000 altas iniciales requiere sincronización
posterior; gobiernan el protocolo versionado y las issues de evaluación.
Este documento no modifica la tesis.

Respaldar los diez volúmenes permitidos y su material criptográfico antes del
constructor. Un respaldo diagnóstico no es un snapshot golden. No usar
`docker prune` ni borrar otros proyectos. Capturar logs antes de limpiar.

Completar el [gate](fabric-gate.md) una vez por artefacto compatible:
construcción limpia, semántica completa, dos restauraciones verificadas,
inactividad y ensayo no citable. Cada intento restaura, verifica, calienta y
ejecuta smoke; no reconstruye el snapshot. HEAD exacto es parte de la
compatibilidad incluso cuando sólo cambia documentación.

## Operación y evidencia

Conservar controladores/logs bajo `build/`, en sesión persistente como tmux.
No ejecutar tests, compilaciones ni profiling durante ventanas medidas.
Detener el muestreo diagnóstico de construcción antes del ensayo. Un estado
running sin proceso ni progreso no demuestra actividad. No reiniciar una
fase terminal ni reemplazar mediciones descartadas. La PC permanece encendida.

Credenciales baseline efímeras en archivo ignorado 0600; nunca versionar
claves, PEM, transient, volúmenes ni dumps. Promover crudos finales, metadata,
índices, manifiestos acreditantes, auditorías, descartes, recursos y reportes
después de procesar. Conservar siempre el commit del artefacto medido.

Pruebas Node, schemas y metadata no requieren SUT activo. Construcción,
restauración, PostgreSQL y observación de bloques requieren Docker real.
Las suites locales no sustituyen la calificación Docker.

Una PR apilada con base distinta de main/develop puede no disparar workflows;
el estado visual de GitHub no sustituye validaciones locales registradas.
Las notas antiguas de #46/#54/#137 sobre PR #141 draft y presupuesto agregado
son históricas. #137 está cerrada; cada nuevo commit experimental todavía
requiere su calificación compatible.

