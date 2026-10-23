# EVAL-6: evidencia Fabric del artefacto congelado

Este subconjunto contiene únicamente los archivos Fabric de `eval67-389ca16-20261021e-full`, su manifiesto de snapshot y los hashes de esos archivos. Se incorpora localmente en `codex/issue-46-round-markers` y luego se integra en `codex/issue-47-paired-series`, donde se añaden baseline, campaña, análisis y reproducción. No se hizo push.

Commit experimental `389ca160b2a490cd66bb0194f012e2b75b3598e8`, etiqueta local `eval67-20261021T0020Z`; contrato 2.11.2, package ID `snt_1.0:a4d9315a304ac10b467c096f37f4960f86e4ce2d828310aed58d5ad34082d9a9`, dataset `de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d`.

20 escenarios cerrados: 112 mediciones y 20 calentamientos, más sus smoke separados. Dieciséis series de cinco repeticiones y cuatro de ocho. Registro 10/20 y lecturas unidad 50/historial 25 se extendieron para mantener pares con la baseline o por variación propia. La variabilidad persistente se conserva, no se descartan mediciones para reducirla.

33.042 marcadores de participación esperados/observados: 31.842 por registros y 1.200 por la mezcla; cero discrepancias. No se ofrecieron eventos regulatorios y no existe un A/B que aísle costo marginal. 1.863 reintentos transitorios y cero fallos finales inesperados; las cuatro variantes de rechazo tienen los códigos previstos.

El procesador estricto validó la campaña completa y se reprodujo desde copias, con snapshots originales preservados fuera de Git. La comparación y sus limitaciones están en el README y analysis de la rama EVAL-7. Este subconjunto por sí solo no pretende sustituir la comparación pareada ni distribuir los bundles de snapshot. No se incluyen claves, dumps, volúmenes ni planes privados. `SHA256SUMS.fabric` verifica este subconjunto.
