'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {availabilityWindows, successTimeline} = require('../src/availability');
const {CANONICAL_COUNTS} = require('../src/evidence');
const {aggregate, compare} = require('../src/metrics');
const {processResults} = require('../src/process');
const {MIX_PATTERN, exclusionReason, verifyMixComposition} = require('../src/runs');
const {coefficientOfVariation, describe, percentile, populationStandardDeviation} = require('../src/stats');

const DATASET = 'de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d';
const FABRIC_ENV = {docker: '27', dockerCompose: '2', contractVersion: '2.11.2', packageID: 'snt_1.0:abc', fabric: '2.5.16', fabricCA: '1.5.17', caliper: '0.7.1'};
const BASELINE_ENV = {docker: '27', dockerCompose: '2', baselineCommit: 'c'.repeat(40), baselineImage: 'snt-baseline@sha256:aa'};

function temporary(prefix) {
    return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

function writeJSON(filePath, value) {
    fs.mkdirSync(path.dirname(filePath), {recursive: true});
    fs.writeFileSync(filePath, JSON.stringify(value));
}

function sha256(filePath) {
    return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex');
}

function metadata({sut = 'fabric', scenario = 'read-unit', phase = 'measurement', repetition = 1, rate = 10, operation = 'query-unit',
    workers = 4, duration = 120, family, discarded, faultInjection, markers} = {}) {
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
    if (markers) document.participationMarkers = markers;
    return document;
}

function summary({throughput = 10, p50 = 5, p95 = 9, p99 = 12, seconds = 120, rejections = 0, attempts} = {}) {
    const completed = Math.round(throughput * seconds);
    return {
        operationCount: completed, successfulOperations: rejections ? 0 : completed, expectedRejections: rejections ? completed : 0,
        unexpectedFailures: 0, transactionCount: attempts ?? completed, successfulTransactions: rejections ? 0 : completed,
        retryAttempts: (attempts ?? completed) - completed, retryByCause: {}, observedErrorCodes: rejections ? {UNIT_ALREADY_EXISTS: completed} : {},
        rate: {offeredOperationsPerSecond: throughput},
        operationLatency: {count: completed, minimumMs: 1, maximumMs: p99 + 1, meanMs: p50, standardDeviationMs: 1, p50Ms: p50, p95Ms: p95, p99Ms: p99},
        transactionLatencyByFunction: {},
        observedDurationSeconds: seconds,
    };
}

function raw(operations = 3, overrides = {}) {
    return Array.from({length: operations}, (_, index) => ({
        workerIndex: 0, ordinal: index, type: 'query-unit', outcome: 'success', latencyMs: 5,
        startedAt: '2026-10-10T10:00:01Z', endedAt: '2026-10-10T10:00:01Z', transactions: [{status: 'success'}],
        ...overrides,
    }));
}

// Snapshot acreditable: manifiesto con conteos canonicos del mismo dataset.
function writeSnapshot(root, id, counts = CANONICAL_COUNTS) {
    const directory = path.join(root, 'snapshots', id.slice(0, 8));
    writeJSON(path.join(directory, 'manifest.json'), {snapshotId: id, dataset: {sha256: DATASET}, counts,
        files: {logicalPlan: {sha256: 'f'.repeat(64)}}});
    return {directory, id, manifestSHA256: sha256(path.join(directory, 'manifest.json'))};
}

// Serie con indice (#141) y una ronda por entrada, en el formato del orquestador.
function writeSeries(root, {sut = 'fabric', plan = 'full', snapshot, rounds}) {
    const directory = path.join(root, `${sut}-${plan}-${crypto.randomUUID().slice(0, 6)}`);
    const scenarios = new Map();
    for (const round of rounds) {
        const key = `${round.meta.scenario ?? 'read-unit'}-${round.meta.rate ?? 10}`;
        const kind = (round.meta.phase ?? 'measurement') === 'warmup' ? 'warmup' : 'measurement';
        const attemptDirectory = kind === 'warmup' ? `${key}/warmup/try-1`
            : `${key}/measurements/run-${String(round.meta.repetition ?? 1).padStart(2, '0')}`;
        const roundDirectory = path.join(directory, attemptDirectory, 'round');
        writeJSON(path.join(roundDirectory, 'metadata.json'), metadata({sut, ...round.meta}));
        writeJSON(path.join(roundDirectory, 'summary.json'), summary(round.sum));
        if (round.raw !== null) writeJSON(path.join(roundDirectory, 'raw.json'), round.raw ?? raw());
        if (snapshot) {
            writeJSON(path.join(roundDirectory, 'snapshot-context.json'),
                sut === 'fabric' ? {snapshotId: snapshot.id, manifestSHA256: snapshot.manifestSHA256}
                    : {snapshot: {id: snapshot.id, manifestSHA256: snapshot.manifestSHA256}, counts: CANONICAL_COUNTS});
        }
        const scenario = scenarios.get(key) ?? {key, scenario: round.meta.scenario ?? 'read-unit', rate: round.meta.rate ?? 10,
            status: 'completed', extended: false, attempts: []};
        scenario.attempts.push({kind, repetition: kind === 'warmup' ? 0 : round.meta.repetition ?? 1, try: 1,
            status: round.attemptStatus ?? 'completed', startedAt: '2026-10-10T10:00:00.000Z',
            endedAt: '2026-10-10T10:02:00.000Z', directory: attemptDirectory});
        scenarios.set(key, scenario);
    }
    writeJSON(path.join(directory, 'series-index.json'), {
        $schema: 'urn:pfi-snt:fabric-series-index:schema:1.0.0', schemaVersion: '1.0.0', seriesToken: 'token',
        plan, citable: plan === 'full', snapshot: snapshot ? {id: snapshot.id, directory: 'x', manifestSHA256: snapshot.manifestSHA256} : undefined,
        startedAt: '2026-10-10T10:00:00.000Z', endedAt: '2026-10-10T11:00:00.000Z', status: 'completed',
        scenarios: [...scenarios.values()],
    });
    return directory;
}

function run(root, options = {}) {
    const output = temporary('snt-analysis-out-');
    return {output, result: processResults({inputs: [root], output, validate: false, snapshots: [], ...options})};
}

function parseCSVLine(line) {
    const values = [];
    let current = '';
    let quoted = false;
    for (let index = 0; index < line.length; index += 1) {
        const character = line[index];
        if (quoted && character === '"' && line[index + 1] === '"') {
            current += '"';
            index += 1;
        } else if (character === '"') {
            quoted = !quoted;
        } else if (character === ',' && !quoted) {
            values.push(current);
            current = '';
        } else {
            current += character;
        }
    }
    values.push(current);
    return values;
}

function readCSV(filePath) {
    const [header, ...lines] = fs.readFileSync(filePath, 'utf8').trim().split('\n');
    const columns = parseCSVLine(header);
    return lines.filter(Boolean).map((line) => Object.fromEntries(parseCSVLine(line).map((value, index) => [columns[index], value])));
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

test('warm-ups, discarded, smoke, availability and rounds without raw evidence stay out', () => {
    const base = {phase: 'measurement', scenario: 'read-unit', summary: {}, raw: []};
    assert.equal(exclusionReason({...base, phase: 'warmup'}), 'fase warmup');
    assert.equal(exclusionReason({...base, scenario: 'smoke'}), 'escenario smoke');
    assert.equal(exclusionReason({...base, scenario: 'raft-1'}), 'escenario de disponibilidad');
    assert.equal(exclusionReason({...base, discardReason: 'fallo'}), 'descartada: fallo');
    assert.equal(exclusionReason({...base, summary: undefined}), 'falta summary.json');
    assert.equal(exclusionReason({...base, raw: undefined}), 'falta raw.json');
    assert.equal(exclusionReason({...base, scenario: 'mixed', raw: undefined}), 'falta raw.json');
    assert.equal(exclusionReason(base), undefined);
});

test('only accredited rounds of citable series enter the statistics', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root, 'a'.repeat(64));
    writeSeries(root, {plan: 'full', snapshot, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}, attemptStatus: 'discarded'}]});
    writeSeries(root, {plan: 'trial', snapshot, rounds: [{meta: {repetition: 1}}]});
    writeSeries(root, {plan: 'full', rounds: [{meta: {repetition: 3}}]});
    const unknown = {id: 'b'.repeat(64), manifestSHA256: 'c'.repeat(64)};
    writeSeries(root, {plan: 'full', snapshot: unknown, rounds: [{meta: {repetition: 4}}]});

    const {output, result} = run(root, {snapshots: [snapshot.directory]});
    assert.equal(result.included, 1);
    assert.equal(result.nonCitable, false);
    const reasons = readCSV(path.join(output, 'excluded.csv')).map((row) => row.reason).sort();
    assert.deepEqual(reasons, [
        'no acreditada: intento discarded en el indice',
        `no acreditada: manifiesto del snapshot ${'b'.repeat(12)} no provisto (--snapshot)`,
        'no acreditada: sin snapshot-context.json: estado inicial no acreditado',
        'no citable: serie trial no citable',
    ]);

    const withTrial = run(root, {snapshots: [snapshot.directory], allowNonCitable: true});
    assert.equal(withTrial.result.included, 2);
    assert.equal(withTrial.result.nonCitable, true);
    assert.match(fs.readFileSync(path.join(withTrial.output, 'report.md'), 'utf8'), /^# NO CITABLE/u);
    assert.match(fs.readFileSync(path.join(withTrial.output, 'figures', 'throughput.svg'), 'utf8'), /NO CITABLE/u);
});

