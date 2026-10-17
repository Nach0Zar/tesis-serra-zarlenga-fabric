'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {validateDocument} = require('../../caliper/src/artifacts');
const {PLANS, createSeriesDirectory, runSeries} = require('../../caliper/src/run-series');
const {parseRoundArguments} = require('../src/arguments');
const {createOutputDirectory, repoRoot} = require('../src/paths');
const {
    SCENARIOS, SIMULATION, availabilityProfile, availabilitySummary, parseArguments, scenarioDefinition,
} = require('../src/run-availability');
const {attemptExecutor, roundArguments, scenarioMatrix} = require('../src/run-series');

test('the two scenarios of section 8.2 take down the single database and the single API', () => {
    assert.deepEqual(Object.keys(SCENARIOS), ['db-1', 'api-1']);
    assert.equal(scenarioDefinition('db-1').service, 'postgres');
    assert.equal(scenarioDefinition('api-1').service, 'api');
    assert.throws(() => scenarioDefinition('raft-1'), /unsupported baseline availability scenario/u);
    assert.match(SIMULATION, /mismo nivel de simulacion que EVAL-4/u);
    assert.match(SIMULATION, /propiedad de la topologia centralizada/u);
});

test('the baseline availability load is the 180 s mixed load of EVAL-4', () => {
    const profile = availabilityProfile({phase: 'measurement', repetition: '1'});
    assert.equal(profile.scenario, 'mixed');
    assert.equal(profile.rate, 20);
    assert.equal(profile.durationSeconds, 180);
    assert.equal(profile.workers, 2);
    assert.ok(Number.isInteger(profile.durationSeconds * profile.rate / profile.workers));
    assert.deepEqual(parseArguments(['--scenario', 'api-1', '--phase', 'warmup', '--repetition', '0', '--snapshot-dir', 's']).scenario, 'api-1');
    assert.throws(() => parseArguments(['--scenario', 'db-2', '--phase', 'warmup', '--repetition', '0', '--snapshot-dir', 's']),
        /unsupported baseline availability scenario/u);
});

test('only failures that ended before the fault discard a baseline availability round', () => {
    const windows = {injectedAt: '2026-10-17T10:01:00.000Z'};
    const definition = scenarioDefinition('db-1');
    const op = (startedAt, endedAt, outcome) => ({startedAt, endedAt, outcome});
    const kept = availabilitySummary({scenario: 'mixed'}, [
        op('2026-10-17T10:00:10.000Z', '2026-10-17T10:00:10.050Z', 'success'),
        op('2026-10-17T10:00:59.900Z', '2026-10-17T10:01:00.300Z', 'unexpected-failure'),
        op('2026-10-17T10:01:10.000Z', '2026-10-17T10:01:40.000Z', 'unexpected-failure'),
    ], windows, definition);
    assert.equal(kept.discardReason, undefined);
    assert.equal(kept.scenario, 'db-1');
    const discarded = availabilitySummary({}, [op('2026-10-17T10:00:10.000Z', '2026-10-17T10:00:11.000Z', 'unexpected-failure')],
        windows, definition);
    assert.match(discarded.discardReason, /1 operaciones fallaron antes/u);
});

test('baseline rounds can be written inside a series directory, never outside build', () => {
    assert.equal(parseRoundArguments([
        '--scenario', 'mixed', '--phase', 'warmup', '--repetition', '0', '--rate', '20', '--snapshot-dir', 's',
        '--output-root', 'build/benchmarks/baseline/series/x/attempt',
    ]).outputRoot, 'build/benchmarks/baseline/series/x/attempt');
    const scratch = fs.mkdtempSync(path.join(repoRoot, 'build', 'baseline-output-'));
    try {
        const directory = createOutputDirectory('runs', 'round', path.relative(repoRoot, scratch));
        assert.equal(directory, path.join(scratch, 'round'));
    } finally {
        fs.rmSync(scratch, {recursive: true, force: true});
    }
    assert.throws(() => createOutputDirectory('runs', 'round', os.tmpdir()), /inside the repository build directory/u);
});

test('the baseline series uses the same plans, arguments and index as Fabric', async () => {
    assert.deepEqual(scenarioMatrix('availability').map((entry) => entry.key), ['db-1', 'api-1']);
    assert.equal(scenarioMatrix('full').length, 20);
    assert.deepEqual(roundArguments({scenario: 'db-1', rate: 20}, 'measurement', 2),
        ['--scenario', 'db-1', '--phase', 'measurement', '--repetition', '2']);
    assert.deepEqual(roundArguments({scenario: 'expected-rejections', rate: 5, family: 'BLOCKING_STATE', operation: 'dispense'}, 'warmup', 0),
        ['--scenario', 'expected-rejections', '--phase', 'warmup', '--repetition', '0', '--rate', '5',
            '--family', 'BLOCKING_STATE', '--operation', 'dispense']);
    assert.equal(PLANS.availability.citable, true);

    const root = fs.mkdtempSync(path.join(repoRoot, 'build', 'baseline-series-'));
    const calls = [];
    const executor = attemptExecutor({spawnSync: (command, args) => {
        calls.push(args);
        const output = path.join(repoRoot, args[args.indexOf('--output-root') + 1], 'round');
        fs.mkdirSync(output, {recursive: true});
        const discarded = calls.length === 1;
        fs.writeFileSync(path.join(output, 'summary.json'), JSON.stringify({
            discardReason: discarded ? 'operaciones fallaron antes de inyectar la falla' : undefined,
            rate: {effectiveOperationsPerSecond: 20}, operationLatency: {p95Ms: 10},
        }));
        return {status: discarded ? 1 : 0, stdout: '', stderr: ''};
    }});
    try {
        const result = await runSeries(repoRoot, {
            plan: 'availability-trial', snapshotDirectory: 'build/snapshot', outputRoot: path.relative(repoRoot, root),
        }, {
            readSnapshot: () => ({
                directory: path.join(repoRoot, 'build', 'snapshot'), manifestPath: __filename,
                manifest: {snapshotId: 'b'.repeat(64)},
            }),
            createSeriesDirectory: (repo, options) => createSeriesDirectory(repo, {...options, seriesToken: 'eval5'}),
            scenarioMatrix: scenarioMatrix('availability-trial'),
            executeAttempt: executor,
        });
        assert.equal(result.index.status, 'completed');
        assert.equal(result.index.citable, false);
        validateDocument(repoRoot, 'fabric-series-index.schema.json', result.index);
        const dbAttempts = result.index.scenarios[0].attempts.map((attempt) => `${attempt.kind}${attempt.repetition}:${attempt.status}`);
        assert.deepEqual(dbAttempts, ['warmup0:discarded', 'warmup0:completed', 'measurement1:completed']);
        assert.ok(calls.every((args) => args[0].endsWith('run-availability.js')));
        assert.ok(calls.every((args) => args.includes('--snapshot-dir') && args.includes('--run-token')));
        const roundDirectory = path.join(result.directory, 'db-1', 'measurements', 'run-01', 'round');
        assert.ok(fs.existsSync(path.join(roundDirectory, 'summary.json')), 'rounds live inside the series index tree');
    } finally {
        fs.rmSync(root, {recursive: true, force: true});
    }
});
