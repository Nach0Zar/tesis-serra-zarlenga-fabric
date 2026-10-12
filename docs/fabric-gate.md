# Gate Docker de preparación Fabric (#137 / PR #141)

## Propósito y límites

El gate valida que el snapshot golden puede construirse, verificarse y
restaurarse, y que el orquestador ejecuta un ensayo no citable sin contaminar
el estado inicial. No es la serie citable de #46 ni el procesamiento de #47.
La baseline de #43 consume el mismo plan lógico; no necesita copiar PDC,
endoso ni marcadores de Fabric (ADR-012).

El contrato experimental sigue en [measurement-protocol.md](measurement-protocol.md).
Los comandos e interfaces están en el [README de Caliper](../benchmarks/caliper/README.md).
Este documento describe la operación y la evidencia del gate, no redefine
estados, contratos públicos, políticas, topología ni parámetros de consenso.

## Qué se ejecuta una vez y qué se repite

| Nivel | Cuándo | Validación |
|---|---|---|
| Calificación del artefacto | Al construir un snapshot nuevo o validar una corrección que afecta al gate | Construcción limpia, verificación completa, dos restauraciones verificadas y `trial` completo. |
| Cada intento de la serie | Antes del warm-up y de cada repetición, incluso los reintentos de warm-up | Restauración propia, integridad, verificación, calentamiento de siete peers, recursos, smoke de lectura y ronda. |
| Serie citable | Según #46 y el protocolo, después del gate | Matriz `full`, repeticiones y extensión por CV; no volver a construir el estado para cada ronda. |

**No se reconstruye el snapshot ni se repite el ensayo trial antes de cada
ronda.** Se reutiliza un artefacto compatible, restaurándolo siempre. El
orquestador actual realiza una verificación completa de las 50.000 unidades
después de cada restauración; además, el runner valida las unidades que usará
la ronda antes de Caliper. No se omite ninguno de estos controles.

La compatibilidad actual es estricta: `assertSnapshotCompatibility` exige el
**mismo HEAD**, versión contractual, package ID, hash de fuentes y configuración
de red. `readSnapshot` valida esquema, archivos y hashes de los volúmenes. Un
nuevo commit, incluso sólo documental, cambia HEAD y requiere un snapshot
ligado al nuevo commit. No se edita el manifiesto para volver compatible un
snapshot viejo. Si cambia el entorno experimental, se registra y se vuelve a
calificar en ese entorno antes de usarlo como evidencia comparativa.

## Prerrequisitos

- Node 22, npm 11 y dependencias del lockfile (`npm ci`).
- CLI y runtime Fabric previstos por el repositorio; material criptográfico,
  canal y lifecycle verificados por `network/network.sh`.
- Bundle canónico de 50.000 unidades, seed y hash del protocolo.
- Espacio para las diez exportaciones, ledger restaurado, logs y resultados.
- Fabric y baseline no corren simultáneamente; tampoco otro constructor o serie.
- Configuración efectiva del SUT documentada y contrastada con Docker.
- Archivos de medición versionados y sin modificaciones staged/unstaged:
  `validate-compose-prerequisites.sh --measurement` lo exige. No se relaja
  este preflight para medir una configuración local sin commit.

La última decisión local revirtió la ampliación agregada de recursos:
peers 0,4 CPU / 384 MiB, orderers 0,25 CPU / 192 MiB, CA 0,2 CPU / 128 MiB,
chaincodes 0,2 CPU / 192 MiB, sin cgroup padre experimental. Es el perfil
original del prototipo, **no** una demostración de equivalencia con un
presupuesto agregado común para ambos SUT. La comparación citable debe
registrar sus recursos efectivos conforme al protocolo y #54.

## Secuencia del gate

1. Registrar commit, versión contractual, package ID, hashes del dataset,
   plan lógico, fuentes y configuración; guardar evidencia diagnóstica previa
   antes de reemplazar el ledger.
2. Para una corrección de transporte, validar carga sostenida y 15 minutos de
   inactividad: lecturas de los siete peers antes/después, sin nuevos
   `too_many_pings`, OOM ni reinicios/salidas inesperadas. Pasar sólo reposo
   no prueba el camino Gateway hacia orderers bajo carga.
