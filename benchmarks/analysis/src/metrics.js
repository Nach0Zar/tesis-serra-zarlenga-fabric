'use strict';

const {comparisonKey, failureListing, groupKey} = require('./runs');
const {describe, mean, percentile} = require('./stats');

const EXTENSION_THRESHOLD = 0.15;
const BASE_REPETITIONS = 5;

function markerMetrics(markers) {
    if (!markers) return undefined;
    return {
        expected: markers.expected,
        observed: markers.observed,
        fromRegistrations: markers.fromRegistrations,
        fromRegulatoryEvents: markers.fromRegulatoryEvents,
        perSecond: markers.perSecond,
        shareOfSuccessfulWrites: markers.shareOfSuccessfulWrites,
    };
}

// Metricas de una repeticion. El throughput es de operaciones conceptuales
// exitosas (pares/s en transferencia) y, en las rondas de rechazo, de rechazos
// esperados: nunca se suman al camino feliz (seccion 6.5).
function runMetrics(run) {
    const summary = run.summary;
    const seconds = summary.observedDurationSeconds;
    const completed = run.scenario === 'expected-rejections' ? summary.expectedRejections : summary.successfulOperations;
    const latency = summary.operationLatency ?? {};
    const byFunction = summary.transactionLatencyByFunction ?? {};
    const failures = run.raw ? failureListing(run.raw) : {unexpected: [], timeouts: undefined};
    // Latencia end-to-end del par (seccion 3.4) solo sobre transferencias: en la
    // mezcla, operationLatency agrupa todas las operaciones.
    const pairLatencies = (run.raw ?? [])
        .filter((operation) => operation.type === 'transfer' && operation.outcome === 'success')
        .map((operation) => operation.latencyMs);
    return {
        sut: run.sut,
        scenario: run.scenario,
        variant: run.variant,
        operation: run.operation,
        targetOperationsPerSecond: run.targetOperationsPerSecond,
        repetition: run.repetition,
        operations: summary.operationCount,
        completed,
        throughput: completed / seconds,
        // Dos tasas fisicas con denominadores distintos (seccion 9.3): las
        // transacciones confirmadas y todos los intentos enviados al SUT, que
        // incluyen los reintentos transitorios de la seccion 3.4.
        confirmedTransactionThroughput: summary.successfulTransactions / seconds,
        attemptedTransactionThroughput: summary.transactionCount / seconds,
        transactionAttempts: summary.transactionCount,
        confirmedTransactions: summary.successfulTransactions,
        offeredOperationsPerSecond: summary.rate?.offeredOperationsPerSecond,
        observedDurationSeconds: seconds,
        latencyMeanMs: latency.meanMs,
        latencyStandardDeviationMs: latency.standardDeviationMs,
        p50Ms: latency.p50Ms,
        p95Ms: latency.p95Ms,
        p99Ms: latency.p99Ms,
        dispatchP95Ms: byFunction.DispatchTransfer?.p95Ms,
        receiveP95Ms: byFunction.ReceiveTransfer?.p95Ms,
        transferPairP95Ms: percentile(pairLatencies, 0.95),
        retryAttempts: summary.retryAttempts ?? 0,
        retriedTransferPairRate: summary.retriedTransferPairRate,
        retryTimeMs: summary.retryTimeMs,
        retryByCause: summary.retryByCause ?? {},
        expectedRejections: summary.expectedRejections ?? 0,
        unexpectedFailures: summary.unexpectedFailures ?? 0,
        observedErrorCodes: summary.observedErrorCodes ?? {},
        timeouts: failures.timeouts,
        unexpected: failures.unexpected,
        markers: markerMetrics(run.participationMarkers),
        nonCitable: run.nonCitable,
        snapshotId: run.accreditation?.snapshot?.id,
        seriesToken: run.accreditation?.series?.token,
        runDirectory: run.directory,
    };
}

function uniqueValues(values) {
    return [...new Set(values.map((value) => JSON.stringify(value)))].map((value) => JSON.parse(value));
}

const DESCRIBED = Object.freeze([
    'throughput', 'confirmedTransactionThroughput', 'attemptedTransactionThroughput', 'p50Ms', 'p95Ms', 'p99Ms',
    'latencyMeanMs', 'dispatchP95Ms', 'receiveP95Ms', 'transferPairP95Ms',
]);

function summarize(metrics) {
    const statistics = {repetitions: metrics.map((metric) => metric.repetition).sort((left, right) => left - right)};
    for (const name of DESCRIBED) statistics[name] = describe(metrics.map((metric) => metric[name]));
    statistics.retriedTransferPairRate = mean(metrics.map((metric) => metric.retriedTransferPairRate).filter(Number.isFinite));
    statistics.retryAttempts = metrics.reduce((sum, metric) => sum + metric.retryAttempts, 0);
    return statistics;
}

