'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {availabilityWindows, successTimeline} = require('../src/availability');
const {CANONICAL_COUNTS, FABRIC_MANIFEST_SCHEMA} = require('../src/evidence');
const {aggregate, compare} = require('../src/metrics');
const {processResults} = require('../src/process');
const {MIX_PATTERN, exclusionReason, verifyMixComposition} = require('../src/runs');
const {coefficientOfVariation, describe, percentile, populationStandardDeviation} = require('../src/stats');

const DATASET = 'de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d';
const FABRIC_ENV = {docker: '27', dockerCompose: '2', contractVersion: '2.11.2', packageID: 'snt_1.0:abc', fabric: '2.5.16', fabricCA: '1.5.17', caliper: '0.7.1'};
const BASELINE_ENV = {docker: '27', dockerCompose: '2', baselineCommit: 'c'.repeat(40), baselineImage: `snt-baseline@sha256:${'a'.repeat(64)}`};

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

function hex(length = 64) {
    return crypto.randomBytes(length / 2).toString('hex');
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
        workerIndex: 0, ordinal: index, datasetSequence: index + 1, type: 'query-unit', outcome: 'success', latencyMs: 5,
        startedAt: '2026-10-10T10:00:01Z', endedAt: '2026-10-10T10:00:01Z', transactions: [{status: 'success'}],
        ...overrides,
    }));
}

function writeFile(directory, name, contents) {
    const filePath = path.join(directory, name);
    fs.mkdirSync(path.dirname(filePath), {recursive: true});
    fs.writeFileSync(filePath, contents);
    return {name, sha256: sha256(filePath), bytes: fs.statSync(filePath).size};
}

// Snapshot acreditable de cada SUT: Fabric cumple el schema de #141; la
// baseline, los campos que #139 registra. `plan` fija el contenido del plan
// logico, es decir su identidad.
function writeSnapshot(root, {sut = 'fabric', plan = 'plan-A', counts = CANONICAL_COUNTS, edit} = {}) {
    const id = hex();
    const directory = path.join(root, 'snapshots', `${sut}-${id.slice(0, 8)}`);
    const logicalPlan = writeFile(directory, 'logical-plan.json', JSON.stringify({plan}));
    const preconditions = writeFile(directory, 'preconditions.json', '[]');
    let manifest;
    if (sut === 'fabric') {
        manifest = {
            $schema: FABRIC_MANIFEST_SCHEMA, schemaVersion: '1.0.0', snapshotId: id, createdAt: '2026-10-10T09:00:00.000Z',
            dataset: {seed: 20260727, sha256: DATASET, manifestSHA256: hex(), units: 50000}, counts,
            preparation: {durationSeconds: 6000, successfulWriteTransactions: 129878, concurrency: 64,
                markers: {expected: 46924, observed: 46924, fromRegistrations: 46920, fromRegulatoryEvents: 4}},
            artifacts: {repositoryCommit: hex(40), contractVersion: '2.11.2', packageID: `snt_1.0:${hex()}`, fabric: '2.5.16',
                fabricCA: '1.5.17', docker: '27', dockerCompose: '2', composeProject: 'snt-fabric',
                networkConfigSHA256: hex(), sourceSHA256: hex()},
            smokeSequence: 2824,
            files: {
                logicalPlan, preconditions,
                semanticVerification: writeFile(directory, 'semantic-verification.json', '{"present":46920}'),
                preparationMetadata: writeFile(directory, 'preparation-metadata.json', '{}'),
                journal: writeFile(directory, 'preparation-journal.jsonl', '{}\n'),
            },
            volumes: ['anmat', 'lab', 'drogueria', 'distribuidor', 'farmacia', 'centromedico', 'financiador', 'x', 'y', 'z']
                .map((organization, index) => ({
                    name: `${index < 3 ? 'orderer' : 'peer'}-${organization}-data`,
                    service: `${index < 3 ? 'orderer' : 'peer0'}.${organization}.snt.local`,
                    containerPath: '/var/hyperledger/production', archive: `volumes/${organization}.tar`,
                    sha256: hex(), treeSHA256: hex(), bytes: 1, fileCount: 1,
                })),
        };
    } else {
        manifest = {
            schemaVersion: '1.0.0', snapshotId: id, dataset: {seed: 20260727, sha256: DATASET, units: 50000},
            counts: {...counts, registered: undefined}, smokeSequence: 2824, logicalPlanSHA256: logicalPlan.sha256,
            artifacts: {baselineCommit: hex(40), baselineImage: `snt-baseline:local@sha256:${hex()}`},
            files: {dump: writeFile(directory, 'baseline.dump', 'dump'), preconditions, logicalPlan},
        };
    }
    if (edit) edit(manifest, directory);
    writeJSON(path.join(directory, 'manifest.json'), manifest);
    return {directory, id, sut, manifestSHA256: sha256(path.join(directory, 'manifest.json')), logicalPlanSHA256: logicalPlan.sha256};
}