3. Construir desde cero con concurrencia 64 y cinco intentos por paso; no usar
   journals de otra configuración como snapshot canónico. La recuperación de
   preparación reconcilia ledger e historial y bloquea resultados ambiguos.
4. Comprobar la verificación semántica completa, historial, escrituras privadas
   y marcadores; validar manifiesto, snapshot ID y diez archivos de volumen.
5. Restaurar y verificar completamente dos veces desde el mismo artefacto.
   Cada restauración detiene la red, usa sólo la allowlist de diez volúmenes,
   comprueba SHA-256 antes de importar y digest ordenado del árbol restaurado;
   luego confirma red, canal, lifecycle y versiones.
6. Detener profiling y muestreo diagnóstico adicional antes de medir. Ejecutar
   `series --plan trial`: lectura a 50 TPS, transferencia a 20 pares/s y mezcla
   a 20 TPS, con warm-up y dos mediciones por escenario. Cada intento vuelve a
   restaurar; no comparte el ledger mutado por el intento anterior.
7. Exigir `series-index.json` válido, `status: completed`, `citable: false`,
   todos los escenarios completos y crudos/metadata disponibles. Revisar
   tiempos, throughput, p95, CV, reintentos y descartes, no sólo el exit code.

Un warm-up fallido se repite exactamente una vez sobre una nueva restauración;
dos fallos bloquean las mediciones de ese escenario. Las mediciones inválidas
se conservan y no se reemplazan silenciosamente. El gate falla si no se
satisfacen todas sus fases; pasar la construcción no equivale a pasar el gate.

Desde `benchmarks/caliper`, con `<snapshot>` como ruta del artefacto:

```bash
npm run check
npm test
npm run test:metadata
npm run snapshot:build -- --concurrency 64 --max-attempts 5

npm run snapshot:verify -- --snapshot-dir <snapshot> --concurrency 64
npm run snapshot:restore -- --snapshot-dir <snapshot>
npm run snapshot:verify -- --snapshot-dir <snapshot> --concurrency 64
npm run snapshot:restore -- --snapshot-dir <snapshot>
npm run snapshot:verify -- --snapshot-dir <snapshot> --concurrency 64

npm run series -- --plan trial --snapshot-dir <snapshot> --series-token <token-nuevo>
```

Antes del trial, desde la raíz:

```bash
./network/scripts/validate-compose-prerequisites.sh --measurement
git diff --check
```

`series` ya incorpora calentamiento, smoke e instrumentación normal de cada
intento; no se agregan consultas diagnósticas concurrentes a la ventana medida.
Las validaciones Go/ShellCheck/Compose de CI pertinentes se ejecutan antes del
gate, no mientras un lote diagnóstico o una ronda medida consume el host.

## Ejecución persistente y evidencia

Los controladores operativos de esta validación local se guardan en un
directorio ignorado `build/benchmarks/fabric/gate-<token>/`, no en `/tmp`.
Pueden correr en tmux para sobrevivir al cierre del terminal; no sobreviven
automáticamente al apagado de WSL. Un estado `running` sin proceso ni cambios
de logs no prueba progreso. Sólo se reanuda una fase explícitamente reanudable
y compatible, después de revisar journal y ledger; no se reinicia un fallo
terminal automáticamente.

La evidencia mínima incluye estado, fases y tiempos, logs por fase, journals,
recursos cada 15 segundos durante construcción, configuración efectiva,
manifiesto, hashes de volúmenes, informes semánticos de las restauraciones y
el índice/crudos del trial. El monitor de 15 minutos observa esta evidencia;
no es un GitHub check ni sustituye una fase del gate. Notifica fallos,
finalización o decisiones necesarias, no cada sondeo normal.