// Costo de los marcadores (seccion 3.5): se informa cuando la ronda lo declara y
// se marca toda diferencia entre esperados y observados.
function summarizeMarkers(metrics) {
    const declared = metrics.filter((metric) => metric.markers);
    if (declared.length === 0) return {declaredRounds: 0, rounds: metrics.length};
    const mismatched = declared.filter((metric) => metric.markers.expected !== metric.markers.observed)
        .map((metric) => metric.repetition);
    return {
        declaredRounds: declared.length,
        rounds: metrics.length,
        expected: describe(declared.map((metric) => metric.markers.expected)),
        observed: describe(declared.map((metric) => metric.markers.observed)),
        fromRegistrations: describe(declared.map((metric) => metric.markers.fromRegistrations)),
        fromRegulatoryEvents: describe(declared.map((metric) => metric.markers.fromRegulatoryEvents)),
        perSecond: describe(declared.map((metric) => metric.markers.perSecond)),
        shareOfSuccessfulWrites: describe(declared.map((metric) => metric.markers.shareOfSuccessfulWrites)),
        mismatchedRepetitions: mismatched,
    };
}

// Agregado por SUT, escenario, variante, tasa y condiciones de carga. Siempre se
// informa la serie original de cinco; si hubo extension, tambien la de ocho
// (seccion 7), con la metrica que la disparo.
function aggregate(includedRuns) {
    const groups = new Map();
    for (const run of includedRuns) {
        const key = groupKey(run);
        const entry = groups.get(key) ?? {runs: [], metrics: []};
        entry.runs.push(run);
        entry.metrics.push(runMetrics(run));
        groups.set(key, entry);
    }
    return [...groups.values()].map(({runs, metrics}) => {
        const first = runs[0];
        const originalMetrics = metrics.filter((metric) => metric.repetition <= BASE_REPETITIONS);
        const original = summarize(originalMetrics);
        const extended = metrics.length > originalMetrics.length ? summarize(metrics) : undefined;
        const extendedBy = [];
        if (extended) {
            if ((original.throughput.coefficientOfVariation ?? 0) > EXTENSION_THRESHOLD) extendedBy.push('throughput');
            if ((original.p95Ms.coefficientOfVariation ?? 0) > EXTENSION_THRESHOLD) extendedBy.push('p95');
        }
        const problems = [];
        const datasets = uniqueValues(runs.map((run) => run.datasetSHA256));
        const artifacts = uniqueValues(runs.map((run) => run.artifact));
        const snapshots = uniqueValues(runs.map((run) => run.accreditation?.snapshot?.id ?? null));
        if (datasets.length !== 1) problems.push('repeticiones con distinto dataset');
        if (artifacts.length !== 1) problems.push('repeticiones con distinto artefacto');
        if (snapshots.length !== 1) problems.push('repeticiones con distinto snapshot');
        const repetitions = metrics.map((metric) => metric.repetition).sort((left, right) => left - right);
        if (new Set(repetitions).size !== repetitions.length) problems.push('repeticiones duplicadas');
        const markers = summarizeMarkers(metrics);
        if (markers.mismatchedRepetitions?.length) {
            problems.push(`marcadores esperados distintos de observados en repeticiones ${markers.mismatchedRepetitions.join(', ')}`);
        }
        const reported = extended ?? original;
        return {
            sut: first.sut,
            scenario: first.scenario,
            variant: first.variant,
            operation: first.operation,
            targetOperationsPerSecond: first.targetOperationsPerSecond,
            transactionsPerOperation: first.transactionsPerOperation,
            workers: first.workers,
            durationSeconds: first.durationSeconds,
            mix: first.mix,
            datasetSHA256: datasets[0],
            artifact: artifacts[0],
            snapshotId: snapshots[0],
            nonCitable: runs.some((run) => run.nonCitable),
            repetitions,
            extended: Boolean(extended),
            extendedBy,
            original,
            extendedSeries: extended,
            // Atajos sobre la serie que se reporta (la extendida si existe).
            ...Object.fromEntries(DESCRIBED.map((name) => [name, reported[name]])),
            retriedTransferPairRate: reported.retriedTransferPairRate,
            unexpectedFailures: metrics.reduce((sum, metric) => sum + metric.unexpectedFailures, 0),
            markers,
            problems,
            metrics,
        };
    }).sort((left, right) => left.scenario.localeCompare(right.scenario)
        || left.variant.localeCompare(right.variant)
        || left.targetOperationsPerSecond - right.targetOperationsPerSecond
        || left.sut.localeCompare(right.sut));
}