function contextFor(sut, snapshot, operations, {withManifestHash = true, verified} = {}) {
    const sequences = verified ?? [...new Set(operations.map((operation) => operation.datasetSequence))];
    const verifiedPreconditions = sequences.map((sequence) => ({sequence, present: true}));
    const manifestSHA256 = withManifestHash ? snapshot.manifestSHA256 : undefined;
    return sut === 'fabric'
        ? {snapshotId: snapshot.id, manifestSHA256, verifiedPreconditions}
        : {snapshot: {id: snapshot.id, manifestSHA256, logicalPlanSHA256: snapshot.logicalPlanSHA256}, verifiedPreconditions};
}

// Serie con indice (#141) y una ronda por entrada, en el formato del orquestador.
function writeSeries(root, {sut = 'fabric', plan = 'full', snapshot, rounds, index = true, indexManifestSHA256, context = {}}) {
    const directory = path.join(root, `${sut}-${plan}-${crypto.randomUUID().slice(0, 6)}`);
    const scenarios = new Map();
    for (const round of rounds) {
        const key = `${round.meta.scenario ?? 'read-unit'}-${round.meta.rate ?? 10}`;
        const kind = (round.meta.phase ?? 'measurement') === 'warmup' ? 'warmup' : 'measurement';
        const attemptDirectory = kind === 'warmup' ? `${key}/warmup/try-1`
            : `${key}/measurements/run-${String(round.meta.repetition ?? 1).padStart(2, '0')}`;
        const roundDirectory = path.join(directory, attemptDirectory, 'round');
        const operations = round.raw ?? raw();
        writeJSON(path.join(roundDirectory, 'metadata.json'), metadata({sut, ...round.meta}));
        writeJSON(path.join(roundDirectory, 'summary.json'), summary(round.sum));
        if (round.raw !== null) writeJSON(path.join(roundDirectory, 'raw.json'), operations);
        if (snapshot) writeJSON(path.join(roundDirectory, 'snapshot-context.json'), contextFor(sut, snapshot, operations, context));
        const scenario = scenarios.get(key) ?? {key, scenario: round.meta.scenario ?? 'read-unit', rate: round.meta.rate ?? 10,
            status: 'completed', extended: false, attempts: []};
        scenario.attempts.push({kind, repetition: kind === 'warmup' ? 0 : round.meta.repetition ?? 1, try: 1,
            status: round.attemptStatus ?? 'completed', startedAt: '2026-10-10T10:00:00.000Z',
            endedAt: '2026-10-10T10:02:00.000Z', directory: attemptDirectory});
        scenarios.set(key, scenario);
    }
    if (index) {
        writeJSON(path.join(directory, 'series-index.json'), {
            $schema: 'urn:pfi-snt:fabric-series-index:schema:1.0.0', schemaVersion: '1.0.0', seriesToken: 'token',
            plan, citable: plan === 'full',
            snapshot: {id: snapshot?.id ?? hex(), directory: 'x', manifestSHA256: indexManifestSHA256 ?? snapshot?.manifestSHA256 ?? hex()},
            startedAt: '2026-10-10T10:00:00.000Z', endedAt: '2026-10-10T11:00:00.000Z', status: 'completed',
            scenarios: [...scenarios.values()],
        });
    }
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

function exclusionReasons(output) {
    return readCSV(path.join(output, 'excluded.csv')).map((row) => row.reason).sort();
}

function pair(root, options = {}) {
    const fabric = writeSnapshot(root, {sut: 'fabric', ...options.fabric});
    const baseline = writeSnapshot(root, {sut: 'baseline', ...options.baseline});
    return {fabric, baseline, snapshots: [fabric.directory, baseline.directory]};
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
    const snapshot = writeSnapshot(root);
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}, attemptStatus: 'discarded'}]});
    writeSeries(root, {plan: 'trial', snapshot, rounds: [{meta: {repetition: 1}}]});
    writeSeries(root, {rounds: [{meta: {repetition: 3}}]});
    writeSeries(root, {snapshot: {id: hex(), manifestSHA256: hex(), logicalPlanSHA256: hex()}, rounds: [{meta: {repetition: 4}}]});

    const {output, result} = run(root, {snapshots: [snapshot.directory]});
    assert.equal(result.included, 1);
    assert.equal(result.nonCitable, false);
    const reasons = exclusionReasons(output);
    assert.equal(reasons.length, 4);
    assert.ok(reasons.includes('no acreditada: intento discarded en el indice'));
    assert.ok(reasons.includes('no acreditada: sin snapshot-context.json: estado inicial no acreditado'));
    assert.ok(reasons.includes('no citable: serie trial no citable'));
    assert.ok(reasons.some((reason) => /no provisto \(--snapshot\)/u.test(reason)));

    const withTrial = run(root, {snapshots: [snapshot.directory], allowNonCitable: true});
    assert.equal(withTrial.result.included, 2);
    assert.equal(withTrial.result.nonCitable, true);
    assert.match(fs.readFileSync(path.join(withTrial.output, 'report.md'), 'utf8'), /^# NO CITABLE/u);
    assert.match(fs.readFileSync(path.join(withTrial.output, 'figures', 'throughput.svg'), 'utf8'), /NO CITABLE/u);
});

