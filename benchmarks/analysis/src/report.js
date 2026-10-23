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

function statisticsRow(entry, label, statistics) {
    return [
        entry.sut, scenarioName(entry), format(entry.targetOperationsPerSecond), label, statistics.repetitions.join(' '),
        `${format(statistics.throughput.mean)} ± ${format(statistics.throughput.standardDeviation)}`,
        percent(statistics.throughput.coefficientOfVariation), format(statistics.p50Ms.mean, 1),
        format(statistics.p95Ms.mean, 1), percent(statistics.p95Ms.coefficientOfVariation), format(statistics.p99Ms.mean, 1),
        format(statistics.confirmedTransactionThroughput.mean), format(statistics.attemptedTransactionThroughput.mean),
    ];
}

function seriesRows(entry) {
    const rows = [statisticsRow(entry, 'original', entry.original)];
    if (entry.extendedSeries) {
        rows.push(statisticsRow(entry, `extendida (${entry.extendedBy.join(', ') || 'sin motivo registrado'})`, entry.extendedSeries));
    }
    return rows;
}

// El componente se restaura salvo en Raft-2, que lo hace fuera de la ronda.
function restorationLabel(entry) {
    if (entry.restoration?.status === 'failed') return entry.reconstruction ? 'fallida (evidencia reconstruida)' : 'fallida';
    return entry.windows?.recovery ? 'correcta' : 'fuera de la ronda';
}