// Paridad de la seccion 9: mismo escenario, dataset, workers, duracion y bloque
// de tasa objetivo. Un par que no la cumple no se compara.
function parityProblems(fabric, baseline) {
    const problems = [];
    if (fabric.datasetSHA256 !== baseline.datasetSHA256) problems.push('dataset distinto');
    if (fabric.workers !== baseline.workers) problems.push('workers distintos');
    if (fabric.durationSeconds !== baseline.durationSeconds) problems.push('duración distinta');
    if (fabric.operation !== baseline.operation) problems.push('operación distinta');
    if (Math.abs(fabric.transactionsPerOperation - baseline.transactionsPerOperation) > 1e-9) problems.push('transacciones por operación distintas');
    if (JSON.stringify(fabric.mix ?? null) !== JSON.stringify(baseline.mix ?? null)) problems.push('mezcla distinta');
    if (fabric.problems.length || baseline.problems.length) problems.push('grupo inconsistente');
    if (fabric.nonCitable !== baseline.nonCitable) problems.push('un lado no es citable');
    return problems;
}

function ratio(left, right) {
    return Number.isFinite(left) && Number.isFinite(right) && right !== 0 ? left / right : undefined;
}

function ratios(fabric, baseline) {
    return {
        throughputRatio: ratio(fabric.throughput.mean, baseline.throughput.mean),
        p50Ratio: ratio(fabric.p50Ms.mean, baseline.p50Ms.mean),
        p95Ratio: ratio(fabric.p95Ms.mean, baseline.p95Ms.mean),
        p99Ratio: ratio(fabric.p99Ms.mean, baseline.p99Ms.mean),
    };
}

function unpairedEntry(key, entry, missing) {
    return {key, scenario: entry.scenario, variant: entry.variant,
        targetOperationsPerSecond: entry.targetOperationsPerSecond, missing};
}

// Fabric frente a baseline sobre repeticiones emparejadas por numero. Las que
// faltan de un lado se listan; nunca se completa ni se compara un agregado
// contra repeticiones que el otro SUT no tiene.
function compare(aggregates) {
    const bySide = new Map();
    for (const entry of aggregates) {
        const key = comparisonKey(entry);
        const pair = bySide.get(key) ?? {fabric: [], baseline: []};
        pair[entry.sut].push(entry);
        bySide.set(key, pair);
    }
    const comparisons = [];
    const unpaired = [];
    for (const [key, pair] of bySide) {
        if (pair.fabric.length === 0 || pair.baseline.length === 0) {
            const present = pair.fabric[0] ?? pair.baseline[0];
            unpaired.push(unpairedEntry(key, present, `falta ${pair.fabric.length ? 'baseline' : 'fabric'}`));
            continue;
        }
        if (pair.fabric.length > 1 || pair.baseline.length > 1) {
            unpaired.push(unpairedEntry(key, pair.fabric[0], 'repeticiones con condiciones de carga distintas'));
            continue;
        }
        const [fabric] = pair.fabric;
        const [baseline] = pair.baseline;
        const problems = parityProblems(fabric, baseline);
        if (problems.length > 0) {
            unpaired.push(unpairedEntry(key, fabric, problems.join(', ')));
            continue;
        }
        const baselineRepetitions = new Set(baseline.repetitions);
        const fabricRepetitions = new Set(fabric.repetitions);
        const paired = fabric.repetitions.filter((repetition) => baselineRepetitions.has(repetition));
        const missing = {
            fabric: baseline.repetitions.filter((repetition) => !fabricRepetitions.has(repetition)),
            baseline: fabric.repetitions.filter((repetition) => !baselineRepetitions.has(repetition)),
        };
        if (paired.length === 0) {
            unpaired.push(unpairedEntry(key, fabric, 'sin repeticiones emparejadas'));
            continue;
        }
        const series = [['original', paired.filter((repetition) => repetition <= BASE_REPETITIONS)]];
        if (paired.some((repetition) => repetition > BASE_REPETITIONS)) series.push(['extendida', paired]);
        for (const [label, repetitions] of series) {
            if (repetitions.length === 0) continue;
            const selected = new Set(repetitions);
            const fabricStats = summarize(fabric.metrics.filter((metric) => selected.has(metric.repetition)));
            const baselineStats = summarize(baseline.metrics.filter((metric) => selected.has(metric.repetition)));
            comparisons.push({
                scenario: fabric.scenario,
                variant: fabric.variant,
                targetOperationsPerSecond: fabric.targetOperationsPerSecond,
                series: label,
                pairedRepetitions: repetitions,
                missingRepetitions: missing,
                nonCitable: fabric.nonCitable,
                fabric: {...fabricStats, artifact: fabric.artifact, datasetSHA256: fabric.datasetSHA256, snapshotId: fabric.snapshotId},
                baseline: {...baselineStats, artifact: baseline.artifact, snapshotId: baseline.snapshotId},
                ...ratios(fabricStats, baselineStats),
            });
        }
    }
    return {comparisons, unpaired};
}

module.exports = {BASE_REPETITIONS, EXTENSION_THRESHOLD, aggregate, compare, parityProblems, runMetrics, summarize};
