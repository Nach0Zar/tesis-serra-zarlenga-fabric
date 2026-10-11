'use strict';

const {SCENARIO_LABELS} = require('./charts');

// Asimetrias estructurales conocidas (#43, #47, ADR-012 seccion 5): se declaran
// junto a los resultados y no se "corrigen".
const ASYMMETRIES = Object.freeze([
    ['Identidad', 'La baseline emula organizaciones con API keys; Fabric usa MSP y PKI.', 'Dentro de lo medido: es parte del costo comparado.'],
    ['Confirmación', 'La baseline confirma en una transacción PostgreSQL; Fabric endosa, ordena con Raft y valida en siete peers.', 'Dentro de lo medido: es la diferencia que se evalúa.'],
    ['Endoso, PDC y marcadores', 'Solo Fabric paga políticas de endoso, colecciones privadas y marcadores de participación (sección 3.5).', 'Dentro de lo medido.'],
    ['Visibilidad entre peers', 'Solo Fabric reintenta la recepción por datos privados no diseminados o estado público atrasado (sección 3.4).', 'Dentro de lo medido: queda en la latencia del par y en los reintentos.'],
    ['Reloj', 'Los timestamps de la baseline vienen del servidor de la API; los de Fabric, de la propuesta.', 'No altera las métricas de cliente.'],
    ['Historial de intervención', 'La baseline paga un INSERT en `lab_intervention_events` por cada escritura de autorización.', 'Fuera de lo medido: esas operaciones no aparecen en las secciones 6.2 ni 6.3.'],
    ['Inmutabilidad', 'El historial de la baseline es append-only por convención; un administrador de base puede alterarlo.', 'Fuera de lo medido: es una propiedad cualitativa.'],
]);

function format(value, digits = 2) {
    if (value === undefined || value === null || !Number.isFinite(value)) return 'n/d';
    return value.toLocaleString('es-AR', {maximumFractionDigits: digits, minimumFractionDigits: 0});
}

function percent(value) {
    return Number.isFinite(value) ? `${format(value * 100, 2)} %` : 'n/d';
}

function scenarioName(entry) {
    const base = SCENARIO_LABELS[entry.scenario] ?? entry.scenario;
    return entry.variant ? `${base} ${entry.variant}` : base;
}

function table(headers, rows) {
    if (rows.length === 0) return '_Sin filas._\n';
    return [
        `| ${headers.join(' | ')} |`,
        `|${headers.map(() => '---').join('|')}|`,
        ...rows.map((row) => `| ${row.join(' | ')} |`),
    ].join('\n') + '\n';
}

function artifactsSection(aggregates) {
    const artifacts = new Map();
    for (const entry of aggregates) artifacts.set(JSON.stringify(entry.artifact), entry.artifact);
    const datasets = [...new Set(aggregates.map((entry) => entry.datasetSHA256))];
    const lines = [...artifacts.values()].map((artifact) => (artifact.sut === 'fabric'
        ? `- **Fabric**: contrato \`${artifact.contractVersion}\`, package ID \`${artifact.packageID}\`, Fabric ${artifact.fabric}.`
        : `- **Baseline**: commit \`${artifact.baselineCommit}\`, imagen \`${artifact.baselineImage}\`.`));
    lines.push(...datasets.map((sha) => `- **Dataset**: \`${sha}\`.`));
    return lines.join('\n') + '\n';
}