test('a snapshot with non-canonical counts or a different manifest is not accredited', () => {
    const root = temporary('snt-analysis-');
    const bad = writeSnapshot(root, 'd'.repeat(64), {...CANONICAL_COUNTS, prepared: 22_000});
    writeSeries(root, {snapshot: bad, rounds: [{meta: {repetition: 1}}]});
    const good = writeSnapshot(root, 'e'.repeat(64));
    writeSeries(root, {snapshot: {...good, manifestSHA256: '0'.repeat(64)}, rounds: [{meta: {repetition: 1}}]});
    const {output, result} = run(root, {snapshots: [bad.directory, good.directory]});
    assert.equal(result.included, 0);
    const reasons = readCSV(path.join(output, 'excluded.csv')).map((row) => row.reason).sort();
    assert.match(reasons[0], /el manifiesto provisto no coincide/u);
    assert.match(reasons[1], /conteos no canonicos: prepared 22000/u);
});

test('repetitions with different load conditions never form one group', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root, 'a'.repeat(64));
    writeSeries(root, {snapshot, rounds: [
        {meta: {repetition: 1, workers: 4, duration: 120}},
        {meta: {repetition: 2, workers: 64, duration: 60}, sum: {seconds: 60}},
    ]});
    writeSeries(root, {sut: 'baseline', snapshot, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}}]});
    const {output, result} = run(root, {snapshots: [snapshot.directory]});
    assert.equal(result.aggregates, 3);
    assert.equal(result.comparisons, 0);
    assert.match(fs.readFileSync(path.join(output, 'report.md'), 'utf8'), /repeticiones con condiciones de carga distintas/u);
});

