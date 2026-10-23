## EVAL-6 / EVAL-7 completos: revisión del corte parcial para EVAL-8

**Terminadas mediciones y procesamiento estricto el 22/10/2026 a las 08:13 Argentina.** Este comentario final complementa el [corte parcial de 13 escenarios](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/issues/48#issuecomment-6333125883), que se conserva como histórico. No cierra #48 ni transforma este aporte en una implementación completa de EVAL-8.

### Aceptación y procedencia

- Campaña `eval67-389ca16-20261021e-full`: **20/20 escenarios por SUT, 264 intentos completed, cero failed; 112 mediciones por SUT y 20 warm-ups por SUT**. Dieciséis escenarios de cinco pares; cuatro de ocho: registro 10/20, lectura de unidad 50 e historial 25.
- Procesador estricto sin `--allow-noncitable` ni `--no-validate`: 224 mediciones incluidas; exclusiones únicamente de 264 smoke y 40 warm-ups. Cero pares faltantes, cero errores finales inesperados, `nonCitable=false`. Son 40 grupos y 24 filas comparativas (20 combinaciones más cuatro extensiones originales/extendidas).
- Commit medido `389ca160b2a490cd66bb0194f012e2b75b3598e8`, etiqueta local `eval67-20261021T0020Z`. Contrato 2.11.2, package ID `snt_1.0:a4d9315a304ac10b467c096f37f4960f86e4ce2d828310aed58d5ad34082d9a9`; baseline del mismo commit con imagen `sha256:c9dc6ed2e16e61bd0e5cca5582f7069b1657c32741ee86d118cc9f71ef30be09`.
- Dataset 50.000, seed 20260727, SHA-256 `de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d`; ambos SUT parten del mismo plan lógico acreditado, con restauración/verificación por intento y ejecución alternada.
- Ryzen 7 7800X3D, WSL2 Ubuntu 24.04, 6 CPU / 8 GiB / 4 GiB swap; Docker 29.7.2 / Compose 5.4.0. **No se demostró igualdad de presupuesto agregado de recursos**; se conservan las cuotas originales y sus registros efectivos. No extrapolar a producción.

### Resultados completos que faltaban en el parcial

Tabla resumida: media entre repeticiones; se usa 1–8 cuando hubo extensión y 1–5 en los demás casos. Se conservan ambas series, los desvíos poblacionales, CV y p50/p99 en `analysis/aggregate.csv`, `repetitions.csv` y `report.md`. Los p95 son percentiles nearest-rank por ronda, después promediados. Throughput conceptual en ops/s; transferencia significa **pares completos/s**. Restauración, verificación y calentamiento no integran las métricas del workload.

| Escenario | Tasa | n por SUT | Throughput F / B | p95 F / B (ms) |
|---|---:|---:|---:|---:|
| mixed | 20 | 5 | 19.46 / 20 | 4336 / 11.2 |
| read-history | 10 | 5 | 10.032 / 10 | 9.2 / 3 |
| read-history | 25 | 8 | 25.031 / 25 | 7.25 / 2.25 |
| read-history | 50 | 5 | 50.029 / 50 | 6.8 / 2 |
| read-unit | 10 | 5 | 10.032 / 10 | 8.6 / 3 |
| read-unit | 25 | 5 | 25.031 / 25 | 5.2 / 2 |
| read-unit | 50 | 8 | 50.029 / 50 | 5.5 / 2 |
| write-dispense | 5 | 5 | 4.991 / 5 | 2079.4 / 6.4 |
| write-dispense | 10 | 5 | 9.941 / 10 | 2100.6 / 6.6 |
| write-dispense | 20 | 5 | 19.846 / 20 | 2141 / 5.8 |
| write-register | 5 | 5 | 4.944 / 5 | 2108.2 / 8 |
| write-register | 10 | 8 | 9.968 / 10 | 2098.125 / 7.125 |
| write-register | 20 | 8 | 19.809 / 20 | 2254.25 / 6.5 |
| write-transfer | 5 | 5 | 4.881 / 5 | 4378.2 / 15.4 |
| write-transfer | 10 | 5 | 9.719 / 10 | 4555.4 / 14 |
| write-transfer | 20 | 5 | 19.456 / 20 | 4767.6 / 12.4 |

Rechazos esperados, separados de throughput exitoso:

| Variante | n | Rechazos/s F / B | p95 F / B (ms) |
|---|---:|---:|---:|
| BLOCKING_STATE/dispense | 5 | 5.033 / 5 | 7 / 3.4 |
| BLOCKING_STATE/transfer | 5 | 5.033 / 5 | 8.6 / 3.8 |
| DUPLICATE_IDENTITY/register | 5 | 5.033 / 5 | 7 / 4 |
| UNAUTHORIZED_TRANSFER/transfer | 5 | 5.033 / 5 | 9.6 / 3.8 |

Códigos acreditados: `TRANSFER_NOT_AUTHORIZED`, `UNIT_ALREADY_EXISTS` e `INVALID_STATE_TRANSITION`. Se contabilizaron 6.040 casos en Fabric y 6.000 en baseline; los bordes de generación no prueban mayor capacidad.

### Costos y límites que EVAL-8 debe conservar

- **Marcadores:**33.042 esperados/observados sin discrepancia, 31.842 en registros y 1.200 en la mezcla; cero en los otros perfiles. Se conservaron auditorías de marcadores. No hubo eventos regulatorios en estos perfiles; no hay un A/B que aísle costo marginal de marcadores.
- **Transferencia:**dos transacciones confirmadas por par. Fabric tuvo 1.863 reintentos transitorios (incluida mezcla), baseline cero. Permanecen en latencia y tasa de intentos; no son fallos finales de operaciones. La mezcla reporta p95 del par de transferencia 4.436 ms frente a 12,2 ms baseline, separado del p95 global de mezcla 4.336 ms frente a 11,2 ms.
- **Variabilidad persistente tras ocho rondas:**p95 de registro 10/20 baseline 16,36% / 15,38%; lectura de unidad 50 Fabric 18,18%; historial 25 Fabric 16,54% y baseline 19,25%. La extensión no garantizó estabilidad; no se eliminaron rondas para reducir esos CV.
- Las tasas fueron ofrecidas y fijas: se observó que ambos sostienen aproximadamente las cargas ensayadas, con latencias menores en baseline. No se midió capacidad máxima ni saturación; no se atribuye causalidad exclusiva a batching, PDC, CPU o red sin un experimento específico.

### Qué cambia respecto del corte parcial

Se incorporan los dos perfiles restantes de historial, mezcla y cuatro variantes de rechazo: siete escenarios adicionales. **Las 32 filas originales/extendidas de los 13 escenarios del parcial coinciden en las diez métricas cotejadas**, sin invalidaciones ni sustituciones. Ahora sí se acreditó la campaña completa y se reprocesó desde copias versionables: los ocho archivos de salida coinciden con el análisis inicial normalizando sólo directorios. SHA-256 de `comparison.csv`: `d233ef6d3d1c820b3ca082a2e50bba274c06fbc1b11641a158d75c59dd4a5dd0`.

### Vinculación con EVAL-4/5 y lectura del trade-off

Mantener la evidencia del [Release de disponibilidad con complemento 22/10](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/releases/tag/evidence-eval4-eval5-citable): API o BD caídas producen 0% de éxito global en baseline; Fabric mantiene 97,1% en el balance de raft-1 y 24,5% sólo lecturas sin quórum. La caída de peer deja 0–2,2% en la clase objetivo frente a 97,3–100% en control. **raft-1 sigue incomplete por una restauración fallida y reconstruida; ocho rondas en el balance no son ocho completed.** La recuperación se mide desde la inyección, no desde la restitución del contenedor.

Disponibilidad y rendimiento proceden de campañas/hosts distintos; no agrupar sus latencias ni calcular un promedio global. Son fallos de contenedor en host único, sin demostrar resiliencia ante pérdida física del host o de una organización completa. La sonda regulatoria de peer-1c está declarada como desviación.

Para EVAL-8, la evidencia cuantifica un costo de latencia y operación a cambio de garantías estructurales compartidas de integridad, endoso, auditabilidad y confidencialidad, apoyadas por las fuentes ya indicadas en #48. No demuestra superioridad absoluta. Fabric puede justificarse cuando actores independientes necesitan verificar/controlar conjuntamente la custodia y reducir confianza en un administrador único, aceptando esos costos y restricciones; la baseline sigue siendo defendible bajo autoridad central confiable y prioridad de latencia/simplicidad. El historial del ledger no equivale a costo nulo de almacenamiento o procesamiento, y deben conservarse los costos funcionales de PDC, retiros parciales por lote y push/pull identificados en la issue.

### Evidencia y reproducción

Ruta local versionada: `benchmarks/results/eval67/eval67-389ca16-20261021e-full/`, con crudos, resúmenes, metadata, índices, campaña, logs, manifiestos, CSV, SVG, hashes y README. `reproduce.py` ejecuta validación estricta desde estas copias y compara sus ocho salidas. Necesita **bundles originales de snapshot fuera de Git**, declarados con sus hashes/rutas en README y campaña; no se versionaron dumps, volúmenes, claves ni planes privados. El paquete no pretende ser autocontenido. Los commits de evidencia son locales: no se hizo push, merge de PR ni cierre de issues.