test('incomplete or inconsistent evidence never accredits the initial state', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root);
    // Contexto sin hash del manifiesto.
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 1}}], context: {withManifestHash: false}});
    // Indice que declara otro manifiesto.
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 2}}], indexManifestSHA256: hex()});
    // Verificacion previa que no cubre las unidades de la ronda.
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 3}}], context: {verified: [1]}});
    // Indice que no cumple el schema de #141.
    const broken = writeSeries(root, {snapshot, rounds: [{meta: {repetition: 4}}]});
    const indexPath = path.join(broken, 'series-index.json');
    const index = JSON.parse(fs.readFileSync(indexPath, 'utf8'));
    delete index.scenarios[0].extended;
    fs.writeFileSync(indexPath, JSON.stringify(index));

    const {output, result} = run(root, {snapshots: [snapshot.directory]});
    assert.equal(result.included, 0);
    const reasons = exclusionReasons(output).join('\n');
    assert.match(reasons, /el contexto no registra el hash del manifiesto/u);
    assert.match(reasons, /la serie declara otro manifiesto del snapshot/u);
    assert.match(reasons, /2 unidades de la ronda sin verificacion previa/u);
    assert.match(reasons, /indice de serie invalido: \/scenarios\/0 must have required property 'extended'/u);
});

test('snapshot manifests must satisfy their contract and reference intact files', () => {
    const root = temporary('snt-analysis-');
    const counts = writeSnapshot(root, {counts: {...CANONICAL_COUNTS, prepared: 22_000}});
    const fabricSchema = writeSnapshot(root, {edit: (manifest) => {
        delete manifest.preparation;
    }});
    const fabricFile = writeSnapshot(root, {edit: (manifest, directory) => {
        fs.writeFileSync(path.join(directory, 'semantic-verification.json'), '{"altered":true}');
    }});
    const baselinePlan = writeSnapshot(root, {sut: 'baseline', edit: (manifest) => {
        delete manifest.files.logicalPlan;
    }});
    const baselineImage = writeSnapshot(root, {sut: 'baseline', edit: (manifest) => {
        manifest.artifacts.baselineImage = 'snt-baseline:local';
    }});
    for (const snapshot of [counts, fabricSchema, fabricFile]) writeSeries(root, {snapshot, rounds: [{meta: {repetition: 1}}]});
    for (const snapshot of [baselinePlan, baselineImage]) writeSeries(root, {sut: 'baseline', snapshot, rounds: [{meta: {repetition: 1}}]});

    const {output, result} = run(root, {snapshots: [counts, fabricSchema, fabricFile, baselinePlan, baselineImage].map((entry) => entry.directory)});
    assert.equal(result.included, 0);
    const reasons = exclusionReasons(output).join('\n');
    assert.match(reasons, /conteos no canonicos: prepared 22000/u);
    assert.match(reasons, /schema: \/ must have required property 'preparation'/u);
    assert.match(reasons, /hash distinto en semantic-verification\.json/u);
    assert.match(reasons, /falta files\.logicalPlan/u);
    assert.match(reasons, /falta un digest inmutable en artifacts\.baselineImage/u);
});