test('comparisons use only paired repetitions and list the missing ones', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root, 'a'.repeat(64));
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}}, {meta: {repetition: 3}}]});
    writeSeries(root, {sut: 'baseline', snapshot, rounds: [{meta: {repetition: 2}}, {meta: {repetition: 3}}, {meta: {repetition: 4}}]});
    const {output} = run(root, {snapshots: [snapshot.directory]});
    const [row] = readCSV(path.join(output, 'comparison.csv'));
    assert.equal(row.pairedRepetitions, '2 3');
    assert.equal(row.missingFabric, '4');
    assert.equal(row.missingBaseline, '1');

    const disjoint = temporary('snt-analysis-');
    const other = writeSnapshot(disjoint, 'a'.repeat(64));
    writeSeries(disjoint, {snapshot: other, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}}]});
    writeSeries(disjoint, {sut: 'baseline', snapshot: other, rounds: [{meta: {repetition: 4}}, {meta: {repetition: 5}}]});
    const result = run(disjoint, {snapshots: [other.directory]});
    assert.equal(result.result.comparisons, 0);
    assert.match(fs.readFileSync(path.join(result.output, 'report.md'), 'utf8'), /sin repeticiones emparejadas/u);
});

test('the original five and the extended eight are both reported, with the metric that extended them', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root, 'a'.repeat(64));
    const throughputs = [10, 10, 6, 10, 10, 10, 10, 10];
    writeSeries(root, {snapshot, rounds: throughputs.map((throughput, index) => ({meta: {repetition: index + 1}, sum: {throughput}}))});
    writeSeries(root, {sut: 'baseline', snapshot, rounds: throughputs.map((_, index) => ({meta: {repetition: index + 1}}))});
    const {output} = run(root, {snapshots: [snapshot.directory]});
    const fabric = readCSV(path.join(output, 'aggregate.csv')).filter((row) => row.sut === 'fabric');
    assert.deepEqual(fabric.map((row) => [row.series, row.repetitions, row.extendedBy]),
        [['original', '1 2 3 4 5', 'throughput'], ['extendida', '1 2 3 4 5 6 7 8', 'throughput']]);
    assert.deepEqual(readCSV(path.join(output, 'comparison.csv')).map((row) => row.series), ['original', 'extendida']);
});