Ante fallo se capturan logs de los siete peers, tres orderers y chaincodes
permitidos **antes** de cualquier limpieza. Se conserva el estado terminal,
fase, motivo y journal. No se ejecuta `docker prune`, no se borran recursos
globales y **la PC permanece encendida**, tanto ante fallo como ante éxito.
Los artefactos generados, transient sintético y credenciales permanecen
ignorados; el PR publica resúmenes y hashes, no propuestas, secretos ni PEM.

## Registro para revisión en PR #141

El próximo commit debe incluir este procedimiento y los fixes locales. El PR
debe conservar la historia de fallos y distinguir qué resultados pertenecen
al checkout local y cuáles al commit publicado:

- commit/fingerprint exacto, perfil efectivo y cambios respecto del remoto;
- snapshot ID y SHA-256 del manifiesto, rutas ignoradas y espacio utilizado;
- conteos de unidades, escrituras, historial, datos privados y marcadores;
- tiempos de construcción, verificación y ambas restauraciones;
- resultado del trial, CV y descartes reales, con rutas del índice y crudos;
- comandos/tests ejecutados y requisitos todavía pendientes;
- límites del diagnóstico: una corrida exitosa no demuestra por sí sola la
  causa física de las pausas anteriores de Lab.

PR #141 permanece draft hacia `develop` hasta que todas las fases pasen.
No se mergea ni se cierra #137 como parte de este gate.

### Resultado local de construcción — 2026-10-12

Validación sobre HEAD `03b0953090a93a8adaf59f9787eb7bc8a2629dbd` **más fixes
locales aún sin commit**; no equivale a validar el HEAD remoto sin esos cambios.
Las altas se normalizan por transacción, se comprueba discovery antes de
preparar y se conserva recuperación estricta. No se amplían los recursos.

- 46.920 recetas completas y 129.878 escrituras confirmadas, sin recuperaciones.
- Verificación: 50.000 unidades lógicas; 46.920 presentes y 3.080 ausentes;
  129.878 entradas de historial; 124.419 escrituras privadas de pares;
  46.924 marcadores esperados y observados.
- Constructor: 6.122,972 s; preparación y semántica: 6.058,326 s;
  controlador: 6.190,5 s. Diez archives: 8.214.251.520 bytes (~7,65 GiB).
- Auditoría independiente: `terminal-audit.json`, `status: passed`;
  408 muestras, 18 contenedores estables, sin nuevos pings, OOM ni reinicios.
- Snapshot ID:
  `8d47a62fdd81e285ca5fb954d98be1b03c643035e1d4a9e5b6f6b35953567b44`.
- SHA-256 del manifiesto:
  `a2ef131819f1d4923ae5a65c0153e5c4d63e3dc3a7a23373170e30133fb020e1`.

Evidencia ignorada: `build/benchmarks/fabric/gate-issue137-normalized-original-local/`
(`local-run.json`, `local-changes.patch`, `terminal-audit.json`, recursos y logs)
y `build/benchmarks/fabric/snapshots/<snapshot-id>/`.

Las dos restauraciones locales también pasaron, con SHA-256 y digests de los
diez volúmenes y verificación semántica completa después de cada una:

- Restauración 1: 171,009 s dentro de una fase de 171,988 s;
  verificación posterior: 170,570 s.
- Restauración 2: 168,201 s dentro de una fase de 169,229 s;
  verificación posterior: 170,709 s.
- Ambas verificaciones: 46.920 presentes, 3.080 ausentes, 129.878 entradas de
  historial, 124.419 escrituras privadas de pares y 46.924 marcadores.
- Evidencia: `remaining-results.json`, `remaining-phases.jsonl` y logs
  `restore-1/2.log`, `verify-1/2.log` en el directorio local del gate.

**El gate completo todavía no pasó.** El controlador local terminó
`blocked: trial-preflight`, con `restorationsVerified: 2` y
`trialExecuted: false`: el trial requiere versionar la configuración para
superar el preflight de medición. No fue un fallo de restauración. El usuario
autorizó publicar los fixes y este procedimiento; ese commit cambia HEAD y
exige una construcción nueva
antes del trial. No se promueve el artefacto local reescribiendo sus hashes o
commit, y no se atribuye toda la mejora a una única causa sin evidencia causal.