function restorationSection(availability) {
    const failed = availability.filter((entry) => entry.restoration?.status === 'failed');
    if (failed.length === 0) return [];
    const blocks = failed.map((entry) => {
        const lines = [
            `- **${entry.sut} ${entry.scenario}, repetición ${entry.repetition}.** ${entry.restoration.reason}. `
            + 'La carga y sus ventanas se informan arriba: el servicio se midió completo; lo que falló fue el reingreso del componente.',
            ...entry.restoration.evidence.map((line) => `  - \`${line.slice(0, 400)}\``),
        ];
        if (entry.reconstruction) {
            const record = entry.reconstruction;
            lines.push(`  - Evidencia reconstruida el ${record.reconstructedAt} con \`${record.tool}\` (commit \`${record.toolCommit.slice(0, 7)}\`); `
                + `la ronda se ejecutó en \`${record.executionCommit.slice(0, 7)}\`. No se recuperaron: ${record.notRecovered.join('; ')}.`);
            lines.push(`  - Insumos: ${record.inputs.map((input) => `\`${input.path}\` (${input.sha256.slice(0, 12)}…)`).join(', ')}.`);
        }
        return lines.join('\n');
    });
    return [
        '### Restauraciones fallidas\n',
        'El intento figura como `failed` en el índice de su serie y se informa como resultado observado del escenario, dentro del balance de sus repeticiones.\n',
        `${blocks.join('\n')}\n`,
    ];
}

function availabilityRows(entry) {
    const row = (label, result) => [
        entry.sut, entry.scenario, entry.repetition, label, entry.target,
        percent(result.windows.preFault.successRate), percent(result.windows.fault.successRate),
        percent(result.windows.recovery?.successRate),
        [result.windows.preFault.p95Ms, result.windows.fault.p95Ms, result.windows.recovery?.p95Ms].map((value) => format(value, 0)).join(' / '),
        result.recoverySeconds === undefined ? 'sin recuperación estable' : format(result.recoverySeconds, 1),
        restorationLabel(entry),
    ];
    if (entry.blocked) return [[entry.sut, entry.scenario, entry.repetition, 'bloqueado', entry.target, entry.blocked, '', '', '', '', '']];
    const rows = [row('global', entry)];
    for (const [name, result] of Object.entries(entry.byClass ?? {})) {
        if (result) rows.push(row(name === 'target' ? 'objetivo' : 'control', result));
    }
    return rows;
}

function buildReport({aggregates, comparisons, unpaired, excluded, failures, preparation = [], availability, inputs, nonCitable}) {
    const stable = aggregates.filter((entry) => entry.scenario !== 'expected-rejections');
    const rejections = aggregates.filter((entry) => entry.scenario === 'expected-rejections');
    const sections = [
        `# ${nonCitable ? 'NO CITABLE · ' : ''}Resultados comparativos Fabric y baseline\n`,
        nonCitable
            ? '> **Resultados no citables.** Se admitieron mediciones de series no citables (por ejemplo un `trial`) o sin índice de serie con `--allow-noncitable`. Sirven como diagnóstico y no pueden usarse en el capítulo de resultados.\n'
            : 'Sólo entran mediciones de series citables con el estado inicial acreditado por el manifiesto de su snapshot.\n',
        `Procesado de ${inputs.map((input) => `\`${input}\``).join(', ')}. Cada fila agrega repeticiones medidas válidas de un mismo SUT, escenario, variante, tasa y condiciones de carga: no hay promedios entre workloads ni cargas distintas. Desvío y coeficiente de variación son poblacionales; los percentiles son nearest-rank por ronda y se promedian entre repeticiones.\n`,
        '## Artefactos medidos\n',
        artifactsSection(aggregates),
        '## Rendimiento estable por SUT\n',
        table(
            ['SUT', 'Escenario', 'Tasa', 'Serie', 'Repeticiones', 'Throughput (op/s)', 'CV thr.', 'p50 (ms)', 'p95 (ms)', 'CV p95', 'p99 (ms)', 'tx confirmadas/s', 'intentos tx/s'],
            stable.flatMap(seriesRows),
        ),
        'La serie original son las repeticiones 1 a 5; la extendida, las 1 a 8 cuando el CV de throughput o p95 de la original superó 15 % (sección 7). En transferencia el throughput está en pares/s. `tx confirmadas/s` cuenta las transacciones confirmadas de ambos pasos; `intentos tx/s` cuenta todos los intentos enviados al SUT, incluidos los reintentos transitorios de la sección 3.4, que son parte del costo observado.\n',
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
        'Sólo sobre repeticiones emparejadas por número; las faltantes de cada lado se listan y no se completan.\n',
        table(
            ['Escenario', 'Tasa', 'Serie', 'Emparejadas', 'Faltan en Fabric', 'Faltan en baseline', 'Throughput Fabric', 'Throughput baseline', 'Fabric/baseline', 'p95 Fabric (ms)', 'p95 baseline (ms)', 'p95 Fabric/baseline'],
            comparisons.filter((entry) => entry.scenario !== 'expected-rejections').map((entry) => [
                scenarioName(entry), format(entry.targetOperationsPerSecond), entry.series, entry.pairedRepetitions.join(' '),
                entry.missingRepetitions.fabric.join(' ') || '—', entry.missingRepetitions.baseline.join(' ') || '—',
                format(entry.fabric.throughput.mean), format(entry.baseline.throughput.mean), format(entry.throughputRatio),
                format(entry.fabric.p95Ms.mean, 1), format(entry.baseline.p95Ms.mean, 1), format(entry.p95Ratio),
            ]),
        ),
        unpaired.length
            ? `Combinaciones sin comparar:\n\n${table(
                ['Escenario', 'Tasa', 'Motivo'],
                unpaired.map((entry) => [scenarioName(entry), format(entry.targetOperationsPerSecond), entry.missing]),
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
        '## Marcadores de participación\n',
        table(
            ['SUT', 'Escenario', 'Tasa', 'Rondas que los declaran', 'Esperados (media)', 'Observados (media)', 'Altas', 'Eventos regulatorios', 'Por segundo', 'Proporción de escrituras'],
            aggregates.filter((entry) => entry.sut === 'fabric').map((entry) => (entry.markers.declaredRounds
                ? [entry.sut, scenarioName(entry), format(entry.targetOperationsPerSecond),
                    `${entry.markers.declaredRounds}/${entry.markers.rounds}`, format(entry.markers.expected.mean),
                    format(entry.markers.observed.mean), format(entry.markers.fromRegistrations.mean),
                    format(entry.markers.fromRegulatoryEvents.mean), format(entry.markers.perSecond.mean),
                    percent(entry.markers.shareOfSuccessfulWrites.mean)]
                : [entry.sut, scenarioName(entry), format(entry.targetOperationsPerSecond), `0/${entry.markers.rounds}`,
                    'no informado', '', '', '', '', ''])),
        ),
        'La sección 3.5 pide atribuir el costo de los marcadores por escenario. Una ronda que no declara `participationMarkers` en su metadata queda como «no informado»: su costo está dentro de la latencia y el throughput, pero no puede atribuirse. Una diferencia entre esperados y observados invalida el grupo.\n',
        preparation.length
            ? `### Preparación del snapshot\n\n${table(
                ['SUT', 'Duración (s)', 'Escrituras confirmadas', 'Esperados', 'Observados', 'Altas', 'Eventos regulatorios', 'Por segundo', 'Proporción'],
                preparation.map((row) => [row.sut, format(row.durationSeconds, 0), format(row.successfulWriteTransactions, 0),
                    format(row.markersExpected, 0), format(row.markersObserved, 0), format(row.markersFromRegistrations, 0),
                    format(row.markersFromRegulatoryEvents, 0), format(row.markersPerSecond), percent(row.markersShareOfSuccessfulWrites)]),
            )}La preparación es su propia fase y no se mezcla con el rendimiento medido (sección 9.6).\n`
            : '',
    ];
    if (availability.length) {
        sections.push('## Disponibilidad\n', table(
            ['SUT', 'Escenario', 'Repetición', 'Clase', 'Componente', 'Éxito pre-falla', 'Éxito en falla', 'Éxito post', 'p95 pre / falla / post (ms)', 'Recuperación (s)', 'Restauración'],
            availability.flatMap(availabilityRows),
        ), 'En las caídas de peer (sección 8.1) la disponibilidad se informa por clase: operaciones que requieren el peer caído (objetivo) y de control. Sin esa clasificación en el crudo el resultado se bloquea. Los escenarios simulan fallas de contenedor en un host único: no demuestran la caída física ni administrativa de una organización (#44, ADR-007).\n',
            ...restorationSection(availability));
    }
    sections.push(
        '## Errores inesperados y timeouts\n',
        table(['SUT', 'Escenario', 'Tasa', 'Fase', 'Repetición', 'Estado', 'Unidad', 'Código', 'Timeouts en la ronda'],
            failures.map((row) => [row.sut, scenarioName(row), format(row.targetOperationsPerSecond), row.phase, row.repetition,
                row.status, row.datasetSequence ?? '—', row.code ?? '—', row.timeouts ?? 0])),
        'Incluye las rondas excluidas: son justamente las que contienen los fallos.\n',
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