test('both SUT are compared only when they share the same logical plan', () => {
    const root = temporary('snt-analysis-');
    const same = pair(root);
    writeSeries(root, {snapshot: same.fabric, rounds: [{meta: {repetition: 1}}]});
    writeSeries(root, {sut: 'baseline', snapshot: same.baseline, rounds: [{meta: {repetition: 1}}]});
    const equal = run(root, {snapshots: same.snapshots});
    assert.equal(equal.result.comparisons, 1);
    assert.equal(readCSV(path.join(equal.output, 'comparison.csv'))[0].logicalPlanSHA256, same.fabric.logicalPlanSHA256);

    const other = temporary('snt-analysis-');
    const different = pair(other, {baseline: {plan: 'plan-B'}});
    writeSeries(other, {snapshot: different.fabric, rounds: [{meta: {repetition: 1}}]});
    writeSeries(other, {sut: 'baseline', snapshot: different.baseline, rounds: [{meta: {repetition: 1}}]});
    const distinct = run(other, {snapshots: different.snapshots});
    assert.equal(distinct.result.comparisons, 0);
    assert.match(fs.readFileSync(path.join(distinct.output, 'report.md'), 'utf8'), /plan logico inicial distinto o no acreditado/u);
});

test('repetitions with different load conditions never form one group', () => {
    const root = temporary('snt-analysis-');
    const snapshots = pair(root);
    writeSeries(root, {snapshot: snapshots.fabric, rounds: [
        {meta: {repetition: 1, workers: 4, duration: 120}},
        {meta: {repetition: 2, workers: 64, duration: 60}, sum: {seconds: 60}},
    ]});
    writeSeries(root, {sut: 'baseline', snapshot: snapshots.baseline, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}}]});
    const {output, result} = run(root, {snapshots: snapshots.snapshots});
    assert.equal(result.aggregates, 3);
    assert.equal(result.comparisons, 0);
    assert.match(fs.readFileSync(path.join(output, 'report.md'), 'utf8'), /repeticiones con condiciones de carga distintas/u);
});