function buildReport({aggregates, comparisons, unpaired, excluded, unexpected, availability, inputs}) {
    const stable = aggregates.filter((entry) => entry.scenario !== 'expected-rejections');
    const rejections = aggregates.filter((entry) => entry.scenario === 'expected-rejections');
    const sections = [
        '# Resultados comparativos Fabric y baseline\n',
        `Procesado de ${inputs.map((input) => `\`${input}\``).join(', ')}. Cada fila agrega repeticiones medidas válidas de un mismo SUT, escenario, variante y tasa: no hay promedios entre workloads distintos. Desvío y coeficiente de variación son poblacionales; los percentiles son nearest-rank por ronda y se promedian entre repeticiones.\n`,
        '## Artefactos medidos\n',
        artifactsSection(aggregates),
        '## Rendimiento estable por SUT\n',
        table(
            ['SUT', 'Escenario', 'Tasa', 'n', 'Throughput (op/s)', 'CV thr.', 'p50 (ms)', 'p95 (ms)', 'CV p95', 'p99 (ms)', 'tx/s', 'Extendida'],
            stable.map((entry) => [
                entry.sut, scenarioName(entry), format(entry.targetOperationsPerSecond), entry.throughput.n,
                `${format(entry.throughput.mean)} ± ${format(entry.throughput.standardDeviation)}`,
                percent(entry.throughput.coefficientOfVariation), format(entry.p50Ms.mean, 1),
                format(entry.p95Ms.mean, 1), percent(entry.p95Ms.coefficientOfVariation), format(entry.p99Ms.mean, 1),
                format(entry.transactionThroughput.mean),
                entry.extended ? `sí (${entry.extendedBy.join(', ') || 'sin motivo registrado'})` : 'no',
            ]),
        ),
        'En transferencia la operación es el par despacho+recepción: el throughput está en pares/s y `tx/s` cuenta las transacciones de ambos pasos, reintentos excluidos.\n',
        '### Transferencia: pasos del par\n',
        table(
            ['SUT', 'Escenario', 'Tasa', 'p95 despacho (ms)', 'p95 recepción (ms)', 'p95 par (ms)', 'Pares con reintento'],
            stable.filter((entry) => entry.dispatchP95Ms.n > 0).map((entry) => [
                entry.sut, scenarioName(entry), format(entry.targetOperationsPerSecond),
                format(entry.dispatchP95Ms.mean, 1), format(entry.receiveP95Ms.mean, 1), format(entry.transferPairP95Ms.mean, 1),
                percent(entry.retriedTransferPairRate),
            ]),
        ),
        '## Comparación Fabric frente a baseline\n',
        table(
            ['Escenario', 'Tasa', 'Throughput Fabric', 'Throughput baseline', 'Fabric/baseline', 'p95 Fabric (ms)', 'p95 baseline (ms)', 'p95 Fabric/baseline'],
            comparisons.filter((entry) => entry.scenario !== 'expected-rejections').map((entry) => [
                scenarioName(entry), format(entry.targetOperationsPerSecond), format(entry.fabric.throughput.mean),
                format(entry.baseline.throughput.mean), format(entry.throughputRatio), format(entry.fabric.p95Ms.mean, 1),
                format(entry.baseline.p95Ms.mean, 1), format(entry.p95Ratio),
            ]),
        ),
        unpaired.length
            ? `Combinaciones sin comparar (falta un SUT o no cumplen la paridad de la sección 9):\n\n${table(
                ['Escenario', 'Tasa', 'Motivo'],
                unpaired.map((entry) => [scenarioName(entry), format(entry.targetOperationsPerSecond),
                    entry.missing === 'baseline' || entry.missing === 'fabric' ? `falta ${entry.missing}` : entry.missing]),
            )}`
            : 'Todas las combinaciones tienen contraparte comparable.\n',
        '## Rechazos esperados\n',
        table(
            ['SUT', 'Variante', 'n', 'Rechazos/s', 'p95 (ms)', 'Códigos observados'],
            rejections.map((entry) => [
                entry.sut, entry.variant, entry.throughput.n, format(entry.throughput.mean), format(entry.p95Ms.mean, 1),
                Object.entries(entry.metrics.reduce((codes, metric) => {
                    for (const [code, count] of Object.entries(metric.observedErrorCodes)) codes[code] = (codes[code] ?? 0) + count;
                    return codes;
                }, {})).map(([code, count]) => `${code} ${count}`).join(', ') || 'n/d',
            ]),
        ),
        'Los rechazos se miden en rondas propias y no se suman al throughput exitoso (sección 6.5).\n',
    ];
    if (availability.length) {
        sections.push('## Disponibilidad\n', table(
            ['SUT', 'Escenario', 'Componente', 'Éxito pre-falla', 'Éxito en falla', 'Éxito post', 'p95 pre / falla / post (ms)', 'Recuperación (s)'],
            availability.map((entry) => [
                entry.sut, entry.scenario, entry.target,
                percent(entry.windows.preFault.successRate), percent(entry.windows.fault.successRate),
                percent(entry.windows.recovery?.successRate),
                [entry.windows.preFault.p95Ms, entry.windows.fault.p95Ms, entry.windows.recovery?.p95Ms].map((value) => format(value, 0)).join(' / '),
                entry.recoverySeconds === undefined ? 'sin recuperación estable' : format(entry.recoverySeconds, 1),
            ]),
        ), 'Los escenarios simulan fallas de contenedor en un host único: no demuestran la caída física ni administrativa de una organización (#44, ADR-007).\n');
    }
    sections.push(
        '## Errores inesperados y timeouts\n',
        table(['SUT', 'Escenario', 'Tasa', 'Repetición', 'Inesperados', 'Timeouts', 'Códigos'],
            unexpected.map((entry) => [entry.sut, scenarioName(entry), format(entry.targetOperationsPerSecond), entry.repetition,
                entry.unexpected.length, entry.timeouts ?? 'n/d',
                [...new Set(entry.unexpected.map((failure) => failure.code))].join(', ') || '—'])),
        '## Rondas excluidas de las estadísticas\n',
        table(['SUT', 'Escenario', 'Fase', 'Repetición', 'Motivo', 'Directorio'],
            excluded.map((run) => [run.sut, run.scenario, run.phase, run.repetition, run.exclusion, `\`${run.directory}\``])),
        'Se conservan sus crudos; no se imputan ni se reemplazan repeticiones.\n',
        '## Asimetrías estructurales declaradas\n',
        table(['Aspecto', 'Diferencia', 'Efecto sobre lo medido'], ASYMMETRIES.map((row) => [...row])),
        '## Alcance de la interpretación\n',
        '- Los valores son indicativos del comportamiento **relativo** de ambos diseños en un host único y no se extrapolan a producción.\n'
        + '- Este reporte contiene resultados cuantitativos medidos. Las propiedades cualitativas (integridad, auditabilidad) y su interpretación pertenecen al análisis de EVAL-8 y al capítulo de resultados.\n'
        + '- Ninguna dimensión aislada permite concluir la superioridad de una arquitectura (sección 10).\n',
    );
    return sections.join('\n');
}

module.exports = {ASYMMETRIES, buildReport};
