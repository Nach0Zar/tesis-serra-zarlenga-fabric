# EVAL-6 / EVAL-7: campaña completa alternada

Campaña `eval67-389ca16-20261021e-full`, plan `full`. Mediciones del 21/10/2026 15:18:02.829 UTC al 22/10/2026 11:12:59.667 UTC; procesamiento estricto terminado a las 11:13:03 UTC (08:13 Argentina). Duración transcurrida de la campaña: aproximadamente 19 h 55 min. No confundirla con las ventanas de carga: las 112 mediciones acumulan 12.346,586 s observados en Fabric y 12.240,077 s en baseline; restauración, verificación, smoke y warm-ups no integran latencia ni throughput medidos.

## Identidad y entorno

- Commit experimental congelado: `389ca160b2a490cd66bb0194f012e2b75b3598e8`; etiqueta `eval67-20261021T0020Z`. Los commits posteriores de evidencia no son el artefacto medido. La etiqueta y la evidencia están publicadas.
- Fabric 2.5.16, CA 1.5.17, Caliper 0.7.1; contrato 2.11.2. Package ID: `snt_1.0:a4d9315a304ac10b467c096f37f4960f86e4ce2d828310aed58d5ad34082d9a9`.
- Baseline del mismo commit, imagen inmutable `sha256:c9dc6ed2e16e61bd0e5cca5582f7069b1657c32741ee86d118cc9f71ef30be09`.
- Dataset 50.000 unidades, seed 20260727, SHA-256 `de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d`. Conteos canónicos: 50.000 / 46.920 / 3.080 / 22.103 / 24.817 / 25.183. Plan lógico normalizado compartido: `de887e31483b8bb3d1a214172054e6f457f1179bcb77284b1a3dc567abeda820`.
- Host Ryzen 7 7800X3D, Ubuntu 24.04.4 LTS en WSL2, 6 CPU visibles, memoria efectiva 8.327.917.568 bytes, swap 4.294.967.296 bytes. Docker 29.7.2 / Compose 5.4.0. `environment-status.json` refleja el arranque anterior (8.327.913.472 bytes); `qualification/resources.log` documenta el efectivo tras la recuperación, 4.096 bytes mayor.
- Sin simultaneidad entre SUT. Se conservaron las cuotas originales de peers, orderers, CA, chaincodes y baseline: **no hay presupuesto agregado equivalente demostrado**. Véase también `docs/eval67-environment.md`.
- Node 22.23.3, npm 11.6.2 y Go 1.23.5 en la preparación local. La imagen baseline necesitó confiar, sólo durante build, en la raíz pública de inspección HTTPS ya confiada por Windows; TLS siguió habilitado y el Dockerfile del repositorio no cambió. Se conserva `provenance/tls-build-provenance.json` y el Dockerfile operativo, sin material privado.

## Resultado y aceptación

20 escenarios por SUT, 264 intentos completed, cero failed: 40 warm-ups y 224 mediciones (112 por SUT). Dieciséis escenarios tienen cinco pares y cuatro tienen ocho: registro 10/20, lectura de unidad 50 e historial 25. La extensión preserva ambas series originales 1–5 y extendidas 1–8. En cinco combinaciones SUT/escenario persiste CV p95 >15% al llegar a ocho: se informa, no se eliminan rondas ni se fuerza estabilidad.

Procesamiento estricto: 528 rondas descubiertas, 224 incluidas, 264 smoke y 40 warm-ups excluidos. No hay exclusiones de mediciones, pares faltantes ni errores inesperados; `nonCitable=false`. Se producen 40 grupos, 48 filas agregadas (originales y extendidas), 24 comparaciones (20 combinaciones más cuatro extensiones) y dos figuras SVG.

- `analysis/report.md`: tablas, percentiles, desvíos, CV, asimetrías y exclusiones.
- `analysis/repetitions.csv`, `aggregate.csv`, `comparison.csv`: valores completos por repetición y agrupación.
- `analysis/figures/{throughput,latency}.svg`: figuras exportables. Los rechazos se informan por separado, no como throughput de éxitos.
- Fabric: 33.042 marcadores esperados/observados sin discrepancia; 31.842 en registros y 1.200 en mixto. Ningún evento regulatorio fue ofrecido por estos perfiles; cero en transferencias, dispensaciones, lecturas y rechazos. Su costo marginal no queda aislado por un A/B.
- Fabric: 1.863 intentos transitorios de recepción reintentados, incluidos en el costo de transferencia y mixto; baseline: cero. Cero fallos finales inesperados no significa cero intentos transitorios fallidos.
- Cuatro variantes de rechazo: 6.040 casos esperados Fabric y 6.000 baseline, con códigos correctos. Las diferencias de borde del generador no se interpretan como mayor capacidad.
- Esta matriz usa tasas ofrecidas fijas: no demuestra throughput máximo, saturación, superioridad global ni comportamiento de producción.

Se conservaron b/c/d fallidos o interrumpidos fuera de este paquete, sin reanudarlos ni sustituir mediciones. El trial-b de 18 intentos se reutilizó sólo como validación no citable del mismo artefacto/snapshots; `qualification/prior-trial-proof.json` conserva la procedencia. Gate-e revalidó dos restores por SUT y 15 min de inactividad antes de medir.

## Archivo publicado y contenido del repositorio