test('comparisons use only paired repetitions and list the missing ones', () => {
    const root = temporary('snt-analysis-');
    const snapshots = pair(root);
    writeSeries(root, {snapshot: snapshots.fabric, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}}, {meta: {repetition: 3}}]});
    writeSeries(root, {sut: 'baseline', snapshot: snapshots.baseline, rounds: [{meta: {repetition: 2}}, {meta: {repetition: 3}}, {meta: {repetition: 4}}]});
    const {output} = run(root, {snapshots: snapshots.snapshots});
    const [row] = readCSV(path.join(output, 'comparison.csv'));
    assert.equal(row.pairedRepetitions, '2 3');
    assert.equal(row.missingFabric, '4');
    assert.equal(row.missingBaseline, '1');

    const disjoint = temporary('snt-analysis-');
    const others = pair(disjoint);
    writeSeries(disjoint, {snapshot: others.fabric, rounds: [{meta: {repetition: 1}}, {meta: {repetition: 2}}]});
    writeSeries(disjoint, {sut: 'baseline', snapshot: others.baseline, rounds: [{meta: {repetition: 4}}, {meta: {repetition: 5}}]});
    const result = run(disjoint, {snapshots: others.snapshots});
    assert.equal(result.result.comparisons, 0);
    assert.match(fs.readFileSync(path.join(result.output, 'report.md'), 'utf8'), /sin repeticiones emparejadas/u);
});

test('the original five and the extended eight are both reported, with the metric that extended them', () => {
    const root = temporary('snt-analysis-');
    const snapshots = pair(root);
    const throughputs = [10, 10, 6, 10, 10, 10, 10, 10];
    writeSeries(root, {snapshot: snapshots.fabric, rounds: throughputs.map((throughput, index) => ({meta: {repetition: index + 1}, sum: {throughput}}))});
    writeSeries(root, {sut: 'baseline', snapshot: snapshots.baseline, rounds: throughputs.map((_, index) => ({meta: {repetition: index + 1}}))});
    const {output} = run(root, {snapshots: snapshots.snapshots});
    const fabric = readCSV(path.join(output, 'aggregate.csv')).filter((row) => row.sut === 'fabric');
    assert.deepEqual(fabric.map((row) => [row.series, row.repetitions, row.extendedBy]),
        [['original', '1 2 3 4 5', 'throughput'], ['extendida', '1 2 3 4 5 6 7 8', 'throughput']]);
    assert.deepEqual(readCSV(path.join(output, 'comparison.csv')).map((row) => row.series), ['original', 'extendida']);
});

test('the physical rate counts every attempt separately from confirmed transactions', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root);
    writeSeries(root, {snapshot, rounds: [{meta: {repetition: 1, operation: 'transfer'}, sum: {throughput: 1, seconds: 1, attempts: 3}}]});
    const {output} = run(root, {snapshots: [snapshot.directory]});
    const [row] = readCSV(path.join(output, 'repetitions.csv'));
    assert.equal(Number(row.confirmedTransactionThroughput), 1);
    assert.equal(Number(row.attemptedTransactionThroughput), 3);
});

test('failures of excluded rounds are still listed with their status', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root);
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
    const snapshot = writeSnapshot(root);
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
                datasetSequence: 1, startedAt: new Date(started).toISOString(), endedAt: new Date(started + 50).toISOString(), latencyMs: 50,
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

test('operations in flight when the fault is injected count in the fault window', () => {
    const injectedAt = Date.parse(FAULT.injectedAt);
    const operation = (startOffset, endOffset, outcome) => ({
        datasetSequence: 1, startedAt: new Date(injectedAt + startOffset).toISOString(),
        endedAt: new Date(injectedAt + endOffset).toISOString(), latencyMs: endOffset - startOffset, outcome,
        transactions: [{status: outcome === 'success' ? 'success' : 'failed'}],
    });
    const raw = [
        operation(-30_000, -29_950, 'success'),
        operation(-2_000, 1_200, 'unexpected-failure'),
        operation(10_000, 10_050, 'success'),
        operation(70_000, 70_050, 'success'),
    ];
    const result = availabilityWindows({scenario: 'raft-1', raw, faultInjection: FAULT});
    assert.equal(result.windows.preFault.operations, 1);
    assert.equal(result.windows.preFault.successRate, 1);
    assert.equal(result.windows.fault.operations, 2);
    assert.equal(result.windows.fault.successRate, 0.5);
});