test('the physical rate counts every attempt separately from confirmed transactions', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root, 'a'.repeat(64));
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 1, operation: 'transfer'}, sum: {throughput: 1, seconds: 1, attempts: 3}}]});
    const {output} = run(root, {snapshots: [snapshot.directory]});
    const [row] = readCSV(path.join(output, 'repetitions.csv'));
    assert.equal(Number(row.confirmedTransactionThroughput), 1);
    assert.equal(Number(row.attemptedTransactionThroughput), 3);
});

test('failures of excluded rounds are still listed with their status', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root, 'a'.repeat(64));
    const failing = raw(1, {outcome: 'unexpected-failure', datasetSequence: 7,
        transactions: [{status: 'failed', errorCode: 'DEADLINE_EXCEEDED'}]});
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 1, discarded: '1 operaciones tuvieron fallos inesperados'}, raw: failing}]});
    const {output} = run(root, {snapshots: [snapshot.directory]});
    const [row] = readCSV(path.join(output, 'unexpected.csv'));
    assert.equal(row.status, 'excluida');
    assert.equal(row.code, 'DEADLINE_EXCEEDED');
    assert.equal(row.timeouts, '1');
});

test('participation markers are reported per round, mismatches invalidate the group, preparation is separate', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root, 'a'.repeat(64));
    const markers = (observed) => ({expected: 600, observed, fromRegistrations: observed, fromRegulatoryEvents: 0,
        successfulWriteTransactions: 600, perSecond: observed / 120, shareOfSuccessfulWrites: observed / 600});
    writeSeries(root, {snapshot, rounds: [
        {meta: {repetition: 1, scenario: 'write-register', operation: 'register', markers: markers(600)}},
        {meta: {repetition: 2, scenario: 'write-register', operation: 'register', markers: markers(599)}},
    ]});
    writeJSON(path.join(root, 'prep', 'metadata.json'), metadata({scenario: 'dataset-preparation', phase: 'preparation', operation: 'register',
        markers: {expected: 46924, observed: 46924, fromRegistrations: 46920, fromRegulatoryEvents: 4,
            successfulWriteTransactions: 129878, perSecond: 7, shareOfSuccessfulWrites: 0.36}}));
    const {output} = run(root, {snapshots: [snapshot.directory]});
    const repetitions = readCSV(path.join(output, 'repetitions.csv'));
    assert.deepEqual(repetitions.map((row) => row.markersObserved).sort(), ['599', '600']);
    const [aggregateRow] = readCSV(path.join(output, 'aggregate.csv'));
    assert.match(aggregateRow.problems, /marcadores esperados distintos de observados en repeticiones 2/u);
    const [preparation] = readCSV(path.join(output, 'preparation.csv'));
    assert.equal(preparation.markersObserved, '46924');
});

