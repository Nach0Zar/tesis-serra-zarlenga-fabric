'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {availabilityWindows, successTimeline} = require('../src/availability');
const {aggregate, compare} = require('../src/metrics');
const {processResults} = require('../src/process');
const {MIX_PATTERN, exclusionReason, loadRuns, verifyMixComposition} = require('../src/runs');
const {coefficientOfVariation, describe, percentile, populationStandardDeviation} = require('../src/stats');

const DATASET = 'de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d';
const FABRIC_ENV = {docker: '27', dockerCompose: '2', contractVersion: '2.11.2', packageID: 'snt_1.0:abc', fabric: '2.5.16', fabricCA: '1.5.17', caliper: '0.7.1'};
const BASELINE_ENV = {docker: '27', dockerCompose: '2', baselineCommit: 'c'.repeat(40), baselineImage: 'snt-baseline@sha256:aa'};

function metadata({sut = 'fabric', scenario = 'read-unit', phase = 'measurement', repetition = 1, rate = 10, operation = 'query-unit',
    workers = 4, duration = 120, family, discarded, faultInjection} = {}) {
    const document = {
        sut, scenario, phase, repetition, workers, durationSeconds: duration,
        dataset: {seed: 20260727, sha256: DATASET, units: 50000},
        rate: {operation, transactionsPerOperation: operation === 'transfer' ? 2 : 1, targetOperationsPerSecond: rate,
            targetTransactionsPerSecond: rate, effectiveTransactionsPerSecond: rate},
        environment: sut === 'fabric' ? FABRIC_ENV : BASELINE_ENV,
        startedAt: '2026-10-10T10:00:00Z', endedAt: '2026-10-10T10:02:00Z',
    };
    if (family) document.rejectionFamily = family;
    if (discarded) document.discarded = {reason: discarded};
    if (faultInjection) document.faultInjection = faultInjection;
    return document;
}

function summary({throughput = 10, p50 = 5, p95 = 9, p99 = 12, seconds = 120, rejections = 0} = {}) {
    const completed = Math.round(throughput * seconds);
    return {
        operationCount: completed, successfulOperations: rejections ? 0 : completed, expectedRejections: rejections ? completed : 0,
        unexpectedFailures: 0, transactionCount: completed, successfulTransactions: rejections ? 0 : completed,
        retryAttempts: 0, retryByCause: {}, observedErrorCodes: rejections ? {UNIT_ALREADY_EXISTS: completed} : {},
        rate: {offeredOperationsPerSecond: throughput},
        operationLatency: {count: completed, minimumMs: 1, maximumMs: p99 + 1, meanMs: p50, standardDeviationMs: 1, p50Ms: p50, p95Ms: p95, p99Ms: p99},
        transactionLatencyByFunction: {},
        observedDurationSeconds: seconds,
    };
}

function writeRun(root, name, meta, sum, raw) {
    const directory = path.join(root, name);
    fs.mkdirSync(directory, {recursive: true});
    fs.writeFileSync(path.join(directory, 'metadata.json'), JSON.stringify(meta));
    if (sum) fs.writeFileSync(path.join(directory, 'summary.json'), JSON.stringify(sum));
    if (raw) fs.writeFileSync(path.join(directory, 'raw.json'), JSON.stringify(raw));
    return directory;
}

test('statistics use the population deviation and nearest-rank percentiles', () => {
    assert.equal(populationStandardDeviation([2, 4, 4, 4, 5, 5, 7, 9]), 2);
    assert.equal(coefficientOfVariation([2, 4, 4, 4, 5, 5, 7, 9]), 0.4);
    assert.equal(percentile([5, 1, 4, 2, 3], 0.95), 5);
    assert.equal(percentile([1, 2, 3, 4], 0.5), 2);
    assert.deepEqual(describe([1, Number.NaN, 3]).n, 2);
});

function mixedOperations(worker, rotation, count) {
    return Array.from({length: count}, (_, ordinal) => ({
        workerIndex: worker, ordinal, type: MIX_PATTERN[(ordinal + rotation) % MIX_PATTERN.length],
    }));
}

test('the mixed composition must follow the exact 2/11/2/5 cycle per worker', () => {
    const valid = [...mixedOperations(0, 7, 45), ...mixedOperations(1, 8, 45)];
    assert.equal(verifyMixComposition(valid).valid, true);
    const broken = [...valid];
    broken[3] = {...broken[3], type: broken[3].type === 'register' ? 'dispense' : 'register'};
    const result = verifyMixComposition(broken);
    assert.equal(result.valid, false);
    assert.match(result.reason, /worker 0/u);
});

test('warm-ups, discarded, smoke and availability rounds stay out of stable statistics', () => {
    const base = {phase: 'measurement', scenario: 'read-unit', summary: {}};
    assert.equal(exclusionReason({...base, phase: 'warmup'}), 'fase warmup');
    assert.equal(exclusionReason({...base, scenario: 'smoke'}), 'escenario smoke');
    assert.equal(exclusionReason({...base, scenario: 'raft-1'}), 'escenario de disponibilidad');
    assert.equal(exclusionReason({...base, discardReason: 'fallo'}), 'descartada: fallo');
    assert.equal(exclusionReason({...base, summary: undefined}), 'falta summary.json');
    assert.equal(exclusionReason(base), undefined);
});