test('peer faults report availability per operation class and block without classes', () => {
    const classified = availabilityWindows({scenario: 'peer-1a', raw: faultOperations((name) => name), faultInjection: FAULT});
    assert.equal(classified.windows.fault.successRate, 0.75);
    assert.equal(classified.byClass.target.windows.fault.successRate, 0);
    assert.equal(classified.byClass.control.windows.fault.successRate, 1);
    const unclassified = availabilityWindows({scenario: 'peer-1a', raw: faultOperations(() => undefined), faultInjection: FAULT});
    assert.match(unclassified.blocked, /sin availabilityClass/u);
});

function availabilityRound(scenario, classOf) {
    return {meta: {repetition: 1, scenario, rate: 20, operation: 'mixed', workers: 2, faultInjection: FAULT}, raw: faultOperations(classOf)};
}

test('availability outputs follow the same accreditation and citability as performance', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root);
    writeSeries(root, {snapshot, rounds: [availabilityRound('raft-1', () => undefined)], index: false});

    const strict = run(root, {snapshots: [snapshot.directory]});
    assert.equal(fs.existsSync(path.join(strict.output, 'availability.json')), false);
    assert.equal(fs.readdirSync(path.join(strict.output, 'figures')).some((name) => name.startsWith('availability-')), false);
    assert.match(exclusionReasons(strict.output).join('\n'), /escenario de disponibilidad; no citable: sin indice de serie/u);

    const diagnostic = run(root, {snapshots: [snapshot.directory], allowNonCitable: true});
    assert.equal(diagnostic.result.nonCitable, true);
    const [entry] = JSON.parse(fs.readFileSync(path.join(diagnostic.output, 'availability.json'), 'utf8'));
    assert.equal(entry.nonCitable, 'sin indice de serie');
    assert.match(fs.readFileSync(path.join(diagnostic.output, 'report.md'), 'utf8'), /^# NO CITABLE/u);
    const figure = fs.readdirSync(path.join(diagnostic.output, 'figures')).find((name) => name.startsWith('availability-'));
    assert.match(fs.readFileSync(path.join(diagnostic.output, 'figures', figure), 'utf8'), /NO CITABLE/u);
});

test('peer-fault figures show the operation classes and are not exported when blocked', () => {
    const root = temporary('snt-analysis-');
    const snapshot = writeSnapshot(root);
    writeSeries(root, {snapshot, rounds: [availabilityRound('peer-1a', () => undefined)]});
    const blocked = run(root, {snapshots: [snapshot.directory]});
    assert.equal(fs.readdirSync(path.join(blocked.output, 'figures')).some((name) => name.startsWith('availability-')), false);
    assert.match(fs.readFileSync(path.join(blocked.output, 'report.md'), 'utf8'), /bloqueado/u);

    const classified = temporary('snt-analysis-');
    const other = writeSnapshot(classified);
    writeSeries(classified, {snapshot: other, rounds: [availabilityRound('peer-1a', (name) => name)]});
    const result = run(classified, {snapshots: [other.directory]});
    const figure = fs.readdirSync(path.join(result.output, 'figures')).find((name) => name.startsWith('availability-'));
    const svg = fs.readFileSync(path.join(result.output, 'figures', figure), 'utf8');
    assert.match(svg, /Objetivo \(requiere el peer caído\)/u);
    assert.match(svg, /Control/u);
});

test('aggregate and compare keep both SUT apart and report ratios on paired repetitions', () => {
    const metricsRun = (sut, repetition, p95) => ({
        sut, scenario: 'read-unit', variant: '', operation: 'query-unit', targetOperationsPerSecond: 10,
        transactionsPerOperation: 1, workers: 4, durationSeconds: 120, repetition, datasetSHA256: DATASET,
        artifact: {sut}, accreditation: {snapshot: {id: sut, logicalPlanSHA256: 'plan'}}, summary: summary({p95}), raw: raw(),
    });
    const aggregates = aggregate([metricsRun('fabric', 1, 900), metricsRun('baseline', 1, 3)]);
    const {comparisons} = compare(aggregates);
    assert.equal(comparisons.length, 1);
    assert.equal(comparisons[0].p95Ratio, 300);
});
