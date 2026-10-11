'use strict';

const {comparisonKey, failureListing, groupKey} = require('./runs');
const {describe, mean, percentile} = require('./stats');

const EXTENSION_THRESHOLD = 0.15;
const BASE_REPETITIONS = 5;

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
        transactionThroughput: summary.successfulTransactions / seconds,
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
        runDirectory: run.directory,
    };
}

function uniqueValues(values) {
    return [...new Set(values.map((value) => JSON.stringify(value)))].map((value) => JSON.parse(value));
}

// Agregado por SUT, escenario, variante y tasa: media y desvio entre
// repeticiones, nunca un promedio entre workloads distintos.
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
        const throughput = describe(metrics.map((metric) => metric.throughput));
        const p95 = describe(metrics.map((metric) => metric.p95Ms));
        const extendedBy = [];
        if (metrics.length > BASE_REPETITIONS) {
            const original = metrics.filter((metric) => metric.repetition <= BASE_REPETITIONS);
            if ((describe(original.map((metric) => metric.throughput)).coefficientOfVariation ?? 0) > EXTENSION_THRESHOLD) {
                extendedBy.push('throughput');
            }
            if ((describe(original.map((metric) => metric.p95Ms)).coefficientOfVariation ?? 0) > EXTENSION_THRESHOLD) {
                extendedBy.push('p95');
            }
        }
        const problems = [];
        const datasets = uniqueValues(runs.map((run) => run.datasetSHA256));
        const artifacts = uniqueValues(runs.map((run) => run.artifact));
        if (datasets.length !== 1) problems.push('repeticiones con distinto dataset');
        if (artifacts.length !== 1) problems.push('repeticiones con distinto artefacto');
        const repetitions = metrics.map((metric) => metric.repetition).sort((left, right) => left - right);
        if (new Set(repetitions).size !== repetitions.length) problems.push('repeticiones duplicadas');
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
            repetitions,
            extended: metrics.length > BASE_REPETITIONS,
            extendedBy,
            throughput,
            transactionThroughput: describe(metrics.map((metric) => metric.transactionThroughput)),
            p50Ms: describe(metrics.map((metric) => metric.p50Ms)),
            p95Ms: p95,
            p99Ms: describe(metrics.map((metric) => metric.p99Ms)),
            latencyMeanMs: describe(metrics.map((metric) => metric.latencyMeanMs)),
            dispatchP95Ms: describe(metrics.map((metric) => metric.dispatchP95Ms)),
            receiveP95Ms: describe(metrics.map((metric) => metric.receiveP95Ms)),
            transferPairP95Ms: describe(metrics.map((metric) => metric.transferPairP95Ms)),
            retriedTransferPairRate: mean(metrics.map((metric) => metric.retriedTransferPairRate).filter(Number.isFinite)),
            unexpectedFailures: metrics.reduce((sum, metric) => sum + metric.unexpectedFailures, 0),
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
    return problems;
}

function ratio(left, right) {
    return Number.isFinite(left) && Number.isFinite(right) && right !== 0 ? left / right : undefined;
}

function compare(aggregates) {
    const bySide = new Map();
    for (const entry of aggregates) {
        const key = comparisonKey(entry);
        const pair = bySide.get(key) ?? {};
        pair[entry.sut] = entry;
        bySide.set(key, pair);
    }
    const comparisons = [];
    const unpaired = [];
    for (const [key, pair] of bySide) {
        if (!pair.fabric || !pair.baseline) {
            const present = pair.fabric ?? pair.baseline;
            unpaired.push({key, scenario: present.scenario, variant: present.variant,
                targetOperationsPerSecond: present.targetOperationsPerSecond, missing: pair.fabric ? 'baseline' : 'fabric'});
            continue;
        }
        const problems = parityProblems(pair.fabric, pair.baseline);
        if (problems.length > 0) {
            unpaired.push({key, scenario: pair.fabric.scenario, variant: pair.fabric.variant,
                targetOperationsPerSecond: pair.fabric.targetOperationsPerSecond, missing: problems.join(', ')});
            continue;
        }
        comparisons.push({
            scenario: pair.fabric.scenario,
            variant: pair.fabric.variant,
            targetOperationsPerSecond: pair.fabric.targetOperationsPerSecond,
            fabric: pair.fabric,
            baseline: pair.baseline,
            throughputRatio: ratio(pair.fabric.throughput.mean, pair.baseline.throughput.mean),
            p50Ratio: ratio(pair.fabric.p50Ms.mean, pair.baseline.p50Ms.mean),
            p95Ratio: ratio(pair.fabric.p95Ms.mean, pair.baseline.p95Ms.mean),
            p99Ratio: ratio(pair.fabric.p99Ms.mean, pair.baseline.p99Ms.mean),
        });
    }
    return {comparisons, unpaired};
}

module.exports = {BASE_REPETITIONS, EXTENSION_THRESHOLD, aggregate, compare, parityProblems, runMetrics};