test('aggregates per SUT and rate, record which metric extended the series and pair both SUT', () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-analysis-'));
    const throughputs = [10, 10, 6, 10, 10, 10, 10, 10];
    throughputs.forEach((throughput, index) => writeRun(root, `f-${index}`,
        metadata({repetition: index + 1}), summary({throughput, p95: 9})));
    for (const repetition of [1, 2, 3, 4, 5]) {
        writeRun(root, `b-${repetition}`, metadata({sut: 'baseline', repetition}), summary({throughput: 10, p95: 3}));
    }
    writeRun(root, 'b-other-workers', metadata({sut: 'baseline', rate: 25, workers: 2}), summary({throughput: 25}));
    writeRun(root, 'f-other-workers', metadata({rate: 25}), summary({throughput: 25}));
    const runs = loadRuns([root]);
    const aggregates = aggregate(runs.filter((run) => !run.exclusion));
    const fabric = aggregates.find((entry) => entry.sut === 'fabric' && entry.targetOperationsPerSecond === 10);
    assert.equal(fabric.throughput.n, 8);
    assert.equal(fabric.extended, true);
    assert.deepEqual(fabric.extendedBy, ['throughput']);
    const {comparisons, unpaired} = compare(aggregates);
    assert.equal(comparisons.length, 1);
    assert.equal(comparisons[0].p95Ratio, 3);
    assert.deepEqual(unpaired.map((entry) => entry.missing), ['workers distintos']);
});

test('availability splits the raw run into pre-fault, fault and recovery windows', () => {
    const injectedAt = Date.parse('2026-10-10T10:01:00Z');
    const operations = [];
    for (let second = -60; second < 120; second += 1) {
        const failing = second >= 0 && second < 20;
        const started = injectedAt + second * 1000;
        operations.push({
            startedAt: new Date(started).toISOString(), endedAt: new Date(started + 50).toISOString(), latencyMs: 50,
            outcome: failing ? 'unexpected-failure' : 'success',
            transactions: failing ? [{status: 'failed', errorCode: 'UNAVAILABLE'}] : [{status: 'success'}],
        });
    }
    const run = {
        raw: operations,
        faultInjection: {target: '1 orderer', injectedAt: '2026-10-10T10:01:00Z', recoveredAt: '2026-10-10T10:02:00Z',
            preFaultSeconds: 60, faultSeconds: 60, recoverySeconds: 60},
    };
    const result = availabilityWindows(run);
    assert.equal(result.windows.preFault.successRate, 1);
    assert.equal(result.windows.fault.successRate, 40 / 60);
    assert.deepEqual(result.windows.fault.failuresByCode, {UNAVAILABLE: 20});
    assert.equal(result.windows.recovery.successRate, 1);
    // Primer tramo de 30 s con exito >= 95 % del previo (seccion 3.3): el que
    // arranca en el segundo 19 tiene un solo error, 29/30 = 96,7 %.
    assert.equal(result.recoverySeconds, 19);
    const timeline = successTimeline(run, 10);
    assert.equal(timeline.find((point) => point.offsetSeconds === 0).successRate, 0);
    assert.equal(timeline.find((point) => point.offsetSeconds === 20).successRate, 1);
});

test('processing writes tables, figures and a report that lists exclusions and asymmetries', () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-analysis-in-'));
    const output = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-analysis-out-'));
    for (const sut of ['fabric', 'baseline']) {
        for (const repetition of [1, 2]) {
            writeRun(root, `${sut}-${repetition}`, metadata({sut, repetition}), summary({p95: sut === 'fabric' ? 900 : 4}));
            writeRun(root, `${sut}-dup-${repetition}`, metadata({sut, repetition, scenario: 'expected-rejections', rate: 5,
                operation: 'register', workers: 2, duration: 60, family: 'DUPLICATE_IDENTITY'}), summary({throughput: 5, rejections: 1, seconds: 60}));
        }
        writeRun(root, `${sut}-warmup`, metadata({sut, phase: 'warmup', repetition: 0}), summary());
    }
    writeRun(root, 'fabric-discarded', metadata({repetition: 3, discarded: 'pares incompletos'}), summary());
    const result = processResults({inputs: [root], output, validate: false});
    assert.equal(result.comparisons, 2);
    assert.deepEqual(result.figures, ['throughput.svg', 'latency.svg']);
    for (const file of ['repetitions.csv', 'aggregate.csv', 'comparison.csv', 'excluded.csv', 'unexpected.csv', 'report.md']) {
        assert.ok(fs.existsSync(path.join(output, file)), file);
    }
    const report = fs.readFileSync(path.join(output, 'report.md'), 'utf8');
    assert.match(report, /contrato `2\.11\.2`/u);
    assert.match(report, /descartada: pares incompletos/u);
    assert.match(report, /lab_intervention_events/u);
    assert.match(report, /DUPLICATE_IDENTITY\/register/u);
    assert.match(fs.readFileSync(path.join(output, 'figures', 'latency.svg'), 'utf8'), /^<svg /u);
});