function faultOperations(classOf) {
    const injectedAt = Date.parse('2026-10-10T10:01:00Z');
    const operations = [];
    for (let second = -60; second < 120; second += 1) {
        for (const availabilityClass of ['target', 'control', 'control', 'control']) {
            const failing = availabilityClass === 'target' && second >= 0 && second < 60;
            const started = injectedAt + second * 1000;
            operations.push({
                startedAt: new Date(started).toISOString(), endedAt: new Date(started + 50).toISOString(), latencyMs: 50,
                outcome: failing ? 'unexpected-failure' : 'success', availabilityClass: classOf(availabilityClass),
                transactions: failing ? [{status: 'failed', errorCode: 'UNAVAILABLE'}] : [{status: 'success'}],
            });
        }
    }
    return operations;
}

const FAULT = {target: 'peer0.farmacia', injectedAt: '2026-10-10T10:01:00Z', recoveredAt: '2026-10-10T10:02:00Z',
    preFaultSeconds: 60, faultSeconds: 60, recoverySeconds: 60};

test('availability splits pre-fault, fault and recovery windows and finds the recovery time', () => {
    const run_ = {scenario: 'raft-1', raw: faultOperations(() => undefined), faultInjection: FAULT};
    const result = availabilityWindows(run_);
    assert.equal(result.windows.preFault.successRate, 1);
    assert.equal(result.windows.fault.successRate, 0.75);
    assert.equal(result.windows.recovery.successRate, 1);
    assert.equal(result.byClass, undefined);
    assert.equal(successTimeline(run_, 10).find((point) => point.offsetSeconds === 0).successRate, 0.75);
});

test('peer faults report availability per operation class and block without classes', () => {
    const classified = availabilityWindows({scenario: 'peer-1a', raw: faultOperations((name) => name), faultInjection: FAULT});
    assert.equal(classified.windows.fault.successRate, 0.75);
    assert.equal(classified.byClass.target.windows.fault.successRate, 0);
    assert.equal(classified.byClass.control.windows.fault.successRate, 1);
    const unclassified = availabilityWindows({scenario: 'peer-1a', raw: faultOperations(() => undefined), faultInjection: FAULT});
    assert.match(unclassified.blocked, /sin availabilityClass/u);
});

test('aggregate and compare keep both SUT apart and report ratios on paired repetitions', () => {
    const metricsRun = (sut, repetition, p95) => ({
        sut, scenario: 'read-unit', variant: '', operation: 'query-unit', targetOperationsPerSecond: 10,
        transactionsPerOperation: 1, workers: 4, durationSeconds: 120, repetition, datasetSHA256: DATASET,
        artifact: {sut}, accreditation: {snapshot: {id: 'a'}}, summary: summary({p95}), raw: raw(),
    });
    const aggregates = aggregate([metricsRun('fabric', 1, 900), metricsRun('baseline', 1, 3)]);
    const {comparisons} = compare(aggregates);
    assert.equal(comparisons.length, 1);
    assert.equal(comparisons[0].p95Ratio, 300);
});