La evidencia completa está en el [Release EVAL-6/EVAL-7](https://github.com/Nach0Zar/tesis-serra-zarlenga-fabric/releases/tag/evidence-eval6-eval7-citable), asset `eval67-389ca16-20261021e-full-evidence.tar.gz` (36152312 bytes comprimidos). SHA-256: `10f536db0205ac8547abcfc43235685c30a33782deef6267c7a7b0baaa6d1dfc`. `SHA256SUMS-release` en este directorio fija ese digest, y el release publica otra copia para consulta.

El archivo conserva **los 3.471 archivos originales, sin cambiar bytes**, del commit de evidencia `18d0d950817e38451953e056b418d49751b384ce`. Al extraer se obtiene `eval67-389ca16-20261021e-full/`, con `fabric/`, `baseline/`, `campaign.json`, `snapshots/` (sólo manifiestos), `qualification/`, `provenance/` y las salidas originales de `analysis/`. Se mantienen warm-ups, smoke, índices, auditorías de marcadores, recursos, logs y HTML identificables. Los crudos conceptuales y CSV son autoritativos frente al HTML. El README/helper internos del archivo son las copias históricas exactas; las instrucciones actuales de descarga son las de este documento.

En el árbol actual del repo permanecen este README, README-FABRIC, reproduce.py, los manifiestos SHA256SUMS/SHA256SUMS.fabric/SHA256SUMS-release y los ocho archivos de analysis/ (CSV, reporte y SVG). **SHA256SUMS y SHA256SUMS.fabric describen el paquete original extraído, no este directorio reducido**: incluyen el README y helper históricos. Se conservan sin cambios para verificar el archivo completo; los documentos y el helper actualizados se revisan en Git.

No se incluyen claves, credenciales, network-config, work-plan con privateData, volúmenes del ledger, dumps ni planes lógicos completos. Los scripts de procedencia sólo refieren archivos ignorados originales del host; no son un instalador autocontenido. `provenance/evidence-review.json`, dentro del archivo, documenta la comparación con fuentes originales.

#156 y #157 ya estaban mergeadas cuando se pidió la migración. Quitar los crudos del árbol actual **no los borra del historial Git ni reduce un clon completo**. No se hizo force-push, reescritura de historia ni cambio del commit/etiqueta medidos. La fuente original sigue disponible para auditoría.

## Reproducir sin volver a medir

Requiere el código de procesamiento integrado desde EVAL-7 (#157), Node 22, Go del proyecto y dependencias fijadas de `benchmarks/analysis`. Las copias se procesan con validación de metadata, crudos, marcadores, campaña y snapshots habilitada: no se usa `--no-validate` ni `--allow-noncitable`.

**Dependencia externa intencional:** la acreditación de snapshots requiere los bundles originales preservados fuera de Git. El manifiesto no basta. Fabric necesita los archivos acreditantes del plan/estado; baseline también su dump. No se publican para aparentar portabilidad completa.

Desde la raíz WSL del repositorio, con GitHub CLI autenticado y Node/Go en PATH, elegir un directorio de descarga nuevo:

```sh
set -e
CAMPAIGN_INDEX="$PWD/benchmarks/results/eval67/eval67-389ca16-20261021e-full"
RELEASE_DIR=build/benchmarks/paired/releases/evidence-eval6-eval7-citable
mkdir -p "$RELEASE_DIR"
gh release download evidence-eval6-eval7-citable \
  --repo Nach0Zar/tesis-serra-zarlenga-fabric \
  --pattern eval67-389ca16-20261021e-full-evidence.tar.gz \
  --dir "$RELEASE_DIR"
(
  cd "$RELEASE_DIR"
  sha256sum -c "$CAMPAIGN_INDEX/SHA256SUMS-release"
  tar -xzf eval67-389ca16-20261021e-full-evidence.tar.gz
)
(
  cd "$RELEASE_DIR/eval67-389ca16-20261021e-full"
  sha256sum -c SHA256SUMS
)
python3 "$CAMPAIGN_INDEX/reproduce.py" \
  --evidence-dir "$RELEASE_DIR/eval67-389ca16-20261021e-full" \
  --fabric-snapshot build/benchmarks/fabric/snapshots/178e51e9ab43094d3cefa076828d032d6c55b576f35eb2a2b2d54fe9889d6fde \
  --baseline-snapshot build/benchmarks/baseline/snapshots/eval67-389ca16-20261021b \
  --output build/analysis/eval67-release-reproduced
```

El helper exige `--evidence-dir`, comprueba que el manifiesto descargado coincida con el fijado en Git, valida sus 3.470 hashes y los ocho resultados retenidos, y ejecuta el procesador estricto sobre esos crudos. Compara las ocho salidas contra `analysis/`, normalizando sólo los prefijos de directorio. La salida debe ser un directorio nuevo fuera de la evidencia y los bundles; no sobrescribe fuentes. No descarga ni modifica snapshots ni crea nuevas mediciones.

El hash de `comparison.csv` sigue siendo `d233ef6d3d1c820b3ca082a2e50bba274c06fbc1b11641a158d75c59dd4a5dd0`. La descarga se comprueba contra el checksum versionado, no confiando sólo en un checksum obtenido del mismo servidor.

Los manifiestos originales requeridos tienen SHA-256 Fabric `975f9ac77e59309d4ac6b66ddd14d1563253a58476a9cac1f3dd9d23c6eb9483` y baseline `40269fcd2a9526e8a6859efa776570eb691d2e4536add4d34ebf38c425313eb5`. Se preservan sus IDs y rutas en `campaign.json`.

La evidencia de disponibilidad EVAL-4/5 pertenece a otro host/campaña y no se agrega a estas estadísticas. El comentario parcial de EVAL-8 (#48) se conserva como histórico y se contrasta con estos resultados completos.
